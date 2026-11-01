package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxResponseBytes = 2 << 20
	maxResultLimit   = 100
	metricsQuery     = "sum(rate(orbit_demo_requests_total[5m]))"
)

var ErrNotConfigured = errors.New("telemetry backend is not configured")

var serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

type QueryWindow struct {
	Name     string
	Duration time.Duration
	Step     string
}

func ParseWindow(value string) (QueryWindow, error) {
	if strings.TrimSpace(value) == "" {
		value = "1h"
	}
	switch strings.TrimSpace(value) {
	case "15m":
		return QueryWindow{Name: "15m", Duration: 15 * time.Minute, Step: "15s"}, nil
	case "1h":
		return QueryWindow{Name: "1h", Duration: time.Hour, Step: "1m"}, nil
	case "6h":
		return QueryWindow{Name: "6h", Duration: 6 * time.Hour, Step: "5m"}, nil
	case "24h":
		return QueryWindow{Name: "24h", Duration: 24 * time.Hour, Step: "15m"}, nil
	default:
		return QueryWindow{}, fmt.Errorf("window must be one of 15m, 1h, 6h, or 24h")
	}
}

func ParseLimit(value string, fallback int) (int, error) {
	if fallback < 1 || fallback > maxResultLimit {
		fallback = 20
	}
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || limit < 1 || limit > maxResultLimit {
		return 0, fmt.Errorf("limit must be an integer between 1 and %d", maxResultLimit)
	}
	return limit, nil
}

func ValidateServiceName(value string) error {
	if !serviceNamePattern.MatchString(value) {
		return fmt.Errorf("service must contain only letters, numbers, dots, underscores, colons, or hyphens")
	}
	return nil
}

type QueryResult struct {
	Source    string          `json:"source"`
	Window    string          `json:"window"`
	QueriedAt time.Time       `json:"queried_at"`
	Query     string          `json:"query,omitempty"`
	Step      string          `json:"step,omitempty"`
	Service   string          `json:"service,omitempty"`
	Limit     int             `json:"limit,omitempty"`
	Data      json.RawMessage `json:"data"`
}

type Client struct {
	prometheusURL string
	lokiURL       string
	tempoURL      string
	httpClient    *http.Client
	now           func() time.Time
}

type upstreamEnvelope struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result json.RawMessage `json:"result"`
	} `json:"data"`
	Traces json.RawMessage `json:"traces"`
}

func NewClient(prometheusURL, lokiURL, tempoURL string) (*Client, error) {
	prometheus, err := normalizeEndpoint("Prometheus", prometheusURL)
	if err != nil {
		return nil, err
	}
	loki, err := normalizeEndpoint("Loki", lokiURL)
	if err != nil {
		return nil, err
	}
	tempo, err := normalizeEndpoint("Tempo", tempoURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		prometheusURL: prometheus,
		lokiURL:       loki,
		tempoURL:      tempo,
		httpClient:    &http.Client{Timeout: 6 * time.Second},
		now:           time.Now,
	}, nil
}

func normalizeEndpoint(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("%s endpoint must be an absolute HTTP(S) URL", name)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s endpoint must use HTTP or HTTPS", name)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%s endpoint must not contain credentials, query parameters, or a fragment", name)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("%s endpoint must be a base URL without a path", name)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (c *Client) Metrics(ctx context.Context, window QueryWindow) (QueryResult, error) {
	if c == nil || c.prometheusURL == "" {
		return QueryResult{}, ErrNotConfigured
	}
	now := c.now().UTC()
	values := url.Values{}
	values.Set("query", metricsQuery)
	values.Set("start", now.Add(-window.Duration).Format(time.RFC3339Nano))
	values.Set("end", now.Format(time.RFC3339Nano))
	values.Set("step", window.Step)
	values.Set("timeout", "5s")
	values.Set("limit", "10")

	payload, err := c.get(ctx, c.prometheusURL, "/api/v1/query_range", values)
	if err != nil {
		return QueryResult{}, fmt.Errorf("query Prometheus: %w", err)
	}
	if payload.Status != "success" {
		return QueryResult{}, errors.New("Prometheus returned an unsuccessful query status")
	}
	return QueryResult{
		Source:    "prometheus",
		Window:    window.Name,
		QueriedAt: now,
		Query:     metricsQuery,
		Step:      window.Step,
		Data:      nonEmptyJSON(payload.Data.Result),
	}, nil
}

func (c *Client) Logs(ctx context.Context, window QueryWindow, service string, limit int) (QueryResult, error) {
	if c == nil || c.lokiURL == "" {
		return QueryResult{}, ErrNotConfigured
	}
	if err := ValidateServiceName(service); err != nil {
		return QueryResult{}, err
	}
	if limit < 1 || limit > maxResultLimit {
		return QueryResult{}, fmt.Errorf("limit must be between 1 and %d", maxResultLimit)
	}

	now := c.now().UTC()
	query := fmt.Sprintf("{service_name=%q}", service)
	values := url.Values{}
	values.Set("query", query)
	values.Set("start", strconv.FormatInt(now.Add(-window.Duration).UnixNano(), 10))
	values.Set("end", strconv.FormatInt(now.UnixNano(), 10))
	values.Set("limit", strconv.Itoa(limit))
	values.Set("direction", "backward")

	payload, err := c.get(ctx, c.lokiURL, "/loki/api/v1/query_range", values)
	if err != nil {
		return QueryResult{}, fmt.Errorf("query Loki: %w", err)
	}
	if payload.Status != "success" {
		return QueryResult{}, errors.New("Loki returned an unsuccessful query status")
	}
	return QueryResult{
		Source:    "loki",
		Window:    window.Name,
		QueriedAt: now,
		Query:     query,
		Service:   service,
		Limit:     limit,
		Data:      nonEmptyJSON(payload.Data.Result),
	}, nil
}

func (c *Client) Traces(ctx context.Context, window QueryWindow, service string, limit int) (QueryResult, error) {
	if c == nil || c.tempoURL == "" {
		return QueryResult{}, ErrNotConfigured
	}
	if err := ValidateServiceName(service); err != nil {
		return QueryResult{}, err
	}
	if limit < 1 || limit > maxResultLimit {
		return QueryResult{}, fmt.Errorf("limit must be between 1 and %d", maxResultLimit)
	}

	now := c.now().UTC()
	values := url.Values{}
	values.Set("tags", "service.name="+service)
	values.Set("start", strconv.FormatInt(now.Add(-window.Duration).Unix(), 10))
	values.Set("end", strconv.FormatInt(now.Unix(), 10))
	values.Set("limit", strconv.Itoa(limit))

	payload, err := c.get(ctx, c.tempoURL, "/api/search", values)
	if err != nil {
		return QueryResult{}, fmt.Errorf("query Tempo: %w", err)
	}
	return QueryResult{
		Source:    "tempo",
		Window:    window.Name,
		QueriedAt: now,
		Query:     "service.name=" + service,
		Service:   service,
		Limit:     limit,
		Data:      nonEmptyJSON(payload.Traces),
	}, nil
}

func (c *Client) get(ctx context.Context, baseURL, path string, values url.Values) (upstreamEnvelope, error) {
	target := baseURL + path + "?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return upstreamEnvelope{}, fmt.Errorf("create upstream request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return upstreamEnvelope{}, fmt.Errorf("send upstream request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return upstreamEnvelope{}, fmt.Errorf("read upstream response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return upstreamEnvelope{}, fmt.Errorf("upstream response exceeded %d bytes", maxResponseBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return upstreamEnvelope{}, fmt.Errorf("upstream returned HTTP %d", response.StatusCode)
	}

	var payload upstreamEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return upstreamEnvelope{}, fmt.Errorf("decode upstream JSON: %w", err)
	}
	return payload, nil
}

func nonEmptyJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 || string(value) == "null" {
		return json.RawMessage("[]")
	}
	return value
}

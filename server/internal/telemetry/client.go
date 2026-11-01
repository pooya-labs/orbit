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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxResponseBytes = 2 << 20
	maxResultLimit   = 100
	maxServiceCount  = 250
	metricName       = "orbit_demo_requests_total"
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
	Status    string          `json:"status"`
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
	Traces    json.RawMessage `json:"traces"`
	TagValues []string        `json:"tagValues"`
}

type DiscoveredService struct {
	ID                 string         `json:"id,omitempty"`
	Name               string         `json:"name"`
	Sources            []string       `json:"sources"`
	FirstSeenAt        time.Time      `json:"first_seen_at,omitempty"`
	LastSeenAt         time.Time      `json:"last_seen_at,omitempty"`
	LastDiscoveredAt   time.Time      `json:"last_discovered_at,omitempty"`
	ResourceAttributes map[string]any `json:"resource_attributes,omitempty"`
}

type ServiceDiscoveryResult struct {
	Source        string              `json:"source"`
	Window        string              `json:"window"`
	QueriedAt     time.Time           `json:"queried_at"`
	SourcesQueried []string           `json:"sources_queried"`
	Warnings      []string            `json:"warnings,omitempty"`
	Data          []DiscoveredService `json:"data"`
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

func (c *Client) Metrics(ctx context.Context, window QueryWindow, service string) (QueryResult, error) {
	if c == nil || c.prometheusURL == "" {
		return QueryResult{}, ErrNotConfigured
	}
	if err := ValidateServiceName(service); err != nil {
		return QueryResult{}, err
	}
	query := fmt.Sprintf("sum(rate(%s{service_name=%q}[5m]))", metricName, service)
	now := c.now().UTC()
	values := url.Values{}
	values.Set("query", query)
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
	var result struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(payload.Data, &result); err != nil {
		return QueryResult{}, fmt.Errorf("decode Prometheus range result: %w", err)
	}
	return QueryResult{
		Source:    "prometheus",
		Window:    window.Name,
		QueriedAt: now,
		Query:     query,
		Step:      window.Step,
		Service:   service,
		Data:      nonEmptyJSON(result.Result),
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
	var result struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(payload.Data, &result); err != nil {
		return QueryResult{}, fmt.Errorf("decode Loki range result: %w", err)
	}
	return QueryResult{
		Source:    "loki",
		Window:    window.Name,
		QueriedAt: now,
		Query:     query,
		Service:   service,
		Limit:     limit,
		Data:      nonEmptyJSON(result.Result),
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


func (c *Client) Services(ctx context.Context, window QueryWindow) (ServiceDiscoveryResult, error) {
	if c == nil || (c.prometheusURL == "" && c.lokiURL == "" && c.tempoURL == "") {
		return ServiceDiscoveryResult{}, ErrNotConfigured
	}

	now := c.now().UTC()
	start := now.Add(-window.Duration)
	type backendQuery struct {
		name string
		base string
		path string
		values url.Values
		tempo bool
	}
	queries := make([]backendQuery, 0, 3)
	if c.prometheusURL != "" {
		values := url.Values{}
		values.Set("start", start.Format(time.RFC3339Nano))
		values.Set("end", now.Format(time.RFC3339Nano))
		values.Set("limit", strconv.Itoa(maxServiceCount))
		queries = append(queries, backendQuery{"prometheus", c.prometheusURL, "/api/v1/label/service_name/values", values, false})
	}
	if c.lokiURL != "" {
		values := url.Values{}
		values.Set("start", strconv.FormatInt(start.UnixNano(), 10))
		values.Set("end", strconv.FormatInt(now.UnixNano(), 10))
		queries = append(queries, backendQuery{"loki", c.lokiURL, "/loki/api/v1/label/service_name/values", values, false})
	}
	if c.tempoURL != "" {
		values := url.Values{}
		values.Set("start", strconv.FormatInt(start.Unix(), 10))
		values.Set("end", strconv.FormatInt(now.Unix(), 10))
		values.Set("limit", strconv.Itoa(maxServiceCount))
		queries = append(queries, backendQuery{"tempo", c.tempoURL, "/api/search/tag/service.name/values", values, true})
	}

	type backendResult struct {
		name string
		values []string
		err error
	}
	results := make(chan backendResult, len(queries))
	var group sync.WaitGroup
	for _, query := range queries {
		query := query
		group.Add(1)
		go func() {
			defer group.Done()
			raw, err := c.getRaw(ctx, query.base, query.path, query.values)
			if err != nil {
				results <- backendResult{name: query.name, err: err}
				return
			}
			if query.tempo {
				var payload struct {
					TagValues []string `json:"tagValues"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					results <- backendResult{name: query.name, err: fmt.Errorf("decode Tempo service values: %w", err)}
					return
				}
				results <- backendResult{name: query.name, values: payload.TagValues}
				return
			}
			var payload struct {
				Status string   `json:"status"`
				Error  string   `json:"error"`
				Data   []string `json:"data"`
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				results <- backendResult{name: query.name, err: fmt.Errorf("decode %s service values: %w", query.name, err)}
				return
			}
			if payload.Status != "success" {
				results <- backendResult{name: query.name, err: fmt.Errorf("%s returned an unsuccessful status", query.name)}
				return
			}
			results <- backendResult{name: query.name, values: payload.Data}
		}()
	}
	group.Wait()
	close(results)

	merged := make(map[string]map[string]struct{})
	warnings := make([]string, 0)
	sourcesQueried := make([]string, 0, len(queries))
	successfulBackends := 0
	for result := range results {
		sourcesQueried = append(sourcesQueried, result.name)
		if result.err != nil {
			warnings = append(warnings, result.name+" service discovery unavailable")
			continue
		}
		successfulBackends++
		for _, name := range result.values {
			name = strings.TrimSpace(name)
			if ValidateServiceName(name) != nil {
				continue
			}
			if _, ok := merged[name]; !ok {
				if len(merged) >= maxServiceCount {
					continue
				}
				merged[name] = make(map[string]struct{})
			}
			merged[name][result.name] = struct{}{}
		}
	}
	if successfulBackends == 0 {
		return ServiceDiscoveryResult{}, errors.New("all configured telemetry backends failed service discovery")
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	services := make([]DiscoveredService, 0, len(names))
	for _, name := range names {
		sources := make([]string, 0, len(merged[name]))
		for source := range merged[name] {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		services = append(services, DiscoveredService{Name: name, Sources: sources})
	}
	sort.Strings(sourcesQueried)
	sort.Strings(warnings)

	return ServiceDiscoveryResult{
		Source:         "telemetry_backends",
		Window:         window.Name,
		QueriedAt:      now,
		SourcesQueried: sourcesQueried,
		Warnings:       warnings,
		Data:           services,
	}, nil
}

func (c *Client) get(ctx context.Context, baseURL, path string, values url.Values) (upstreamEnvelope, error) {
	body, err := c.getRaw(ctx, baseURL, path, values)
	if err != nil {
		return upstreamEnvelope{}, err
	}
	var payload upstreamEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return upstreamEnvelope{}, fmt.Errorf("decode upstream JSON: %w", err)
	}
	return payload, nil
}

func (c *Client) getRaw(ctx context.Context, baseURL, path string, values url.Values) (json.RawMessage, error) {
	target := baseURL + path + "?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send upstream request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("upstream response exceeded %d bytes", maxResponseBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("upstream returned HTTP %d", response.StatusCode)
	}
	if !json.Valid(body) {
		return nil, errors.New("upstream returned invalid JSON")
	}
	return json.RawMessage(body), nil
}

func nonEmptyJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 || string(value) == "null" {
		return json.RawMessage("[]")
	}
	return value
}

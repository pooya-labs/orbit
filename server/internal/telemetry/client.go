package telemetry

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
	maxResponseBytes         = 2 << 20
	maxResultLimit           = 100
	maxServiceCount          = 250
	maxMetricSeries          = 10
	maxMetricGroupDimensions = 3
	metricName               = "orbit_demo_requests_total"

	MetricRequestRate          = "request_rate"
	MetricSuccessfulRequestRate = "successful_request_rate"
	MetricFailedRequestRate     = "failed_request_rate"
)

var ErrNotConfigured = errors.New("telemetry backend is not configured")
var ErrWorkspaceTraceNotFound = errors.New("workspace trace not found")
var ErrInvalidTraceID = errors.New("invalid trace ID")

var traceIDPattern = regexp.MustCompile("^[0-9a-fA-F]{32}$")

var sloRoutePattern = regexp.MustCompile("^/[A-Za-z0-9][A-Za-z0-9/_-]{0,127}$")

type AvailabilityEvents struct {
	GoodEvents float64
	BadEvents float64
	WindowStart time.Time
	WindowEnd time.Time
}

func ValidateSLORoute(route string) error {
	if !sloRoutePattern.MatchString(route) || strings.Contains(route, "//") {
		return errors.New("SLO route must be a normalized low-cardinality absolute route path")
	}
	return nil
}

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

var allowedMetricGroupLabels = map[string]struct{}{
	"instance":               {},
	"service_version":        {},
	"deployment_environment": {},
}

type MetricDefinition struct {
	ID          string   `json:"id"`
	Unit        string   `json:"unit"`
	Description string   `json:"description"`
	GroupBy     []string `json:"group_by"`
}

type MetricCatalogResponse struct {
	Data []MetricDefinition `json:"data"`
}

var workspaceMetricCatalog = []MetricDefinition{
	{ID: MetricRequestRate, Unit: "requests/second", Description: "All instrumented HTTP requests per second.", GroupBy: []string{"instance", "service_version", "deployment_environment"}},
	{ID: MetricSuccessfulRequestRate, Unit: "requests/second", Description: "Instrumented successful HTTP 2xx and 3xx requests per second.", GroupBy: []string{"instance", "service_version", "deployment_environment"}},
	{ID: MetricFailedRequestRate, Unit: "requests/second", Description: "Instrumented failed HTTP requests per second.", GroupBy: []string{"instance", "service_version", "deployment_environment"}},
}

// ListMetricCatalog returns the server-owned metric definitions. It deliberately
// does not expose PromQL or discover arbitrary Prometheus metric names.
func ListMetricCatalog() MetricCatalogResponse {
	result := MetricCatalogResponse{Data: make([]MetricDefinition, 0, len(workspaceMetricCatalog))}
	for _, definition := range workspaceMetricCatalog {
		definition.GroupBy = append([]string(nil), definition.GroupBy...)
		result.Data = append(result.Data, definition)
	}
	return result
}

// ParseMetricID accepts only stable IDs defined by the server-owned catalog.
func ParseMetricID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return MetricRequestRate, nil
	}
	switch value {
	case MetricRequestRate, MetricSuccessfulRequestRate, MetricFailedRequestRate:
		return value, nil
	default:
		return "", errors.New("metric must be one of request_rate, successful_request_rate, or failed_request_rate")
	}
}

func buildMetricQuery(metricID string, groupLabels, matchers []string) (string, error) {
	metricID, err := ParseMetricID(metricID)
	if err != nil {
		return "", err
	}
	selector := func(extra string) string {
		parts := append([]string(nil), matchers...)
		if extra != "" {
			parts = append(parts, extra)
		}
		return metricName + "{" + strings.Join(parts, ",") + "}"
	}
	group := strings.Join(groupLabels, ", ")
	switch metricID {
	case MetricRequestRate:
		return fmt.Sprintf("sum by (%s) (rate(%s[5m]))", group, selector("")), nil
	case MetricSuccessfulRequestRate:
		return fmt.Sprintf("sum by (%s) (rate(%s[5m]))", group, selector(`slo_outcome="good"`)), nil
	case MetricFailedRequestRate:
		return fmt.Sprintf("sum by (%s) (rate(%s[5m]))", group, selector(`slo_outcome="bad"`)), nil
	default:
		return "", errors.New("unsupported metric")
	}
}

func ParseMetricGroupBy(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "instance,service_version,deployment_environment"
	}
	if value == "none" {
		return []string{}, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) > maxMetricGroupDimensions {
		return nil, fmt.Errorf("group_by accepts at most %d dimensions", maxMetricGroupDimensions)
	}
	seen := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		label := strings.TrimSpace(part)
		if _, ok := allowedMetricGroupLabels[label]; !ok {
			return nil, fmt.Errorf("group_by contains an unsupported dimension")
		}
		if _, exists := seen[label]; exists {
			continue
		}
		seen[label] = struct{}{}
		result = append(result, label)
	}
	sort.Strings(result)
	return result, nil
}

func ValidateServiceName(value string) error {
	if !serviceNamePattern.MatchString(value) {
		return fmt.Errorf("service must contain only letters, numbers, dots, underscores, colons, or hyphens")
	}
	return nil
}

type TraceDetailResult struct {
	Source    string          `json:"source"`
	TraceID   string          `json:"trace_id"`
	QueriedAt time.Time       `json:"queried_at"`
	Data      json.RawMessage `json:"data"`
}

type tempoTraceBatch struct {
	Resource struct {
		Attributes []struct {
			Key   string `json:"key"`
			Value struct {
				StringValue string `json:"stringValue"`
			} `json:"value"`
		} `json:"attributes"`
	} `json:"resource"`
	ScopeSpans []tempoTraceScopeSpans `json:"scopeSpans"`
}

type tempoTraceScopeSpans struct {
	Spans []tempoTraceSpan `json:"spans"`
}

type tempoTraceSpan struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	ParentSpanID      string          `json:"parentSpanId"`
	Name              string          `json:"name"`
	Kind              json.RawMessage `json:"kind"`
	StartTimeUnixNano string          `json:"startTimeUnixNano"`
	EndTimeUnixNano   string          `json:"endTimeUnixNano"`
	Status            struct {
		Code json.RawMessage `json:"code"`
	} `json:"status"`
}

type tempoTraceEnvelope struct {
	Batches []tempoTraceBatch `json:"batches"`
	Trace   struct {
		ResourceSpans []tempoTraceBatch `json:"resourceSpans"`
		Batches       []tempoTraceBatch `json:"batches"`
	} `json:"trace"`
}

type workspaceTraceDetailData struct {
	Batches []workspaceTraceDetailBatch `json:"batches"`
}

type workspaceTraceDetailBatch struct {
	ScopeSpans []workspaceTraceDetailScopeSpans `json:"scopeSpans"`
}

type workspaceTraceDetailScopeSpans struct {
	Spans []workspaceTraceDetailSpan     `json:"spans"`
}

type workspaceTraceDetailSpan struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	ParentSpanID      string          `json:"parentSpanId,omitempty"`
	Name              string          `json:"name"`
	Kind              json.RawMessage `json:"kind,omitempty"`
	StartTimeUnixNano string          `json:"startTimeUnixNano"`
	EndTimeUnixNano   string          `json:"endTimeUnixNano"`
	Status            struct {
		Code int `json:"code"`
	} `json:"status"`
}

type QueryResult struct {
	Source    string          `json:"source"`
	Window    string          `json:"window"`
	QueriedAt time.Time       `json:"queried_at"`
	Query     string          `json:"query,omitempty"`
	Step      string          `json:"step,omitempty"`
	Service   string          `json:"service,omitempty"`
	Metric    string          `json:"metric,omitempty"`
	GroupBy   []string        `json:"group_by,omitempty"`
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
	ResourceAttributeSources map[string][]string `json:"resource_attribute_sources,omitempty"`
}

type ServiceDiscoveryResult struct {
	Source        string              `json:"source"`
	Window        string              `json:"window"`
	QueriedAt     time.Time           `json:"queried_at"`
	SourcesQueried []string           `json:"sources_queried"`
	Warnings      []string            `json:"warnings,omitempty"`
	NextCursor   string              `json:"next_cursor,omitempty"`
	HasMore      bool                `json:"has_more"`
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

func (c *Client) Metrics(ctx context.Context, window QueryWindow, service, metricID string, groupBy []string) (QueryResult, error) {
	if c == nil || c.prometheusURL == "" {
		return QueryResult{}, ErrNotConfigured
	}
	if err := ValidateServiceName(service); err != nil {
		return QueryResult{}, err
	}
	metricID, err := ParseMetricID(metricID)
	if err != nil { return QueryResult{}, err }
	groupLabels := []string{"service_name"}
	for _, label := range groupBy {
		if _, ok := allowedMetricGroupLabels[label]; !ok {
			return QueryResult{}, fmt.Errorf("unsupported metric grouping dimension")
		}
		groupLabels = append(groupLabels, label)
	}
	sort.Strings(groupLabels)
	query, err := buildMetricQuery(metricID, groupLabels, []string{fmt.Sprintf("service_name=%q", service)})
	if err != nil { return QueryResult{}, err }
	now := c.now().UTC()
	values := url.Values{}
	values.Set("query", query)
	values.Set("start", now.Add(-window.Duration).Format(time.RFC3339Nano))
	values.Set("end", now.Format(time.RFC3339Nano))
	values.Set("step", window.Step)
	values.Set("timeout", "5s")
	values.Set("limit", strconv.Itoa(maxMetricSeries))

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
		GroupBy:   append([]string(nil), groupBy...),
		Metric:    metricID,
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



var workspaceIDPattern = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$")

func validateWorkspaceID(workspaceID string) error {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return errors.New("workspace identifier is invalid")
	}
	return nil
}


// WorkspaceRequestRates returns the latest bounded request-rate sample per
// service for exactly one workspace. It never accepts caller-supplied PromQL.
func (c *Client) WorkspaceRequestRates(
	ctx context.Context,
	workspaceID, service string,
	windowSeconds int,
) (map[string]float64, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if c == nil || c.prometheusURL == "" {
		return nil, ErrNotConfigured
	}
	if service != "" {
		if err := ValidateServiceName(service); err != nil {
			return nil, err
		}
	}
	switch windowSeconds {
	case 60, 300, 900:
	default:
		return nil, errors.New("alert evaluation window must be 60, 300, or 900 seconds")
	}

	selector := "orbit_workspace_id=" + strconv.Quote(workspaceID)
	if service != "" {
		selector += ",service_name=" + strconv.Quote(service)
	}
	query := fmt.Sprintf("sum by (service_name) (rate(%s{%s}[%ds]))", metricName, selector, windowSeconds)
	now := c.now().UTC()
	values := url.Values{}
	values.Set("query", query)
	values.Set("time", now.Format(time.RFC3339Nano))
	values.Set("timeout", "5s")

	payload, err := c.get(ctx, c.prometheusURL, "/api/v1/query", values)
	if err != nil {
		return nil, fmt.Errorf("query workspace request rate: %w", err)
	}
	if payload.Status != "success" {
		return nil, errors.New("Prometheus returned an unsuccessful alert-evaluation query")
	}
	var result struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload.Data, &result); err != nil {
		return nil, fmt.Errorf("decode workspace request-rate result: %w", err)
	}
	samples := make(map[string]float64, len(result.Result))
	for _, item := range result.Result {
		serviceName := item.Metric["service_name"]
		if serviceName == "" || ValidateServiceName(serviceName) != nil || len(item.Value) != 2 {
			continue
		}
		var rawValue string
		if err := json.Unmarshal(item.Value[1], &rawValue); err != nil {
			return nil, fmt.Errorf("decode request-rate sample for %q: %w", serviceName, err)
		}
		value, err := strconv.ParseFloat(rawValue, 64)
		if err != nil {
			return nil, fmt.Errorf("parse request-rate sample for %q: %w", serviceName, err)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			continue
		}
		samples[serviceName] = value
	}
	return samples, nil
}

// WorkspaceAvailabilityEvents reads the fixed allow-listed request counter.
// Only requests instrumented with slo_outcome=good|bad and a canonical
// workspace label are included. No caller-supplied PromQL is accepted.
func (c *Client) WorkspaceAvailabilityEvents(
	ctx context.Context,
	workspaceID, service, route string,
	windowDays int,
) (AvailabilityEvents, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return AvailabilityEvents{}, err
	}
	if c == nil || c.prometheusURL == "" {
		return AvailabilityEvents{}, ErrNotConfigured
	}
	if err := ValidateServiceName(service); err != nil {
		return AvailabilityEvents{}, err
	}
	if err := ValidateSLORoute(route); err != nil {
		return AvailabilityEvents{}, err
	}
	switch windowDays {
	case 1, 3, 7:
	default:
		return AvailabilityEvents{}, errors.New("SLO window must be 1, 3, or 7 days")
	}

	window := fmt.Sprintf("%dd", windowDays)
	selector := strings.Join([]string{
		"orbit_workspace_id=" + strconv.Quote(workspaceID),
		"service_name=" + strconv.Quote(service),
		"slo_route=" + strconv.Quote(route),
		`slo_outcome=~"good|bad"`,
	}, ",")
	query := fmt.Sprintf("sum by (slo_outcome) (increase(%s{%s}[%s]))", metricName, selector, window)
	now := c.now().UTC()
	values := url.Values{}
	values.Set("query", query)
	values.Set("time", now.Format(time.RFC3339Nano))
	values.Set("timeout", "5s")
	payload, err := c.get(ctx, c.prometheusURL, "/api/v1/query", values)
	if err != nil {
		return AvailabilityEvents{}, fmt.Errorf("query workspace availability events: %w", err)
	}
	if payload.Status != "success" {
		return AvailabilityEvents{}, errors.New("Prometheus returned an unsuccessful SLO query status")
	}
	var result struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value []json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload.Data, &result); err != nil {
		return AvailabilityEvents{}, fmt.Errorf("decode workspace availability result: %w", err)
	}
	events := AvailabilityEvents{WindowStart: now.Add(-time.Duration(windowDays) * 24 * time.Hour), WindowEnd: now}
	for _, item := range result.Result {
		outcome := item.Metric["slo_outcome"]
		if (outcome != "good" && outcome != "bad") || len(item.Value) != 2 {
			continue
		}
		var rawValue string
		if err := json.Unmarshal(item.Value[1], &rawValue); err != nil {
			return AvailabilityEvents{}, fmt.Errorf("decode %s availability counter: %w", outcome, err)
		}
		value, err := strconv.ParseFloat(rawValue, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return AvailabilityEvents{}, fmt.Errorf("Prometheus returned an invalid %s availability count", outcome)
		}
		if outcome == "good" {
			events.GoodEvents += value
		} else {
			events.BadEvents += value
		}
	}
	return events, nil
}


func (c *Client) WorkspaceMetrics(ctx context.Context, workspaceID string, window QueryWindow, service, metricID string, groupBy []string) (QueryResult, error) {
	if err := validateWorkspaceID(workspaceID); err != nil { return QueryResult{}, err }
	if c == nil || c.prometheusURL == "" { return QueryResult{}, ErrNotConfigured }
	if err := ValidateServiceName(service); err != nil { return QueryResult{}, err }
	metricID, err := ParseMetricID(metricID)
	if err != nil { return QueryResult{}, err }
	groupLabels := []string{"service_name", "orbit_workspace_id"}
	for _, label := range groupBy {
		if _, ok := allowedMetricGroupLabels[label]; !ok { return QueryResult{}, errors.New("unsupported metric grouping dimension") }
		groupLabels = append(groupLabels, label)
	}
	sort.Strings(groupLabels)
	query, err := buildMetricQuery(metricID, groupLabels, []string{
		fmt.Sprintf("service_name=%q", service),
		fmt.Sprintf("orbit_workspace_id=%q", workspaceID),
	})
	if err != nil { return QueryResult{}, err }
	now := c.now().UTC()
	values := url.Values{}
	values.Set("query", query)
	values.Set("start", now.Add(-window.Duration).Format(time.RFC3339Nano))
	values.Set("end", now.Format(time.RFC3339Nano))
	values.Set("step", window.Step)
	values.Set("timeout", "5s")
	values.Set("limit", strconv.Itoa(maxMetricSeries))
	payload, err := c.get(ctx, c.prometheusURL, "/api/v1/query_range", values)
	if err != nil { return QueryResult{}, fmt.Errorf("query workspace Prometheus: %w", err) }
	if payload.Status != "success" { return QueryResult{}, errors.New("Prometheus returned an unsuccessful query status") }
	var result map[string]json.RawMessage
	if err := json.Unmarshal(payload.Data, &result); err != nil { return QueryResult{}, fmt.Errorf("decode Prometheus range result: %w", err) }
	return QueryResult{Source:"prometheus", Window:window.Name, QueriedAt:now, Query:query, Step:window.Step, Service:service, Metric:metricID, GroupBy:append([]string(nil),groupBy...), Data:nonEmptyJSON(result["result"])}, nil
}

func (c *Client) WorkspaceLogs(ctx context.Context, workspaceID string, window QueryWindow, service string, limit int) (QueryResult, error) {
	if err := validateWorkspaceID(workspaceID); err != nil { return QueryResult{}, err }
	if c == nil || c.lokiURL == "" { return QueryResult{}, ErrNotConfigured }
	if err := ValidateServiceName(service); err != nil { return QueryResult{}, err }
	if limit < 1 || limit > maxResultLimit { return QueryResult{}, fmt.Errorf("limit must be between 1 and %d", maxResultLimit) }
	now := c.now().UTC()
	query := fmt.Sprintf("{orbit_workspace_id=%q,service_name=%q}", workspaceID, service)
	values := url.Values{}
	values.Set("query", query)
	values.Set("start", strconv.FormatInt(now.Add(-window.Duration).UnixNano(),10))
	values.Set("end", strconv.FormatInt(now.UnixNano(),10))
	values.Set("limit",strconv.Itoa(limit))
	values.Set("direction","backward")
	payload,err:=c.get(ctx,c.lokiURL,"/loki/api/v1/query_range",values)
	if err!=nil{return QueryResult{},fmt.Errorf("query workspace Loki: %w",err)}
	if payload.Status!="success"{return QueryResult{},errors.New("Loki returned an unsuccessful query status")}
	var result map[string]json.RawMessage
	if err:=json.Unmarshal(payload.Data,&result);err!=nil{return QueryResult{},fmt.Errorf("decode Loki range result: %w",err)}
	return QueryResult{Source:"loki",Window:window.Name,QueriedAt:now,Query:query,Service:service,Limit:limit,Data:nonEmptyJSON(result["result"])},nil
}

func (c *Client) WorkspaceTraces(ctx context.Context, workspaceID string, window QueryWindow, service string, limit int) (QueryResult, error) {
	if err:=validateWorkspaceID(workspaceID);err!=nil{return QueryResult{},err}
	if c==nil||c.tempoURL==""{return QueryResult{},ErrNotConfigured}
	if err:=ValidateServiceName(service);err!=nil{return QueryResult{},err}
	if limit<1||limit>maxResultLimit{return QueryResult{},fmt.Errorf("limit must be between 1 and %d",maxResultLimit)}
	now:=c.now().UTC()
	traceQL:=fmt.Sprintf("{ resource.service.name = %q && resource.orbit.workspace.id = %q }",service,workspaceID)
	values:=url.Values{}
	values.Set("q",traceQL)
	values.Set("start",strconv.FormatInt(now.Add(-window.Duration).Unix(),10))
	values.Set("end",strconv.FormatInt(now.Unix(),10))
	values.Set("limit",strconv.Itoa(limit))
	payload,err:=c.get(ctx,c.tempoURL,"/api/search",values)
	if err!=nil{return QueryResult{},fmt.Errorf("query workspace Tempo: %w",err)}
	return QueryResult{Source:"tempo",Window:window.Name,QueriedAt:now,Query:traceQL,Service:service,Limit:limit,Data:nonEmptyJSON(payload.Traces)},nil
}

// WorkspaceTrace retrieves one Tempo trace only after the upstream response proves
// that every returned resource batch belongs to the requested workspace. Tempo's
// trace-ID lookup is not itself tenant-scoped, so this check must fail closed.
func (c *Client) WorkspaceTrace(ctx context.Context, workspaceID, traceID string) (TraceDetailResult, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return TraceDetailResult{}, err
	}
	normalizedTraceID := strings.ToLower(strings.TrimSpace(traceID))
	if !traceIDPattern.MatchString(normalizedTraceID) || normalizedTraceID == strings.Repeat("0", 32) {
		return TraceDetailResult{}, ErrInvalidTraceID
	}
	if c == nil || c.tempoURL == "" {
		return TraceDetailResult{}, ErrNotConfigured
	}

	now := c.now().UTC()
	body, err := c.getRaw(ctx, c.tempoURL, "/api/v2/traces/"+normalizedTraceID, nil)
	if err != nil {
		var statusErr *upstreamHTTPError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			return TraceDetailResult{}, ErrWorkspaceTraceNotFound
		}
		return TraceDetailResult{}, fmt.Errorf("read workspace trace from Tempo: %w", err)
	}

	var response tempoTraceEnvelope
	if err := json.Unmarshal(body, &response); err != nil {
		return TraceDetailResult{}, fmt.Errorf("decode Tempo trace detail: %w", err)
	}
	batches := response.Trace.ResourceSpans
	if len(batches) == 0 {
		batches = response.Trace.Batches
	}
	if len(batches) == 0 {
		batches = response.Batches
	}
	if len(batches) == 0 {
		return TraceDetailResult{}, ErrWorkspaceTraceNotFound
	}

	normalized := workspaceTraceDetailData{Batches: make([]workspaceTraceDetailBatch, 0, len(batches))}
	spanCount := 0
	for _, batch := range batches {
		batchWorkspaceID := ""
		workspaceAttributeSeen := false
		for _, attribute := range batch.Resource.Attributes {
			if attribute.Key != "orbit.workspace.id" {
				continue
			}
			value := strings.TrimSpace(attribute.Value.StringValue)
			if workspaceAttributeSeen && !strings.EqualFold(batchWorkspaceID, value) {
				return TraceDetailResult{}, ErrWorkspaceTraceNotFound
			}
			batchWorkspaceID = value
			workspaceAttributeSeen = true
		}
		if batchWorkspaceID == "" || !strings.EqualFold(batchWorkspaceID, workspaceID) {
			return TraceDetailResult{}, ErrWorkspaceTraceNotFound
		}

		normalizedBatch := workspaceTraceDetailBatch{
			ScopeSpans: make([]workspaceTraceDetailScopeSpans, 0, len(batch.ScopeSpans)),
		}
		for _, scope := range batch.ScopeSpans {
			normalizedScope := workspaceTraceDetailScopeSpans{
				Spans: make([]workspaceTraceDetailSpan, 0, len(scope.Spans)),
			}
			for _, span := range scope.Spans {
				spanTraceID, ok := normalizeTempoIdentifier(span.TraceID, 16)
				if !ok || spanTraceID != normalizedTraceID {
					return TraceDetailResult{}, ErrWorkspaceTraceNotFound
				}
				spanID, ok := normalizeTempoIdentifier(span.SpanID, 8)
				if !ok {
					return TraceDetailResult{}, ErrWorkspaceTraceNotFound
				}
				parentSpanID := ""
				if strings.TrimSpace(span.ParentSpanID) != "" {
					parentSpanID, ok = normalizeTempoIdentifier(span.ParentSpanID, 8)
					if !ok {
						return TraceDetailResult{}, ErrWorkspaceTraceNotFound
					}
				}

				normalizedSpan := workspaceTraceDetailSpan{
					TraceID:           spanTraceID,
					SpanID:            spanID,
					ParentSpanID:      parentSpanID,
					Name:              span.Name,
					Kind:              span.Kind,
					StartTimeUnixNano: span.StartTimeUnixNano,
					EndTimeUnixNano:   span.EndTimeUnixNano,
				}
				normalizedSpan.Status.Code = normalizeTempoStatusCode(span.Status.Code)
				normalizedScope.Spans = append(normalizedScope.Spans, normalizedSpan)
				spanCount++
			}
			normalizedBatch.ScopeSpans = append(normalizedBatch.ScopeSpans, normalizedScope)
		}
		normalized.Batches = append(normalized.Batches, normalizedBatch)
	}
	if spanCount == 0 {
		return TraceDetailResult{}, ErrWorkspaceTraceNotFound
	}

	normalizedBody, err := json.Marshal(normalized)
	if err != nil {
		return TraceDetailResult{}, fmt.Errorf("encode workspace trace detail: %w", err)
	}
	return TraceDetailResult{
		Source:    "tempo",
		TraceID:   normalizedTraceID,
		QueriedAt: now,
		Data:      normalizedBody,
	}, nil
}

func normalizeTempoIdentifier(value string, expectedBytes int) (string, bool) {
	value = strings.TrimSpace(value)
	if expectedBytes <= 0 || value == "" {
		return "", false
	}
	if len(value) == expectedBytes*2 {
		decoded, err := hex.DecodeString(value)
		if err == nil && len(decoded) == expectedBytes {
			return hex.EncodeToString(decoded), true
		}
	}
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil && len(decoded) == expectedBytes {
			return hex.EncodeToString(decoded), true
		}
	}
	return "", false
}

func normalizeTempoStatusCode(value json.RawMessage) int {
	if len(value) == 0 {
		return 0
	}
	var numeric int
	if err := json.Unmarshal(value, &numeric); err == nil {
		if numeric >= 0 && numeric <= 2 {
			return numeric
		}
		return 0
	}
	var name string
	if err := json.Unmarshal(value, &name); err != nil {
		return 0
	}
	switch strings.ToUpper(name) {
	case "STATUS_CODE_OK", "OK":
		return 1
	case "STATUS_CODE_ERROR", "ERROR":
		return 2
	default:
		return 0
	}
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


const (
	maxResourceAttributeCount  = 16
	maxResourceAttributeValues = 8
	maxTempoAttributeWorkers   = 8
)

type ResourceAttributesResult struct {
	Service          string              `json:"service"`
	Window           string              `json:"window"`
	QueriedAt        time.Time           `json:"queried_at"`
	SourcesQueried   []string            `json:"sources_queried"`
	Warnings         []string            `json:"warnings,omitempty"`
	Attributes       map[string]any     `json:"attributes"`
	AttributeSources map[string][]string `json:"attribute_sources"`
}

type attributeBackendResult struct {
	name       string
	observed   bool
	attributes map[string][]any
	warnings   []string
	err        error
}

type tempoTagValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

func (c *Client) DiscoverResourceAttributes(ctx context.Context, window QueryWindow, service string) (ResourceAttributesResult, error) {
	if c == nil || (c.prometheusURL == "" && c.lokiURL == "" && c.tempoURL == "") {
		return ResourceAttributesResult{}, ErrNotConfigured
	}
	if err := ValidateServiceName(service); err != nil {
		return ResourceAttributesResult{}, err
	}

	now := c.now().UTC()
	results := make(chan attributeBackendResult, 3)
	configured := 0
	if c.prometheusURL != "" {
		configured++
		go func() { results <- c.prometheusResourceAttributes(ctx, window, service) }()
	}
	if c.lokiURL != "" {
		configured++
		go func() { results <- c.lokiResourceAttributes(ctx, window, service) }()
	}
	if c.tempoURL != "" {
		configured++
		go func() { results <- c.tempoResourceAttributes(ctx, window, service) }()
	}

	attributes := make(map[string][]any)
	sources := make(map[string]map[string]struct{})
	warnings := make([]string, 0)
	sourcesQueried := make([]string, 0, configured)
	successful := 0
	observedSources := make([]string, 0, configured)
	for range configured {
		result := <-results
		sourcesQueried = append(sourcesQueried, result.name)
		if result.err != nil {
			warnings = append(warnings, result.name+" resource attribute query unavailable")
			continue
		}
		successful++
		if result.observed {
			observedSources = append(observedSources, result.name)
		}
		warnings = append(warnings, result.warnings...)
		for key, values := range result.attributes {
			for _, value := range values {
				mergeResourceAttribute(attributes, sources, key, value, result.name)
			}
		}
	}
	if successful == 0 {
		return ResourceAttributesResult{}, errors.New("all configured telemetry backends failed resource attribute discovery")
	}

	// service.name is authoritative from the selector itself. Record only
	// backends that successfully answered the service-scoped metadata query.
	for _, source := range observedSources {
		mergeResourceAttribute(attributes, sources, "service.name", service, source)
	}
	orderedAttributes := make(map[string]any, len(attributes))
	attributeSources := make(map[string][]string, len(sources))
	for key, values := range attributes {
		sort.Slice(values, func(i, j int) bool {
			left, _ := json.Marshal(values[i])
			right, _ := json.Marshal(values[j])
			return string(left) < string(right)
		})
		if len(values) == 1 {
			orderedAttributes[key] = values[0]
		} else {
			orderedAttributes[key] = values
		}
		for source := range sources[key] {
			attributeSources[key] = append(attributeSources[key], source)
		}
		sort.Strings(attributeSources[key])
	}
	sort.Strings(sourcesQueried)
	sort.Strings(warnings)
	return ResourceAttributesResult{
		Service: service, Window: window.Name, QueriedAt: now,
		SourcesQueried: sourcesQueried, Warnings: warnings,
		Attributes: orderedAttributes, AttributeSources: attributeSources,
	}, nil
}

func (c *Client) prometheusResourceAttributes(ctx context.Context, window QueryWindow, service string) attributeBackendResult {
	result := attributeBackendResult{name: "prometheus", attributes: make(map[string][]any)}
	now := c.now().UTC()
	values := url.Values{}
	values.Add("match[]", fmt.Sprintf("{service_name=%q}", service))
	values.Set("start", now.Add(-window.Duration).Format(time.RFC3339Nano))
	values.Set("end", now.Format(time.RFC3339Nano))
	values.Set("limit", "100")
	raw, err := c.getRaw(ctx, c.prometheusURL, "/api/v1/series", values)
	if err != nil {
		result.err = fmt.Errorf("query Prometheus series metadata: %w", err)
		return result
	}
	var payload struct {
		Status string              `json:"status"`
		Data   []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Status != "success" {
		result.err = errors.New("decode Prometheus series metadata")
		return result
	}
	for _, series := range payload.Data {
		if series["service_name"] != service {
			continue
		}
		result.observed = true
		for key, value := range series {
			if normalized := normalizePromResourceLabel(key); normalized != "" {
				mergeResourceAttribute(result.attributes, nil, normalized, value, "")
			}
		}
	}
	return result
}

func (c *Client) lokiResourceAttributes(ctx context.Context, window QueryWindow, service string) attributeBackendResult {
	result := attributeBackendResult{name: "loki", attributes: make(map[string][]any)}
	now := c.now().UTC()
	values := url.Values{}
	values.Add("match[]", fmt.Sprintf("{service_name=%q}", service))
	values.Set("start", strconv.FormatInt(now.Add(-window.Duration).UnixNano(), 10))
	values.Set("end", strconv.FormatInt(now.UnixNano(), 10))
	raw, err := c.getRaw(ctx, c.lokiURL, "/loki/api/v1/series", values)
	if err != nil {
		result.err = fmt.Errorf("query Loki series metadata: %w", err)
		return result
	}
	var payload struct {
		Status string              `json:"status"`
		Data   []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Status != "success" {
		result.err = errors.New("decode Loki series metadata")
		return result
	}
	for _, series := range payload.Data {
		if series["service_name"] != service {
			continue
		}
		result.observed = true
		for key, value := range series {
			if normalized := normalizeLokiResourceLabel(key); normalized != "" {
				mergeResourceAttribute(result.attributes, nil, normalized, value, "")
			}
		}
	}
	return result
}

func (c *Client) tempoResourceAttributes(ctx context.Context, window QueryWindow, service string) attributeBackendResult {
	result := attributeBackendResult{name: "tempo", attributes: make(map[string][]any)}
	now := c.now().UTC()
	query := fmt.Sprintf("{resource.service.name=%q}", service)
	values := url.Values{}
	values.Set("scope", "resource")
	values.Set("q", query)
	values.Set("start", strconv.FormatInt(now.Add(-window.Duration).Unix(), 10))
	values.Set("end", strconv.FormatInt(now.Unix(), 10))
	values.Set("limit", strconv.Itoa(maxResourceAttributeCount))
	raw, err := c.getRaw(ctx, c.tempoURL, "/api/v2/search/tags", values)
	if err != nil {
		result.err = fmt.Errorf("query Tempo resource tag names: %w", err)
		return result
	}
	var payload struct {
		Scopes []struct {
			Name string   `json:"name"`
			Tags []string `json:"tags"`
		} `json:"scopes"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		result.err = fmt.Errorf("decode Tempo resource tag names: %w", err)
		return result
	}
	tagNames := make([]string, 0, maxResourceAttributeCount)
	for _, scope := range payload.Scopes {
		if scope.Name != "resource" {
			continue
		}
		result.observed = true
		for _, tag := range scope.Tags {
			if tag == "service.name" || !resourceTagNamePattern.MatchString(tag) {
				continue
			}
			tagNames = append(tagNames, tag)
			if len(tagNames) == maxResourceAttributeCount {
				break
			}
		}
	}
	sort.Strings(tagNames)
	if len(tagNames) == maxResourceAttributeCount {
		result.warnings = append(result.warnings, "tempo resource attributes capped at 16 keys")
	}
	if len(tagNames) == 0 {
		return result
	}

	type tagResult struct {
		name   string
		values []any
		err    error
	}
	tasks := make(chan string)
	tagResults := make(chan tagResult, len(tagNames))
	workerCount := min(maxTempoAttributeWorkers, len(tagNames))
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for tag := range tasks {
				tagQuery := url.Values{}
				tagQuery.Set("q", query)
				tagQuery.Set("start", strconv.FormatInt(now.Add(-window.Duration).Unix(), 10))
				tagQuery.Set("end", strconv.FormatInt(now.Unix(), 10))
				tagQuery.Set("limit", strconv.Itoa(maxResourceAttributeValues))
				endpoint := "/api/v2/search/tag/" + url.PathEscape("."+tag) + "/values"
				body, requestErr := c.getRaw(ctx, c.tempoURL, endpoint, tagQuery)
				if requestErr != nil {
					tagResults <- tagResult{name: tag, err: requestErr}
					continue
				}
				var response struct {
					TagValues []tempoTagValue `json:"tagValues"`
				}
				if decodeErr := json.Unmarshal(body, &response); decodeErr != nil {
					tagResults <- tagResult{name: tag, err: decodeErr}
					continue
				}
				decoded := make([]any, 0, len(response.TagValues))
				for _, item := range response.TagValues {
					var value any
					if len(item.Value) == 0 || string(item.Value) == "null" {
						continue
					}
					if err := json.Unmarshal(item.Value, &value); err != nil {
						continue
					}
					decoded = append(decoded, value)
				}
				tagResults <- tagResult{name: tag, values: decoded}
			}
		}()
	}
	go func() {
		for _, tag := range tagNames {
			tasks <- tag
		}
		close(tasks)
	}()
	workers.Wait()
	close(tagResults)

	for item := range tagResults {
		if item.err != nil {
			result.warnings = append(result.warnings, "tempo resource attribute value unavailable for "+item.name)
			continue
		}
		for _, value := range item.values {
			mergeResourceAttribute(result.attributes, nil, item.name, value, "")
		}
	}
	return result
}

var resourceTagNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

func normalizePromResourceLabel(name string) string {
	switch name {
	case "__name__", "job", "instance":
		return ""
	case "service_name":
		return "service.name"
	case "service_version":
		return "service.version"
	case "deployment_environment":
		return "deployment.environment"
	case "host_name":
		return "host.name"
	case "cloud_provider":
		return "cloud.provider"
	case "cloud_region":
		return "cloud.region"
	case "cloud_availability_zone":
		return "cloud.availability_zone"
	case "k8s_namespace_name":
		return "k8s.namespace.name"
	case "k8s_pod_name":
		return "k8s.pod.name"
	case "container_name":
		return "container.name"
	default:
		return ""
	}
}

func normalizeLokiResourceLabel(name string) string {
	if normalized := normalizePromResourceLabel(name); normalized != "" {
		return normalized
	}
	switch name {
	case "job", "instance", "filename", "level":
		return "loki.labels." + name
	}
	return ""
}

func mergeResourceAttribute(
	attributes map[string][]any,
	sources map[string]map[string]struct{},
	key string,
	value any,
	source string,
) {
	if key == "" || key == "service.name" && value == nil {
		return
	}
	if len(attributes) >= maxResourceAttributeCount {
		if _, exists := attributes[key]; !exists {
			return
		}
	}
	values := attributes[key]
	for _, existing := range values {
		left, _ := json.Marshal(existing)
		right, _ := json.Marshal(value)
		if string(left) == string(right) {
			goto sourceMerge
		}
	}
	if len(values) < maxResourceAttributeValues {
		attributes[key] = append(values, value)
	}
sourceMerge:
	if sources != nil && source != "" {
		if sources[key] == nil {
			sources[key] = make(map[string]struct{})
		}
		sources[key][source] = struct{}{}
	}
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

type upstreamHTTPError struct {
	StatusCode int
}

func (e *upstreamHTTPError) Error() string {
	return fmt.Sprintf("upstream returned HTTP %d", e.StatusCode)
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
		return nil, &upstreamHTTPError{StatusCode: response.StatusCode}
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

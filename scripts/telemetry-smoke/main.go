package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	demoURL       = "http://telemetry-demo:8081"
	prometheusURL = "http://prometheus:9090"
	lokiURL       = "http://loki:3100"
	tempoURL      = "http://tempo:3200"
	orbitAPIURL   = "http://api:8080"
)

var client = &http.Client{Timeout: 5 * time.Second}
type payload map[string]any

func getJSON(rawURL string) (payload, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil { return nil, err }
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected HTTP status %d from %s", resp.StatusCode, rawURL)
	}
	var body payload
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode JSON from %s: %w", rawURL, err)
	}
	return body, nil
}

func getStatus(rawURL string) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil { return err }
	resp, err := client.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected HTTP status %d from %s", resp.StatusCode, rawURL)
	}
	return nil
}

func object(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}
func array(value any) []any {
	result, _ := value.([]any)
	return result
}
func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func traceIDsFromLogs(body payload) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, rawStream := range array(body["data"]) {
		for _, rawPair := range array(object(rawStream)["values"]) {
			pair := array(rawPair)
			if len(pair) < 2 { continue }
			for _, field := range strings.Fields(stringValue(pair[1])) {
				key, candidate, ok := strings.Cut(strings.Trim(field, ",;"), "=")
				if !ok || strings.ToLower(strings.TrimSpace(key)) != "trace_id" { continue }
				id := strings.ToLower(strings.Trim(candidate, ",;"))
				decoded, err := hex.DecodeString(id)
				if err == nil && len(decoded) == 16 && strings.Trim(id, "0") != "" {
					ids[id] = struct{}{}
				}
			}
		}
	}
	return ids
}

func hasData(body payload) bool {
	return len(array(object(body["data"])["result"])) > 0
}
func queryURL(base, path string, values url.Values) string {
	return base + path + "?" + values.Encode()
}

func waitFor(label string, timeout time.Duration, probe func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	lastError := errors.New("data not visible yet")
	for time.Now().Before(deadline) {
		ok, err := probe()
		if err == nil && ok {
			fmt.Printf("PASS: %s\n", label)
			return nil
		}
		if err != nil { lastError = err }
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for %s: %w", label, lastError)
}

func waitReady() error {
	probes := []struct{ name, url string }{
		{"Prometheus", prometheusURL + "/-/ready"},
		{"Loki", lokiURL + "/ready"},
		{"Tempo", tempoURL + "/ready"},
		{"Collector", "http://otel-collector:13133/"},
		{"demo", demoURL + "/healthz"},
		{"ORBIT API", orbitAPIURL + "/health/ready"},
	}
	pending := make(map[string]string, len(probes))
	for _, probe := range probes { pending[probe.name] = probe.url }
	deadline := time.Now().Add(90 * time.Second)
	for len(pending) > 0 && time.Now().Before(deadline) {
		for name, endpoint := range pending {
			if err := getStatus(endpoint); err == nil {
				fmt.Printf("READY: %s\n", name)
				delete(pending, name)
			}
		}
		if len(pending) > 0 { time.Sleep(2 * time.Second) }
	}
	if len(pending) > 0 {
		names := make([]string, 0, len(pending))
		for name := range pending { names = append(names, name) }
		return fmt.Errorf("timed out waiting for: %s", strings.Join(names, ", "))
	}
	return nil
}

func checkPrometheusScrape() error {
	endpoint := queryURL(prometheusURL, "/api/v1/query", url.Values{
		"query": {`up{job="orbit-otel-collector"}`},
	})
	return waitFor("Prometheus scrapes the ORBIT Collector over the private network", 60*time.Second, func() (bool, error) {
		body, err := getJSON(endpoint)
		if err != nil { return false, err }
		if stringValue(body["status"]) != "success" { return false, errors.New("Prometheus query did not return success") }
		rows := array(object(body["data"])["result"])
		if len(rows) == 0 { return false, nil }
		value := array(object(rows[0])["value"])
		return len(value) > 1 && stringValue(value[1]) == "1", nil
	})
}

func smoke() error {
	var emitted payload
	for i := 0; i < 3; i++ {
		body, err := getJSON(demoURL + "/emit")
		if err != nil { return err }
		emitted = body
		time.Sleep(500 * time.Millisecond)
	}
	traceID := strings.ToLower(stringValue(emitted["trace_id"]))
	if stringValue(emitted["status"]) != "accepted" || len(traceID) != 32 {
		return fmt.Errorf("demo service returned an invalid trace identifier: %v", emitted)
	}
	if _, err := hex.DecodeString(traceID); err != nil {
		return fmt.Errorf("demo service returned a malformed trace identifier: %s", traceID)
	}
	fmt.Printf("Submitted OTLP smoke payload; trace_id=%s\n", traceID)

	// Exercise the production API itself after it has been started with the optional
	// private-Collector endpoint. The API emits no IDs, query strings, or auth data.
	if _, err := getJSON(orbitAPIURL + "/api/v1/meta"); err != nil {
		return fmt.Errorf("request ORBIT API metadata for self-telemetry smoke: %w", err)
	}
	apiMetricURL := queryURL(prometheusURL, "/api/v1/query", url.Values{
		"query": {`orbit_api_http_requests_total{service_name="orbit-api"}`},
	})
	if err := waitFor("ORBIT API request metrics are queryable in Prometheus", 120*time.Second, func() (bool, error) {
		body, err := getJSON(apiMetricURL)
		if err != nil { return false, err }
		return stringValue(body["status"]) == "success" && hasData(body), nil
	}); err != nil { return err }

	apiLogURL := queryURL(lokiURL, "/loki/api/v1/query_range", url.Values{
		"query": {`{service_name="orbit-api"} |= "orbit_api_runtime_started"`},
		"limit": {"10"}, "direction": {"backward"},
		"start": {fmt.Sprint(time.Now().Add(-10*time.Minute).UnixNano())},
		"end": {fmt.Sprint(time.Now().Add(time.Minute).UnixNano())},
	})
	for _, worker := range []string{"alert_evaluator", "notification_delivery"} {
		workerMetricURL := queryURL(prometheusURL, "/api/v1/query", url.Values{
			"query": {fmt.Sprintf("orbit_worker_runs_total{service_name=\"orbit-api\",worker_name=%q}", worker)},
		})
		if err := waitFor("ORBIT "+worker+" telemetry is queryable in Prometheus", 120*time.Second, func() (bool, error) {
			body, err := getJSON(workerMetricURL)
			if err != nil {
				return false, err
			}
			return stringValue(body["status"]) == "success" && hasData(body), nil
		}); err != nil {
			return err
		}
	}

	if err := waitFor("ORBIT API lifecycle log is queryable in Loki", 120*time.Second, func() (bool, error) {
		body, err := getJSON(apiLogURL)
		if err != nil { return false, err }
		return stringValue(body["status"]) == "success" && hasData(body), nil
	}); err != nil { return err }

	if err := waitFor("ORBIT API request traces are queryable through Tempo", 120*time.Second, func() (bool, error) {
		body, err := getJSON(orbitAPIURL + "/api/v1/telemetry/traces?window=1h&service=orbit-api&limit=10")
		if err != nil { return false, err }
		return stringValue(body["source"]) == "tempo" && len(array(body["data"])) > 0, nil
	}); err != nil { return err }

	metricURL := queryURL(prometheusURL, "/api/v1/query", url.Values{"query": {"orbit_demo_requests_total"}})
	if err := waitFor("OTLP metric is queryable in Prometheus", 120*time.Second, func() (bool, error) {
		body, err := getJSON(metricURL)
		if err != nil { return false, err }
		return stringValue(body["status"]) == "success" && hasData(body), nil
	}); err != nil { return err }

	now := time.Now()
	logURL := queryURL(lokiURL, "/loki/api/v1/query_range", url.Values{
		"query": {`{service_name="orbit-telemetry-demo"} |= "orbit-telemetry-smoke"`},
		"limit": {"5"}, "direction": {"backward"},
		"start": {fmt.Sprint(now.Add(-120*time.Second).UnixNano())},
		"end": {fmt.Sprint(now.Add(10*time.Second).UnixNano())},
	})
	if err := waitFor("OTLP log is queryable in Loki", 120*time.Second, func() (bool, error) {
		body, err := getJSON(logURL)
		if err != nil { return false, err }
		return stringValue(body["status"]) == "success" && hasData(body), nil
	}); err != nil { return err }

	if err := waitFor("OTLP trace is queryable in Tempo", 120*time.Second, func() (bool, error) {
		body, err := getJSON(tempoURL + "/api/traces/" + traceID)
		if err != nil { return false, err }
		return len(body) > 0, nil
	}); err != nil { return err }
	fmt.Println("PASS: all three OTLP signals reached their local backends")

	if err := waitFor("ORBIT API discovers services across all three backends", 120*time.Second, func() (bool, error) {
		body, err := getJSON(orbitAPIURL + "/api/v1/telemetry/services?window=1h")
		if err != nil { return false, err }
		if stringValue(body["source"]) != "telemetry_backends" { return false, nil }
		for _, raw := range array(body["data"]) {
			item := object(raw)
			if stringValue(item["name"]) != "orbit-telemetry-demo" { continue }
			found := map[string]bool{}
			for _, source := range array(item["sources"]) { found[stringValue(source)] = true }
			return found["prometheus"] && found["loki"] && found["tempo"], nil
		}
		return false, nil
	}); err != nil { return err }

	if err := waitFor("ORBIT API returns the Prometheus metric", 120*time.Second, func() (bool, error) {
		body, err := getJSON(orbitAPIURL + "/api/v1/telemetry/metrics?window=1h&service=orbit-telemetry-demo")
		if err != nil { return false, err }
		if stringValue(body["source"]) != "prometheus" || stringValue(body["window"]) != "1h" || stringValue(body["queried_at"]) == "" { return false, nil }
		for _, raw := range array(body["data"]) { if len(array(object(raw)["values"])) > 0 { return true, nil } }
		return false, nil
	}); err != nil { return err }

	var correlatedTraceIDs map[string]struct{}
	if err := waitFor("ORBIT API returns Loki logs with a valid trace_id", 120*time.Second, func() (bool, error) {
		body, err := getJSON(orbitAPIURL + "/api/v1/telemetry/logs?service=orbit-telemetry-demo&window=1h&limit=50")
		if err != nil { return false, err }
		if stringValue(body["source"]) != "loki" || stringValue(body["service"]) != "orbit-telemetry-demo" { return false, nil }
		correlatedTraceIDs = traceIDsFromLogs(body)
		return len(array(body["data"])) > 0 && len(correlatedTraceIDs) > 0, nil
	}); err != nil { return err }

	if err := waitFor("Tempo exposes a trace ID also present in the Loki logs", 120*time.Second, func() (bool, error) {
		body, err := getJSON(orbitAPIURL + "/api/v1/telemetry/traces?service=orbit-telemetry-demo&window=1h&limit=20")
		if err != nil { return false, err }
		if stringValue(body["source"]) != "tempo" || stringValue(body["service"]) != "orbit-telemetry-demo" { return false, nil }
		for _, raw := range array(body["data"]) {
			id := strings.ToLower(stringValue(object(raw)["traceID"]))
			if len(id) != 32 || strings.Trim(id, "0") == "" { continue }
			if _, err := hex.DecodeString(id); err != nil { continue }
			if _, found := correlatedTraceIDs[id]; found { return true, nil }
		}
		return false, nil
	}); err != nil { return err }
	fmt.Println("PASS: all three signals are queryable through the ORBIT API")
	return nil
}

func main() {
	command := "smoke"
	if len(os.Args) > 1 { command = os.Args[1] }
	var err error
	switch command {
	case "wait": err = waitReady()
	case "smoke": err = smoke()
	case "scrape": err = checkPrometheusScrape()
	default: err = fmt.Errorf("unknown command %q (expected wait, smoke, or scrape)", command)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

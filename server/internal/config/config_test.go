package config

import (
	"reflect"
	"testing"
)

func TestCommaSeparatedOriginsTrimsAndDeduplicates(t *testing.T) {
	got := commaSeparated(" https://one.example, ,http://localhost:3000,https://one.example ")
	want := []string{"https://one.example", "http://localhost:3000"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commaSeparated() = %#v, want %#v", got, want)
	}
}

func TestCommaSeparatedOriginsEmpty(t *testing.T) {
	if got := commaSeparated(" , "); len(got) != 0 {
		t.Fatalf("commaSeparated() = %#v, want empty list", got)
	}
}


func TestLoadReadsOptionalTelemetryEndpoints(t *testing.T) {
	t.Setenv("ORBIT_VERSION", "0.6.0")
	t.Setenv("ORBIT_PROMETHEUS_URL", " http://prometheus:9090 ")
	t.Setenv("ORBIT_LOKI_URL", "http://loki:3100")
	t.Setenv("ORBIT_TEMPO_URL", "http://tempo:3200")

	got := Load()
	if got.Version != "0.6.0" {
		t.Fatalf("version = %q, want 0.6.0", got.Version)
	}
	if got.PrometheusURL != "http://prometheus:9090" || got.LokiURL != "http://loki:3100" || got.TempoURL != "http://tempo:3200" {
		t.Fatalf("unexpected telemetry URLs: %+v", got)
	}
}

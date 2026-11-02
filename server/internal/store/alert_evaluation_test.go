package store

import (
	"encoding/json"
	"testing"
)

func TestAlertInstanceIncludesNullableLifecycleTimestamps(t *testing.T) {
	payload, err := json.Marshal(AlertInstance{Status: "pending"})
	if err != nil {
		t.Fatalf("marshal pending alert instance: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode alert instance JSON: %v", err)
	}
	for _, name := range []string{"fired_at", "resolved_at"} {
		value, ok := fields[name]
		if !ok {
			t.Errorf("%s is missing; nullable lifecycle timestamps must be present in the API contract", name)
			continue
		}
		if string(value) != "null" {
			t.Errorf("%s = %s; want JSON null for a pending alert", name, value)
		}
	}
}

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

var (
	workloadResourceWorkspaceIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	workloadResourceServiceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
)

var workloadResourceAttributePersistenceAllowlist = map[string]struct{}{
	"service.name": {},
	"service.namespace": {},
	"service.version": {},
	"deployment.environment.name": {},
	"cloud.provider": {},
	"cloud.region": {},
	"cloud.availability_zone": {},
	"cloud.platform": {},
	"host.name": {},
	"host.type": {},
	"host.arch": {},
	"os.type": {},
	"os.name": {},
	"os.version": {},
	"os.description": {},
	"container.name": {},
	"container.image.name": {},
	"container.image.tag": {},
	"k8s.cluster.name": {},
	"k8s.namespace.name": {},
	"k8s.deployment.name": {},
	"k8s.statefulset.name": {},
	"k8s.daemonset.name": {},
	"k8s.job.name": {},
	"k8s.cronjob.name": {},
	"k8s.pod.name": {},
	"k8s.container.name": {},
	"k8s.node.name": {},
	"process.runtime.name": {},
	"process.runtime.version": {},
	"telemetry.sdk.name": {},
	"telemetry.sdk.language": {},
	"telemetry.sdk.version": {},
	"telemetry.distro.name": {},
	"telemetry.distro.version": {},
}

const (
	maxPersistedWorkloadResourceObservations = 32
	maxPersistedWorkloadResourceAttributes = 32
	maxPersistedWorkloadResourceValues = 8
	maxPersistedWorkloadResourceJSONBytes = 16 << 10
	maxPersistedWorkloadResourceValueBytes = 256
)

// RecordWorkloadResourceAttributes persists only the bounded attribute
// projection produced by the authenticated OTLP gateway. The workspace ID is
// taken from the gateway's verified authorization decision, never from OTLP.
func (r *Repository) RecordWorkloadResourceAttributes(
	ctx context.Context,
	workspaceID string,
	observations []store.WorkloadResourceObservation,
) error {
	if r == nil || r.pool == nil {
		return errors.New("PostgreSQL repository is not configured")
	}
	if !workloadResourceWorkspaceIDPattern.MatchString(workspaceID) {
		return errors.New("invalid workspace ID for workload resource attributes")
	}
	if len(observations) == 0 {
		return nil
	}
	if len(observations) > maxPersistedWorkloadResourceObservations {
		return fmt.Errorf("workload resource observation count exceeds %d", maxPersistedWorkloadResourceObservations)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin workload resource attribute transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	seenServices := make(map[string]struct{}, len(observations))
	for _, observation := range observations {
		if !workloadResourceServiceNamePattern.MatchString(observation.ServiceName) {
			return errors.New("invalid workload resource service name")
		}
		if _, exists := seenServices[observation.ServiceName]; exists {
			return errors.New("duplicate workload resource service observation")
		}
		seenServices[observation.ServiceName] = struct{}{}

		attributesJSON, err := marshalWorkloadResourceAttributes(observation)
		if err != nil {
			return err
		}
		incomingSources := make(map[string][]string, len(observation.Attributes))
		for key := range observation.Attributes {
			incomingSources[key] = []string{"otlp"}
		}
		incomingSources, err = normalizeResourceAttributeSources(incomingSources)
		if err != nil {
			return fmt.Errorf("validate OTLP resource attribute provenance: %w", err)
		}

		var serviceID string
		err = tx.QueryRow(ctx, `
			INSERT INTO telemetry_services (workspace_id, name, resource_attributes)
			VALUES ($1::uuid, $2, $3::jsonb)
			ON CONFLICT (workspace_id, name) DO UPDATE SET
				resource_attributes = telemetry_services.resource_attributes || EXCLUDED.resource_attributes,
				last_seen_at = NOW(),
				last_discovered_at = NOW(),
				updated_at = NOW()
			RETURNING id::text
		`, workspaceID, observation.ServiceName, string(attributesJSON)).Scan(&serviceID)
		if err != nil {
			return fmt.Errorf("upsert workspace OTLP telemetry service: %w", err)
		}

		var existingJSON []byte
		if err := tx.QueryRow(ctx, `
			SELECT resource_attribute_sources
			FROM telemetry_services
			WHERE id = $1::uuid
			FOR UPDATE
		`, serviceID).Scan(&existingJSON); err != nil {
			return fmt.Errorf("read workspace OTLP attribute provenance: %w", err)
		}
		existingSources := make(map[string][]string)
		if err := json.Unmarshal(existingJSON, &existingSources); err != nil {
			return fmt.Errorf("decode workspace OTLP attribute provenance: %w", err)
		}
		mergedSources := mergeResourceAttributeSources(existingSources, incomingSources)
		mergedJSON, err := json.Marshal(mergedSources)
		if err != nil {
			return fmt.Errorf("encode workspace OTLP attribute provenance: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE telemetry_services
			SET resource_attribute_sources = $2::jsonb
			WHERE id = $1::uuid
		`, serviceID, string(mergedJSON)); err != nil {
			return fmt.Errorf("persist workspace OTLP attribute provenance: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO telemetry_service_sources (service_id, source)
			VALUES ($1::uuid, 'otlp')
			ON CONFLICT (service_id, source) DO UPDATE SET
				last_seen_at = NOW(),
				last_discovered_at = NOW()
		`, serviceID); err != nil {
			return fmt.Errorf("upsert workspace OTLP source observation: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit workspace OTLP resource attributes: %w", err)
	}
	return nil
}

func marshalWorkloadResourceAttributes(observation store.WorkloadResourceObservation) ([]byte, error) {
	attributes := observation.Attributes
	if len(attributes) == 0 || len(attributes) > maxPersistedWorkloadResourceAttributes {
		return nil, fmt.Errorf("workload resource attributes must contain between 1 and %d keys", maxPersistedWorkloadResourceAttributes)
	}
	serviceNameValues, ok := attributes["service.name"].([]any)
	if !ok || len(serviceNameValues) != 1 || serviceNameValues[0] != observation.ServiceName {
		return nil, errors.New("workload resource attributes must include their canonical service.name")
	}

	for key, rawValues := range attributes {
		if !resourceAttributeNamePattern.MatchString(key) {
			return nil, errors.New("invalid workload resource attribute key")
		}
		if _, allowed := workloadResourceAttributePersistenceAllowlist[key]; !allowed {
			return nil, errors.New("workload resource attribute key is not allow-listed")
		}
		values, ok := rawValues.([]any)
		if !ok || len(values) == 0 || len(values) > maxPersistedWorkloadResourceValues {
			return nil, errors.New("workload resource attribute values must be bounded arrays")
		}
		for _, value := range values {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) == "" || len(typed) > maxPersistedWorkloadResourceValueBytes {
					return nil, errors.New("workload resource attribute string is empty or too large")
				}
			case bool, int64:
			case float64:
				if math.IsNaN(typed) || math.IsInf(typed, 0) {
					return nil, errors.New("workload resource attribute contains a non-finite number")
				}
			default:
				return nil, errors.New("unsupported workload resource attribute value type")
			}
		}
	}
	encoded, err := json.Marshal(attributes)
	if err != nil {
		return nil, fmt.Errorf("encode workload resource attributes: %w", err)
	}
	if len(encoded) > maxPersistedWorkloadResourceJSONBytes {
		return nil, fmt.Errorf("workload resource attributes exceed %d bytes", maxPersistedWorkloadResourceJSONBytes)
	}
	return encoded, nil
}

var _ store.WorkloadResourceAttributeRepository = (*Repository)(nil)

func TestMarshalWorkloadResourceAttributesEnforcesCuratedAllowlist(t *testing.T) {
	valid := store.WorkloadResourceObservation{
		ServiceName: "payments",
		Attributes: map[string]any{
			"service.name": []any{"payments"},
			"service.version": []any{"1.2.3"},
			"cloud.region": []any{"eu-central-1"},
			"host.type": []any{"compute"},
		},
	}
	if _, err := marshalWorkloadResourceAttributes(valid); err != nil {
		t.Fatalf("valid curated resource attributes were rejected: %v", err)
	}

	cases := []struct {
		name string
		mutate func(map[string]any)
	}{
		{
			name: "private user identifier",
			mutate: func(attributes map[string]any) {
				attributes["user.email"] = []any{"private@example.invalid"}
			},
		},
		{
			name: "process command line",
			mutate: func(attributes map[string]any) {
				attributes["process.command_line"] = []any{"app --token secret"}
			},
		},
		{
			name: "high cardinality pod UID",
			mutate: func(attributes map[string]any) {
				attributes["k8s.pod.uid"] = []any{"random-uid"}
			},
		},
		{
			name: "producer tenant identity",
			mutate: func(attributes map[string]any) {
				attributes["orbit.workspace.id"] = []any{"another-workspace"}
			},
		},
		{
			name: "non-array value",
			mutate: func(attributes map[string]any) {
				attributes["cloud.region"] = "eu-central-1"
			},
		},
		{
			name: "missing canonical service name",
			mutate: func(attributes map[string]any) {
				delete(attributes, "service.name")
			},
		},
		{
			name: "mismatched service name",
			mutate: func(attributes map[string]any) {
				attributes["service.name"] = []any{"another-service"}
			},
		},
		{
			name: "oversized string value",
			mutate: func(attributes map[string]any) {
				attributes["host.name"] = []any{strings.Repeat("x", maxPersistedWorkloadResourceValueBytes+1)}
			},
		},
		{
			name: "non-finite floating point",
			mutate: func(attributes map[string]any) {
				attributes["host.type"] = []any{math.Inf(1)}
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			attributes := make(map[string]any, len(valid.Attributes)+1)
			for key, value := range valid.Attributes {
				values, ok := value.([]any)
				if ok {
					attributes[key] = append([]any(nil), values...)
				} else {
					attributes[key] = value
				}
			}
			test.mutate(attributes)
			observation := store.WorkloadResourceObservation{
				ServiceName: valid.ServiceName,
				Attributes: attributes,
			}
			if _, err := marshalWorkloadResourceAttributes(observation); err == nil {
				t.Fatal("invalid workload resource attributes were accepted")
			}
		})
	}
}

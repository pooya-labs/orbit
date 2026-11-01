package ingest

import (
	"errors"
	"testing"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
)

const canonicalWorkspace = "00000000-0000-4000-8000-000000000001"

func stringAttribute(key, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key: key,
		Value: &commonv1.AnyValue{
			Value: &commonv1.AnyValue_StringValue{StringValue: value},
		},
	}
}

func attributeValue(resource *resourcev1.Resource, key string) string {
	for _, attribute := range resource.GetAttributes() {
		if attribute.GetKey() == key {
			return attribute.GetValue().GetStringValue()
		}
	}
	return ""
}

func TestAttributeWorkspaceInjectsCanonicalResourceAttribute(t *testing.T) {
	resource := &resourcev1.Resource{
		Attributes: []*commonv1.KeyValue{
			stringAttribute("service.name", "payments"),
			stringAttribute("deployment.environment", "test"),
		},
	}
	if err := AttributeWorkspace(resource, canonicalWorkspace); err != nil {
		t.Fatalf("AttributeWorkspace() error = %v", err)
	}
	if got := attributeValue(resource, WorkspaceAttribute); got != canonicalWorkspace {
		t.Fatalf("canonical workspace attribute = %q, want %q", got, canonicalWorkspace)
	}
	if got := attributeValue(resource, "service.name"); got != "payments" {
		t.Fatalf("unrelated resource attribute changed: %q", got)
	}
}

func TestAttributeWorkspaceRejectsProducerTenantAttributes(t *testing.T) {
	for _, key := range []string{
		WorkspaceAttribute,
		"orbit.workspace_id",
		"orbit.tenant.id",
		"tenant.id",
		"workspace.id",
	} {
		t.Run(key, func(t *testing.T) {
			resource := &resourcev1.Resource{Attributes: []*commonv1.KeyValue{
				stringAttribute(key, "attacker-workspace"),
			}}
			err := AttributeWorkspace(resource, canonicalWorkspace)
			if !errors.Is(err, ErrReservedTenantAttribute) {
				t.Fatalf("AttributeWorkspace() error = %v, want ErrReservedTenantAttribute", err)
			}
			if got := attributeValue(resource, WorkspaceAttribute); got != "" {
				t.Fatalf("canonical attribute injected after rejection: %q", got)
			}
		})
	}
}

func TestRejectReservedAttributesAtNestedSignalScopes(t *testing.T) {
	scope := &commonv1.InstrumentationScope{
		Attributes: []*commonv1.KeyValue{stringAttribute(WorkspaceAttribute, "forged")},
	}
	if err := rejectReservedAttributes(scope.ProtoReflect()); !errors.Is(err, ErrReservedTenantAttribute) {
		t.Fatalf("instrumentation-scope attribute error = %v", err)
	}

	point := &metricsv1.NumberDataPoint{
		Attributes: []*commonv1.KeyValue{stringAttribute("tenant.id", "forged")},
	}
	if err := rejectReservedAttributes(point.ProtoReflect()); !errors.Is(err, ErrReservedTenantAttribute) {
		t.Fatalf("data-point attribute error = %v", err)
	}
}

func TestAttributeWorkspaceRejectsInvalidRequestOrWorkspace(t *testing.T) {
	if err := AttributeWorkspace(nil, canonicalWorkspace); err == nil {
		t.Fatal("nil OTLP request was accepted")
	}
	if err := AttributeWorkspace(&resourcev1.Resource{}, "not-a-uuid"); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("invalid workspace error = %v, want ErrInvalidTarget", err)
	}
}

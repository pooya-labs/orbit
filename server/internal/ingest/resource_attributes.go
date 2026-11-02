package ingest

import (
	"context"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracespb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const (
	maxWorkloadResourceAttributes = 32
	maxWorkloadResourceValues = 8
	maxWorkloadResourceServices = 32
	maxWorkloadResourceValueBytes = 256
	resourceAttributePersistTimeout = time.Second
)

var workloadServiceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

// workloadResourceAttributeAllowlist intentionally excludes process command
// lines, executable paths, user identifiers, pod UIDs, container IDs, and
// other values that can contain secrets or high-cardinality identifiers.
var workloadResourceAttributeAllowlist = map[string]struct{}{
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

// ExtractWorkloadResourceAttributes returns a bounded, curated projection of
// OTLP resource metadata. It must be called only after AttributeWorkspace has
// rejected producer-supplied tenant fields and injected the authorized ID.
// Values are grouped by service.name so different services in one batch are
// persisted independently.
func ExtractWorkloadResourceAttributes(request proto.Message) []store.WorkloadResourceObservation {
	resources := make([]*resourcev1.Resource, 0)
	switch typed := request.(type) {
	case *metricsv1.ExportMetricsServiceRequest:
		for _, item := range typed.GetResourceMetrics() {
			if item != nil && item.GetResource() != nil {
				resources = append(resources, item.GetResource())
			}
		}
	case *logsv1.ExportLogsServiceRequest:
		for _, item := range typed.GetResourceLogs() {
			if item != nil && item.GetResource() != nil {
				resources = append(resources, item.GetResource())
			}
		}
	case *tracesv1.ExportTraceServiceRequest:
		for _, item := range typed.GetResourceSpans() {
			if item != nil && item.GetResource() != nil {
				resources = append(resources, item.GetResource())
			}
		}
	default:
		return nil
	}

	byService := make(map[string]map[string][]any)
	for _, resource := range resources {
		serviceName := workloadResourceServiceName(resource)
		if serviceName == "" {
			continue
		}
		if _, exists := byService[serviceName]; !exists {
			if len(byService) >= maxWorkloadResourceServices {
				continue
			}
			byService[serviceName] = map[string][]any{
				"service.name": {serviceName},
			}
		}
		attributes := byService[serviceName]
		for _, attribute := range resource.GetAttributes() {
			if attribute == nil {
				continue
			}
			key := normalizeWorkloadResourceAttributeKey(attribute.GetKey())
			if _, allowed := workloadResourceAttributeAllowlist[key]; !allowed || key == "service.name" {
				continue
			}
			value, ok := workloadResourceScalar(attribute)
			if !ok {
				continue
			}
			values, exists := attributes[key]
			if !exists && len(attributes) >= maxWorkloadResourceAttributes {
				continue
			}
			if !containsWorkloadResourceValue(values, value) && len(values) < maxWorkloadResourceValues {
				attributes[key] = append(values, value)
			}
		}
	}

	names := make([]string, 0, len(byService))
	for name := range byService {
		names = append(names, name)
	}
	sort.Strings(names)

	observations := make([]store.WorkloadResourceObservation, 0, len(names))
	for _, name := range names {
		attributes := make(map[string]any, len(byService[name]))
		for key, values := range byService[name] {
			attributes[key] = values
		}
		observations = append(observations, store.WorkloadResourceObservation{
			ServiceName: name,
			Attributes: attributes,
		})
	}
	return observations
}

func workloadResourceServiceName(resource *resourcev1.Resource) string {
	if resource == nil {
		return ""
	}
	for _, attribute := range resource.GetAttributes() {
		if attribute == nil || attribute.GetKey() != "service.name" {
			continue
		}
		value, ok := workloadResourceScalar(attribute)
		if !ok {
			return ""
		}
		name, ok := value.(string)
		if !ok || !workloadServiceNamePattern.MatchString(name) {
			return ""
		}
		return name
	}
	return ""
}

func normalizeWorkloadResourceAttributeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "deployment.environment" {
		return "deployment.environment.name"
	}
	return key
}

func workloadResourceScalar(attribute *commonv1.KeyValue) (any, bool) {
	if attribute == nil || attribute.GetValue() == nil {
		return nil, false
	}
	switch value := attribute.GetValue().GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		text := strings.TrimSpace(value.StringValue)
		if text == "" || len(text) > maxWorkloadResourceValueBytes {
			return nil, false
		}
		return text, true
	case *commonv1.AnyValue_IntValue:
		return value.IntValue, true
	case *commonv1.AnyValue_DoubleValue:
		if math.IsNaN(value.DoubleValue) || math.IsInf(value.DoubleValue, 0) {
			return nil, false
		}
		return value.DoubleValue, true
	case *commonv1.AnyValue_BoolValue:
		return value.BoolValue, true
	default:
		return nil, false
	}
}

func containsWorkloadResourceValue(values []any, candidate any) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, candidate) {
			return true
		}
	}
	return false
}

func recordWorkloadResourceAttributes(
	ctx context.Context,
	recorder store.WorkloadResourceAttributeRepository,
	onError func(error),
	workspaceID string,
	observations []store.WorkloadResourceObservation,
) {
	if recorder == nil || len(observations) == 0 {
		return
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resourceAttributePersistTimeout)
	defer cancel()
	if err := recorder.RecordWorkloadResourceAttributes(persistCtx, workspaceID, observations); err != nil && onError != nil {
		onError(err)
	}
}

// Ensure the three generated OTLP resource-bearing request families remain
// represented in this extractor's compile-time dependency surface.
var (
	_ proto.Message = (*metricsv1.ExportMetricsServiceRequest)(nil)
	_ proto.Message = (*logsv1.ExportLogsServiceRequest)(nil)
	_ proto.Message = (*tracesv1.ExportTraceServiceRequest)(nil)
	_ proto.Message = (*metricspb.ResourceMetrics)(nil)
	_ proto.Message = (*logspb.ResourceLogs)(nil)
	_ proto.Message = (*tracespb.ResourceSpans)(nil)
)

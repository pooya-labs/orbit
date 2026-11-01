// Package ingest contains the security boundary for workload telemetry.
package ingest

import (
	"errors"
	"fmt"
	"regexp"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/proto"
)

const WorkspaceAttribute = "orbit.workspace.id"

var workspaceUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var ErrReservedTenantAttribute = errors.New("producer supplied a reserved tenant attribute")

// AttributeWorkspace validates an OTLP Export request, rejects any producer
// attempt to set tenant identity at any attribute scope, and injects the
// server-authorized workspace ID into every Resource message. The caller must
// invoke this only after Authorizer.Authorize returns a successful Decision.
func AttributeWorkspace(request proto.Message, workspaceID string) error {
	if request == nil || !request.ProtoReflect().IsValid() {
		return errors.New("OTLP request is required")
	}
	if !workspaceUUIDPattern.MatchString(workspaceID) {
		return ErrInvalidTarget
	}

	root := request.ProtoReflect()
	if err := rejectReservedAttributes(root); err != nil {
		return err
	}
	resources := 0
	injectWorkspaceResource(root, workspaceID, &resources)
	if resources == 0 {
		return errors.New("OTLP request contains no resource scope")
	}
	return nil
}

func rejectReservedAttributes(message protoreflect.Message) error {
	if !message.IsValid() {
		return nil
	}
	fields := message.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		value := message.Get(field)

		if field.Name() == "attributes" && field.IsList() &&
			field.Kind() == protoreflect.MessageKind &&
			field.Message().FullName() == "opentelemetry.proto.common.v1.KeyValue" {
			list := value.List()
			for j := 0; j < list.Len(); j++ {
				attribute := list.Get(j).Message()
				keyField := attribute.Descriptor().Fields().ByName("key")
				if keyField == nil {
					continue
				}
				key := attribute.Get(keyField).String()
				if isReservedTenantAttribute(key) {
					return fmt.Errorf("%w: %s", ErrReservedTenantAttribute, key)
				}
			}
		}

		switch {
		case field.IsList() && field.Kind() == protoreflect.MessageKind:
			list := value.List()
			for j := 0; j < list.Len(); j++ {
				if err := rejectReservedAttributes(list.Get(j).Message()); err != nil {
					return err
				}
			}
		case field.IsMap() && field.MapValue().Kind() == protoreflect.MessageKind:
			var nestedErr error
			value.Map().Range(func(_ protoreflect.MapKey, item protoreflect.Value) bool {
				nestedErr = rejectReservedAttributes(item.Message())
				return nestedErr == nil
			})
			if nestedErr != nil {
				return nestedErr
			}
		case field.Kind() == protoreflect.MessageKind:
			if message.Has(field) {
				if err := rejectReservedAttributes(value.Message()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func injectWorkspaceResource(message protoreflect.Message, workspaceID string, count *int) {
	if !message.IsValid() {
		return
	}
	if message.Descriptor().FullName() == "opentelemetry.proto.resource.v1.Resource" {
		field := message.Descriptor().Fields().ByName("attributes")
		if field != nil && field.IsList() && field.Message().FullName() == "opentelemetry.proto.common.v1.KeyValue" {
			attribute := &commonv1.KeyValue{
				Key: WorkspaceAttribute,
				Value: &commonv1.AnyValue{
					Value: &commonv1.AnyValue_StringValue{StringValue: workspaceID},
				},
			}
			message.Mutable(field).List().Append(protoreflect.ValueOfMessage(attribute.ProtoReflect()))
			*count++
		}
	}

	fields := message.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		value := message.Get(field)
		switch {
		case field.IsList() && field.Kind() == protoreflect.MessageKind:
			list := value.List()
			for j := 0; j < list.Len(); j++ {
				injectWorkspaceResource(list.Get(j).Message(), workspaceID, count)
			}
		case field.IsMap() && field.MapValue().Kind() == protoreflect.MessageKind:
			value.Map().Range(func(_ protoreflect.MapKey, item protoreflect.Value) bool {
				injectWorkspaceResource(item.Message(), workspaceID, count)
				return true
			})
		case field.Kind() == protoreflect.MessageKind:
			if message.Has(field) {
				injectWorkspaceResource(value.Message(), workspaceID, count)
			}
		}
	}
}

func isReservedTenantAttribute(key string) bool {
	switch key {
	case WorkspaceAttribute, "orbit.workspace_id", "orbit.tenant.id", "tenant.id", "workspace.id":
		return true
	default:
		return false
	}
}

// Keep the concrete resource type in this package's public dependency surface
// explicit: it documents the OTLP resource model used for canonical attribution.
var _ proto.Message = (*resourcev1.Resource)(nil)

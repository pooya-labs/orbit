# ADR 0005 — Review of the authenticated OTLP resource-attribute allowlist

- Status: Accepted (current allowlist retained; expansion deferred)
- Date: 2026-11-03
- Scope: M04 resource metadata capture and storage
- Related: ADR 0002 (authenticated workspace-aware OTLP ingestion)

## Context

ORBIT captures a bounded projection of OTLP resource attributes only after workload identity verification, workspace/signal-grant authorization, rejection of producer-supplied tenant fields, canonical workspace attribution, and successful Collector export. The extractor and PostgreSQL repository intentionally keep separate allowlists: the gateway controls what it collects, while persistence independently validates what it will store.

The two lists had no automated parity check. A future change could therefore be silently dropped at persistence or, in the opposite direction, accepted by storage without being produced by the reviewed extractor. The current limits also exist at both boundaries and must stay aligned.

## Review basis

The review compared the current projection with the OpenTelemetry Resource Semantic Conventions 1.44.0 and its guidance for resource/entity identity. Standard naming alone is not a privacy or cardinality approval. A field also needs a concrete ORBIT use case, a bounded value model, an operational owner, and an acceptable disclosure profile.

References:
- [OpenTelemetry Resource semantic conventions](https://opentelemetry.io/docs/specs/semconv/resource/)
- [Service and service-instance conventions](https://opentelemetry.io/docs/specs/semconv/resource/service/)
- [Resource/entity modelling guidance](https://opentelemetry.io/docs/specs/semconv/how-to-write-conventions/resource-and-entities/)

## Decision

Do not add new producer-controlled attributes in this review. The existing curated set already covers service identity/version/namespace, deployment environment, selected cloud and host descriptors, OS/container/Kubernetes names, process runtime, and telemetry SDK/distro metadata. No additional candidate had a sufficiently clear product use case to justify the extra disclosure and cardinality surface.

The following candidates remain explicitly deferred:

| Candidate | Reason for deferral |
| --- | --- |
| `service.instance.id` | Although standardized, it uniquely identifies a service instance and is commonly an opaque UUID. It would add a per-instance identity dimension without a current ORBIT view that consumes it. |
| `service.criticality` | Alpha stability in the reviewed conventions; no current UI or policy consumes it. |
| `host.id`, `container.id`, `k8s.pod.uid`, `k8s.cluster.uid` | Infrastructure identifiers with high cardinality and cross-system correlation implications. |
| `cloud.account.id`, `cloud.resource_id` | Account/resource identifiers are unnecessary for the current service catalog and can expose customer infrastructure topology. |
| `host.ip`, `host.mac` | Network identifiers are not needed by the current catalog and are explicitly opt-in in the host conventions. |
| `process.command_line`, `process.command_args`, `process.executable.path`, `process.owner` | May disclose credentials, local paths, usernames, or other sensitive runtime details. |
| `user.id`, `user.name`, `user.email` | User identifiers are outside the purpose of a service resource catalog and create privacy risk. |

The existing extractor and persistence bounds remain unchanged: at most 32 services per request, 32 attribute keys per service, 8 distinct values per key, 256 bytes per string, and 16 KiB for the persisted JSON projection. The canonical workspace identifier is never accepted as producer metadata.

## Guardrails

PR #126 adds `scripts/check_resource_attribute_allowlist.py` to CI. It verifies:
- exact equality of the independent ingestion and PostgreSQL persistence allowlists;
- explicit exclusion of the sensitive/high-cardinality candidates above;
- equality of service-count, attribute-count, value-count, and string-length limits at both boundaries; and
- retention of the persistence JSON size cap.

This check prevents accidental drift; it does not replace review when a new attribute is proposed. A future expansion must name a concrete consumer/use case, cite the relevant convention and its stability, document privacy/cardinality characteristics, and update both allowlists and extractor/persistence tests together.

## Consequences

The M04 expansion item remains open. The current allowlist is reviewed and guarded, but no claim is made that every possible standard resource attribute is safe or useful. Arbitrary producer attributes remain unsupported.

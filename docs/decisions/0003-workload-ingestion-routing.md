# ADR 0003 — Workspace selection for authenticated OTLP ingestion

- Status: Accepted
- Date: 2026-11-01
- Depends on: ADR 0002 (authenticated workspace-aware OTLP ingestion)

## Context

A workload identity may hold grants for more than one workspace. The ingestion gateway therefore needs an explicit way to determine which workspace a request is attempting to write to. The value used to select a candidate workspace is necessarily supplied by the client, but it must never become proof of authorization.

Treating a workspace identifier as trusted merely because it appears in a URL, HTTP header, gRPC metadata, or OTLP resource would recreate the tenant-spoofing vulnerability described in ADR 0002.

## Decision

The gateway uses an explicit, request-scoped workspace selector. The selector is untrusted input and is accepted only after the verified workload identity is authorized for that workspace and signal by PostgreSQL.

### OTLP/HTTP

Use workspace-qualified OTLP paths:

- `POST /v1/workspaces/{workspace_id}/v1/metrics`
- `POST /v1/workspaces/{workspace_id}/v1/logs`
- `POST /v1/workspaces/{workspace_id}/v1/traces`

The path workspace ID is a candidate selector only. For every request, the gateway must:

1. Verify the bearer token with the dedicated workload-token verifier.
2. Take issuer and subject only from the verified token principal.
3. Validate the candidate workspace ID format.
4. Call `AuthorizeWorkloadTelemetry(issuer, subject, workspace_id, signal)` before forwarding any bytes to the Collector.
5. Fail closed on authorization denial or database errors.
6. Remove or validate reserved tenant attributes and inject the canonical workspace ID obtained from the authorized server-side decision.

The gateway must not accept a workspace selector from query parameters, arbitrary HTTP headers, or the OTLP body as a substitute for the path selector. Supplying multiple competing selectors is a bad request.

### OTLP/gRPC

Standard OTLP gRPC method paths cannot carry a workspace identifier. Require exactly one gRPC metadata value named `orbit-workspace-id` on each Export call. Treat it as the same untrusted candidate selector used by HTTP, never as an authorization claim. Verify the token and database grant for the requested signal before reading/forwarding the Export payload. Reject missing, duplicate, malformed, or conflicting metadata.

### One request, one workspace, one signal

- Each HTTP request or gRPC Export call is authorized for exactly one workspace and one signal.
- A workload with multiple grants must explicitly select the destination on every request; there is no implicit default workspace, even when only one grant currently exists.
- A batch containing reserved workspace attributes that disagree with the selected, authorized workspace is rejected as a whole. The gateway must never split and partially forward a mixed batch.
- Producer-supplied reserved workspace attributes are removed from all relevant OTLP attribute scopes before canonical attribution is injected.
- Authorization is checked on every request in the initial implementation. Any future cache must have a short, documented bound and revocation invalidation.

## Security properties

- Knowing or guessing a workspace UUID does not grant access.
- A valid workload token without a matching workspace-and-signal grant cannot write telemetry.
- A workload granted access to workspace A cannot write to workspace B by changing the HTTP path or gRPC metadata; the database authorization check denies the request.
- Human API tokens, workspace membership, producer attributes, and arbitrary headers are not accepted as workload authorization.
- Authentication, authorization, malformed-selector, and upstream errors must not reveal whether an unrelated workspace or workload identity exists.
- Bearer tokens and telemetry payloads are never written to logs or authorization audit events.

## Required tests

The gateway integration suite must cover both transports and all three signals:

- missing, duplicate, malformed, and conflicting selectors;
- valid identity and matching workspace/signal grant;
- valid identity with no grant for the selected workspace;
- identity disabled, expired, or revoked;
- workspace A grant with workspace B selected;
- forged reserved attribute equal to another workspace;
- multiple conflicting workspace values within one payload;
- mixed-resource batches and all-or-nothing rejection;
- database unavailable during authorization;
- oversized payloads and rate-limit exhaustion;
- revocation taking effect on the next authorization decision.

## Consequences

The client must configure a workspace selector in addition to its workload credential. This is intentional: routing is explicit, while authorization remains entirely server-side. The HTTP path and gRPC metadata are routing hints, not trusted identity claims.

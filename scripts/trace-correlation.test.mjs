import assert from "node:assert/strict";
import test from "node:test";

import { extractTraceIdFromLog } from "../lib/orbit/trace-correlation.mjs";

const traceId = "4bf92f3577b34da6a3ce929d0e0e4736";

test("extracts a lower-case Trace ID from structured JSON fields", () => {
  assert.equal(extractTraceIdFromLog(JSON.stringify({ trace_id: traceId.toUpperCase() })), traceId);
  assert.equal(extractTraceIdFromLog(JSON.stringify({ traceID: traceId })), traceId);
});

test("extracts a Trace ID from bounded nested OpenTelemetry attributes", () => {
  assert.equal(
    extractTraceIdFromLog(JSON.stringify({ attributes: { "trace.id": traceId } })),
    traceId,
  );
  assert.equal(
    extractTraceIdFromLog(JSON.stringify({ resource: { context: { trace_id: traceId } } })),
    traceId,
  );
});

test("extracts a Trace ID from a Loki stream field", () => {
  assert.equal(extractTraceIdFromLog("request completed", { trace_id: traceId }), traceId);
});

test("extracts a Trace ID from a plain-text key/value log line", () => {
  assert.equal(extractTraceIdFromLog(`request completed trace_id=${traceId} status=200`), traceId);
  assert.equal(extractTraceIdFromLog(`{"traceID":"${traceId}"}`), traceId);
});

test("rejects zero, malformed, truncated, and unrelated identifiers", () => {
  assert.equal(extractTraceIdFromLog(`trace_id=${"0".repeat(32)}`), null);
  assert.equal(extractTraceIdFromLog("trace_id=1234"), null);
  assert.equal(extractTraceIdFromLog(`trace_id=${"g".repeat(32)}`), null);
  assert.equal(extractTraceIdFromLog(`request_id=${traceId}`), null);
  assert.equal(extractTraceIdFromLog(`not_trace_id=${traceId}`), null);
  assert.equal(extractTraceIdFromLog(""), null);
  assert.equal(extractTraceIdFromLog(null), null);
});

test("does not accept a 32-character prefix of a longer hexadecimal value", () => {
  assert.equal(extractTraceIdFromLog(`trace_id=${traceId}a`), null);
  assert.equal(extractTraceIdFromLog(`trace_id=${traceId}g`), null);
});

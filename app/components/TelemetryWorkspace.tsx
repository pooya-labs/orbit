"use client";

import { useEffect, useMemo, useState } from "react";
import {
  Activity,
  AlertCircle,
  Clock3,
  FileText,
  RefreshCw,
  Workflow,
} from "lucide-react";
import {
  getTelemetryMetrics,
  listTelemetryLogs,
  listTelemetryTraces,
} from "../lib/orbit/client";
import type {
  TelemetryLogsResponse,
  TelemetryMetricsResponse,
  TelemetryTracesResponse,
  TelemetryWindow,
} from "../lib/orbit/contracts";

export type TelemetryView = "Metrics" | "Logs" | "Traces";

const windows: { value: TelemetryWindow; label: string }[] = [
  { value: "15m", label: "15 min" },
  { value: "1h", label: "1 hour" },
  { value: "6h", label: "6 hours" },
  { value: "24h", label: "24 hours" },
];

function formatClock(timestamp: number | string): string {
  const value = typeof timestamp === "number" ? timestamp : Number(timestamp);
  if (!Number.isFinite(value)) return "Time unavailable";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Time unavailable";
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}

function formatRate(value: number): string {
  if (!Number.isFinite(value)) return "—";
  if (Math.abs(value) >= 1000) return `${(value / 1000).toFixed(1)}k`;
  return value.toFixed(value >= 10 ? 1 : 2);
}

function nanoTimestamp(value: string): number {
  try {
    return Number(BigInt(value) / 1_000_000n);
  } catch {
    return Number.NaN;
  }
}

function TelemetryEmpty({ title, message }: { title: string; message: string }) {
  return (
    <div className="telemetry-empty">
      <div className="telemetry-empty-icon"><Activity size={19} /></div>
      <strong>{title}</strong>
      <p>{message}</p>
    </div>
  );
}

function TelemetryLoading() {
  return (
    <div className="telemetry-loading" role="status" aria-live="polite">
      <span className="telemetry-spinner" />
      <span>Querying telemetry backends…</span>
    </div>
  );
}

export default function TelemetryWorkspace({
  view,
  apiBaseUrl,
}: {
  view: TelemetryView;
  apiBaseUrl: string;
}) {
  const [windowRange, setWindowRange] = useState<TelemetryWindow>("1h");
  const [metrics, setMetrics] = useState<TelemetryMetricsResponse | null>(null);
  const [logs, setLogs] = useState<TelemetryLogsResponse | null>(null);
  const [traces, setTraces] = useState<TelemetryTracesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    let mounted = true;

    const load = async (initial: boolean) => {
      if (initial) setLoading(true);
      else setRefreshing(true);
      setError("");
      try {
        if (view === "Metrics") {
          const response = await getTelemetryMetrics(windowRange, controller.signal, apiBaseUrl);
          if (mounted) setMetrics(response);
        } else if (view === "Logs") {
          const response = await listTelemetryLogs(
            { window: windowRange, service: "orbit-telemetry-demo", limit: 50 },
            controller.signal,
            apiBaseUrl,
          );
          if (mounted) setLogs(response);
        } else {
          const response = await listTelemetryTraces(
            { window: windowRange, service: "orbit-telemetry-demo", limit: 50 },
            controller.signal,
            apiBaseUrl,
          );
          if (mounted) setTraces(response);
        }
      } catch (cause) {
        if (controller.signal.aborted) return;
        if (mounted) {
          setError(cause instanceof Error ? cause.message : "Unable to query telemetry.");
        }
      } finally {
        if (mounted) {
          setLoading(false);
          setRefreshing(false);
        }
      }
    };

    void load(true);
    const interval = window.setInterval(() => void load(false), 30_000);

    return () => {
      mounted = false;
      controller.abort();
      window.clearInterval(interval);
    };
  }, [view, windowRange, apiBaseUrl, reloadKey]);

  const metricSamples = useMemo(() => {
    if (!metrics) return [];
    return metrics.data
      .flatMap((series) => series.values)
      .map(([timestamp, value]) => ({ timestamp, value: Number(value) }))
      .filter((point) => Number.isFinite(point.value))
      .sort((a, b) => a.timestamp - b.timestamp);
  }, [metrics]);

  const chartPoints = useMemo(() => {
    if (metricSamples.length < 2) return "";
    const values = metricSamples.map((point) => point.value);
    const min = Math.min(0, ...values);
    const max = Math.max(...values);
    const span = max - min || 1;
    return values.map((value, index) => {
      const x = 8 + (index / (values.length - 1)) * 584;
      const y = 108 - ((value - min) / span) * 92;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    }).join(" ");
  }, [metricSamples]);

  const logRows = useMemo(() => {
    if (!logs) return [];
    return logs.data.flatMap((stream) =>
      stream.values.map(([timestamp, line]) => ({
        timestamp,
        line,
        labels: stream.stream,
      })),
    ).sort((a, b) => Number(b.timestamp) - Number(a.timestamp));
  }, [logs]);

  const traceRows = traces?.data ?? [];
  const queriedAt =
    view === "Metrics" ? metrics?.queried_at :
    view === "Logs" ? logs?.queried_at :
    traces?.queried_at;
  const backend =
    view === "Metrics" ? "PROMETHEUS" :
    view === "Logs" ? "LOKI" : "TEMPO";
  const count =
    view === "Metrics" ? metricSamples.length :
    view === "Logs" ? logRows.length : traceRows.length;

  return (
    <section className="telemetry-workspace" aria-labelledby="telemetry-title">
      <div className="telemetry-toolbar">
        <div className="telemetry-view-title">
          <span className="telemetry-view-icon">
            {view === "Metrics" ? <Activity size={18} /> : view === "Logs" ? <FileText size={18} /> : <Workflow size={18} />}
          </span>
          <div>
            <h2 id="telemetry-title">{view}</h2>
            <p>Live query · {backend} · orbit-telemetry-demo</p>
          </div>
        </div>
        <div className="telemetry-controls">
          <div className="range-switcher" role="group" aria-label="Telemetry time range">
            {windows.map((item) => (
              <button key={item.value} className={windowRange === item.value ? "selected" : ""} onClick={() => setWindowRange(item.value)}>
                {item.label}
              </button>
            ))}
          </div>
          <button className="icon-button" onClick={() => setReloadKey((value) => value + 1)} disabled={refreshing} aria-label="Refresh telemetry" title="Refresh telemetry">
            <RefreshCw size={15} className={refreshing ? "telemetry-refreshing" : ""} />
          </button>
        </div>
      </div>

      <div className="telemetry-meta-row">
        <span><i className="telemetry-live-dot" /> SOURCE: {backend}</span>
        <span>{count} {view === "Metrics" ? "points" : view === "Logs" ? "log entries" : "traces"}</span>
        <span><Clock3 size={12} /> {queriedAt ? `Queried ${formatClock(queriedAt)}` : "Waiting for first query"}</span>
      </div>

      {error && (
        <div className="telemetry-error" role="alert">
          <AlertCircle size={16} />
          <div><strong>Telemetry query failed</strong><span>{error}</span></div>
          <button className="text-action" onClick={() => setReloadKey((value) => value + 1)}>Retry</button>
        </div>
      )}

      {view === "Metrics" && (
        <div className="panel telemetry-content-panel">
          {loading && !metrics ? <TelemetryLoading /> : metricSamples.length === 0 ? (
            <TelemetryEmpty title="No metric samples in this window" message="Generate traffic from the instrumented demo service, then refresh. The current API exposes its request-rate counter." />
          ) : (
            <>
              <div className="telemetry-stat-row">
                <div><span className="telemetry-kicker">REQUEST RATE</span><strong>{formatRate(metricSamples[metricSamples.length - 1].value)}<small> req/s</small></strong><p>Latest value returned by Prometheus</p></div>
                <div className="telemetry-stat-secondary"><span className="telemetry-kicker">SAMPLES</span><strong>{metricSamples.length}</strong><p>Across {windowRange}</p></div>
                <div className="telemetry-stat-secondary"><span className="telemetry-kicker">QUERY STEP</span><strong>{metrics?.step ?? "—"}</strong><p>Prometheus resolution</p></div>
              </div>
              <div className="telemetry-chart">
                {chartPoints ? (
                  <svg viewBox="0 0 600 120" role="img" aria-label="Request rate over the selected time window">
                    <defs><linearGradient id="telemetryMetricFill" x1="0" x2="0" y1="0" y2="1"><stop offset="0%" stopColor="#65d9c1" stopOpacity=".2" /><stop offset="100%" stopColor="#65d9c1" stopOpacity="0" /></linearGradient></defs>
                    <path d={`M ${chartPoints.replace(/ /g, " L ")} L 592 116 L 8 116 Z`} fill="url(#telemetryMetricFill)" />
                    <polyline points={chartPoints} fill="none" stroke="#65d9c1" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                ) : <TelemetryEmpty title="Not enough samples to draw a trend" message="Prometheus returned a single point. More scrapes will create a trend line." />}
              </div>
              <div className="telemetry-data-footer"><span>Query: <code>{metrics?.query}</code></span><span>Window: {windowRange}</span></div>
            </>
          )}
        </div>
      )}

      {view === "Logs" && (
        <div className="panel telemetry-content-panel">
          {loading && !logs ? <TelemetryLoading /> : logRows.length === 0 ? (
            <TelemetryEmpty title="No logs found" message="Generate a request at the telemetry demo /emit endpoint and refresh this view." />
          ) : (
            <div className="telemetry-log-list">
              {logRows.map((entry, index) => (
                <article className="telemetry-log-row" key={`${entry.timestamp}-${index}`}>
                  <time>{formatClock(Number(entry.timestamp) / 1_000_000)}</time>
                  <span className="telemetry-log-level">INFO</span>
                  <code>{entry.line}</code>
                  <small>{entry.labels.service_name ?? "unknown-service"}</small>
                </article>
              ))}
            </div>
          )}
        </div>
      )}

      {view === "Traces" && (
        <div className="panel telemetry-content-panel">
          {loading && !traces ? <TelemetryLoading /> : traceRows.length === 0 ? (
            <TelemetryEmpty title="No traces found" message="Send a request through the instrumented demo workload and refresh this view." />
          ) : (
            <div className="telemetry-trace-list">
              {traceRows.map((trace) => (
                <article className="telemetry-trace-row" key={trace.traceID}>
                  <div className="telemetry-trace-icon"><Workflow size={16} /></div>
                  <div className="telemetry-trace-main">
                    <strong>{trace.rootTraceName || "Trace"}</strong>
                    <span>{trace.rootServiceName || trace.service || "unknown-service"}</span>
                    <code>{trace.traceID}</code>
                  </div>
                  <div className="telemetry-trace-side">
                    <strong>{Number.isFinite(trace.durationMs) ? `${trace.durationMs.toFixed(1)} ms` : "—"}</strong>
                    <span>{trace.startTimeUnixNano ? formatClock(nanoTimestamp(trace.startTimeUnixNano)) : "Time unavailable"}</span>
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="telemetry-source-note">
        <span>Data source: {backend}</span>
        <span>Auto-refresh every 30 seconds · API timestamps preserved</span>
      </div>
    </section>
  );
}

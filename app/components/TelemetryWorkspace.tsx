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
  listTelemetryServices,
} from "../../lib/orbit/client";
import { formatLocale, navLabel, t, type Language, type TranslationKey } from "../../lib/orbit/i18n";
import type {
  TelemetryLogsResponse,
  TelemetryMetricsResponse,
  TelemetryTracesResponse,
  TelemetryWindow,
  DiscoveredTelemetryService,
} from "../../lib/orbit/contracts";

export type TelemetryView = "Metrics" | "Logs" | "Traces";

const windows: { value: TelemetryWindow; label: string }[] = [
  { value: "15m", label: "15 min" },
  { value: "1h", label: "1 hour" },
  { value: "6h", label: "6 hours" },
  { value: "24h", label: "24 hours" },
];

function formatClock(timestamp: number | string, language: Language): string {
  const numericTimestamp = typeof timestamp === "string" ? Number(timestamp) : timestamp;
  const value = typeof timestamp === "number"
    ? timestamp
    : timestamp.trim() !== "" && Number.isFinite(numericTimestamp)
      ? numericTimestamp
      : Date.parse(timestamp);
  if (!Number.isFinite(value)) return t(language, "timeUnavailable");
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return t(language, "timeUnavailable");
  return new Intl.DateTimeFormat(formatLocale(language), {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}

function formatRate(value: number, language: Language): string {
  if (!Number.isFinite(value)) return "—";
  const formatter = new Intl.NumberFormat(formatLocale(language), { maximumFractionDigits: value >= 10 ? 1 : 2, minimumFractionDigits: value >= 10 ? 1 : 2 });
  if (Math.abs(value) >= 1000) return `${formatter.format(value / 1000)}k`;
  return formatter.format(value);
}

function nanoTimestamp(value: string): number {
  const timestamp = Number(value);
  return Number.isFinite(timestamp) ? timestamp / 1_000_000 : Number.NaN;
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

function TelemetryLoading({ language }: { language: Language }) {
  return (
    <div className="telemetry-loading" role="status" aria-live="polite">
      <span className="telemetry-spinner" />
      <span>{t(language, "liveQuery")} {language === "fa" ? "از زیرساخت‌های تله‌متری…" : "telemetry backends…"}</span>
    </div>
  );
}

export default function TelemetryWorkspace({
  view,
  apiBaseUrl,
  language,
}: {
  view: TelemetryView;
  apiBaseUrl: string;
  language: Language;
}) {
  const tr = (key: TranslationKey) => t(language, key);
  const [windowRange, setWindowRange] = useState<TelemetryWindow>("1h");
  const [selectedService, setSelectedService] = useState("orbit-telemetry-demo");
  const [discoveredServices, setDiscoveredServices] = useState<DiscoveredTelemetryService[]>([]);
  const [serviceWarnings, setServiceWarnings] = useState<string[]>([]);
  const [serviceError, setServiceError] = useState("");
  const [serviceLoading, setServiceLoading] = useState(true);
  const [metrics, setMetrics] = useState<TelemetryMetricsResponse | null>(null);
  const [logs, setLogs] = useState<TelemetryLogsResponse | null>(null);
  const [traces, setTraces] = useState<TelemetryTracesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [reloadKey, setReloadKey] = useState(0);
  const windowOptions = windows.map((item) => ({ ...item, label: language === "fa" ? ({ "15 min": "۱۵ دقیقه", "1 hour": "۱ ساعت", "6 hours": "۶ ساعت", "24 hours": "۲۴ ساعت" }[item.label] ?? item.label) : item.label }));


  useEffect(() => {
    const controller = new AbortController();
    let mounted = true;

    const loadServices = async () => {
      setServiceLoading(true);
      setServiceError("");
      try {
        const response = await listTelemetryServices(windowRange, controller.signal, apiBaseUrl);
        if (!mounted) return;
        setDiscoveredServices(response.data);
        setServiceWarnings(response.warnings ?? []);
        if (response.data.length > 0) {
          setSelectedService((current) => {
            if (response.data.some((service) => service.name === current)) return current;
            const demo = response.data.find((service) => service.name === "orbit-telemetry-demo");
            return demo?.name ?? response.data[0].name;
          });
        }
      } catch (cause) {
        if (controller.signal.aborted) return;
        if (mounted) {
          setServiceError(cause instanceof Error ? cause.message : "Unable to discover telemetry services.");
          setServiceWarnings([]);
        }
      } finally {
        if (mounted) setServiceLoading(false);
      }
    };

    void loadServices();
    const interval = window.setInterval(() => void loadServices(), 60_000);
    return () => {
      mounted = false;
      controller.abort();
      window.clearInterval(interval);
    };
  }, [windowRange, apiBaseUrl, reloadKey]);

  useEffect(() => {
    const controller = new AbortController();
    let mounted = true;

    const load = async (initial: boolean) => {
      if (initial) {
        setLoading(true);
        if (view === "Metrics") setMetrics(null);
        else if (view === "Logs") setLogs(null);
        else setTraces(null);
      } else setRefreshing(true);
      setError("");
      try {
        if (view === "Metrics") {
          const response = await getTelemetryMetrics(windowRange, controller.signal, apiBaseUrl, selectedService);
          if (mounted) setMetrics(response);
        } else if (view === "Logs") {
          const response = await listTelemetryLogs(
            { window: windowRange, service: selectedService, limit: 50 },
            controller.signal,
            apiBaseUrl,
          );
          if (mounted) setLogs(response);
        } else {
          const response = await listTelemetryTraces(
            { window: windowRange, service: selectedService, limit: 50 },
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
  }, [view, windowRange, apiBaseUrl, reloadKey, selectedService]);

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
            <h2 id="telemetry-title">{navLabel(language, view)}</h2>
            <p>{tr("liveQuery")} · {backend} · {selectedService}</p>
          </div>
        </div>
        <label className="telemetry-service-select">
          <span>{tr("service")}</span>
          <select
            value={selectedService}
            onChange={(event) => setSelectedService(event.target.value)}
            disabled={serviceLoading || discoveredServices.length === 0}
            aria-label={tr("selectService")}
          >
            {discoveredServices.length > 0
              ? discoveredServices.map((service) => (
                  <option key={service.name} value={service.name}>
                    {service.name} · {service.sources.join(", ")}
                  </option>
                ))
              : <option value={selectedService}>{selectedService} · default</option>}
          </select>
          <small>
            {serviceLoading ? tr("discoveringServices") :
              discoveredServices.length > 0 ? `${discoveredServices.length} ${tr("discovered")}` : tr("noDiscoveredServices")}
          </small>
        </label>
        <div className="telemetry-controls">
          <div className="range-switcher" role="group" aria-label={tr("windowLabel")}>
            {windowOptions.map((item) => (
              <button key={item.value} className={windowRange === item.value ? "selected" : ""} aria-pressed={windowRange === item.value} onClick={() => setWindowRange(item.value)}>
                {item.label}
              </button>
            ))}
          </div>
          <button className="icon-button" onClick={() => setReloadKey((value) => value + 1)} disabled={refreshing} aria-label={tr("refresh")} title={tr("refresh")}>
            <RefreshCw size={15} className={refreshing ? "telemetry-refreshing" : ""} />
          </button>
        </div>
      </div>

      <div className="telemetry-meta-row">
        <span><i className="telemetry-live-dot" /> {tr("sourceLabel")}: {backend}</span>
        <span>{count} {view === "Metrics" ? tr("points") : view === "Logs" ? tr("logEntries") : tr("tracesCount")}</span>
        <span><Clock3 size={12} /> {queriedAt ? `${tr("queried")} ${formatClock(queriedAt, language)}` : tr("waitingFirstQuery")}</span>
      </div>

      {(serviceError || serviceWarnings.length > 0) && (
        <div className="telemetry-service-warning" role="status">
          <AlertCircle size={14} />
          <span>{serviceError || `${language === "fa" ? "شناسایی سرویس‌ها ناقص است:" : "Partial service discovery:"} ${serviceWarnings.join(", ")}`}</span>
          <button className="text-action" onClick={() => setReloadKey((value) => value + 1)}>{tr("retry")}</button>
        </div>
      )}

      {error && (
        <div className="telemetry-error" role="alert">
          <AlertCircle size={16} />
          <div><strong>{tr("queryFailed")}</strong><span>{error}</span></div>
          <button className="text-action" onClick={() => setReloadKey((value) => value + 1)}>{tr("retry")}</button>
        </div>
      )}

      {view === "Metrics" && (
        <div className="panel telemetry-content-panel">
          {loading && !metrics ? <TelemetryLoading language={language} /> : metricSamples.length === 0 ? (
            <TelemetryEmpty title={tr("noMetricSamplesTitle")} message={language === "fa" ? tr("noMetricSamplesMessage").replace("این سرویس", selectedService) : tr("noMetricSamplesMessage").replace("this service", selectedService)} />
          ) : (
            <>
              <div className="telemetry-stat-row">
                <div><span className="telemetry-kicker">{tr("requestRate")}</span><strong>{formatRate(metricSamples[metricSamples.length - 1].value, language)}<small> req/s</small></strong><p>{tr("latestPrometheus")}</p></div>
                <div className="telemetry-stat-secondary"><span className="telemetry-kicker">{tr("samples")}</span><strong>{metricSamples.length}</strong><p>{tr("acrossWindow")} {windowRange}</p></div>
                <div className="telemetry-stat-secondary"><span className="telemetry-kicker">{tr("queryStep")}</span><strong>{metrics?.step ?? "—"}</strong><p>{tr("prometheusResolution")}</p></div>
              </div>
              <div className="telemetry-chart">
                {chartPoints ? (
                  <svg viewBox="0 0 600 120" role="img" aria-label={`${tr("requestRate")} · ${windowRange}`}>
                    <defs><linearGradient id="telemetryMetricFill" x1="0" x2="0" y1="0" y2="1"><stop offset="0%" stopColor="#65d9c1" stopOpacity=".2" /><stop offset="100%" stopColor="#65d9c1" stopOpacity="0" /></linearGradient></defs>
                    <path d={`M ${chartPoints.replace(/ /g, " L ")} L 592 116 L 8 116 Z`} fill="url(#telemetryMetricFill)" />
                    <polyline points={chartPoints} fill="none" stroke="#65d9c1" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                ) : <TelemetryEmpty title={tr("notEnoughSamples")} message={tr("singleSample")} />}
              </div>
              <div className="telemetry-data-footer"><span>{tr("queryLabel")}: <code>{metrics?.query}</code></span><span>{tr("windowLabel")}: {windowRange}</span></div>
            </>
          )}
        </div>
      )}

      {view === "Logs" && (
        <div className="panel telemetry-content-panel">
          {loading && !logs ? <TelemetryLoading language={language} /> : logRows.length === 0 ? (
            <TelemetryEmpty title={tr("noLogsTitle")} message={language === "fa" ? tr("noLogsMessage").replace("این سرویس", selectedService) : tr("noLogsMessage").replace("this service", selectedService)} />
          ) : (
            <div className="telemetry-log-list">
              {logRows.map((entry, index) => (
                <article className="telemetry-log-row" key={`${entry.timestamp}-${index}`}>
                  <time>{formatClock(Number(entry.timestamp) / 1_000_000, language)}</time>
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
          {loading && !traces ? <TelemetryLoading language={language} /> : traceRows.length === 0 ? (
            <TelemetryEmpty title={tr("noTracesTitle")} message={language === "fa" ? tr("noTracesMessage").replace("این سرویس", selectedService) : tr("noTracesMessage").replace("this service", selectedService)} />
          ) : (
            <div className="telemetry-trace-list">
              {traceRows.map((trace) => (
                <article className="telemetry-trace-row" key={trace.traceID}>
                  <div className="telemetry-trace-icon"><Workflow size={16} /></div>
                  <div className="telemetry-trace-main">
                    <strong>{trace.rootTraceName || "Trace"}</strong>
                    <span>{trace.rootServiceName || "unknown-service"}</span>
                    <code>{trace.traceID}</code>
                  </div>
                  <div className="telemetry-trace-side">
                    <strong>{Number.isFinite(trace.durationMs) ? `${trace.durationMs.toFixed(1)} ms` : "—"}</strong>
                    <span>{trace.startTimeUnixNano ? formatClock(nanoTimestamp(trace.startTimeUnixNano), language) : t(language, "timeUnavailable")}</span>
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="telemetry-source-note">
        <span>{tr("dataSource")}: {backend}</span>
        <span>{tr("autoRefresh")}</span>
      </div>
    </section>
  );
}

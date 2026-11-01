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
  getTelemetryServiceAttributes,
} from "../../lib/orbit/client";
import { formatLocale, navLabel, t, type Language, type TranslationKey } from "../../lib/orbit/i18n";
import type {
  TelemetryLogsResponse,
  TelemetryMetricsResponse,
  TelemetryTracesResponse,
  TelemetryWindow,
  DiscoveredTelemetryService,
  TelemetryResourceAttributesResponse,
} from "../../lib/orbit/contracts";

export type TelemetryView = "Metrics" | "Logs" | "Traces";

const windows: { value: TelemetryWindow; label: string }[] = [
  { value: "15m", label: "15 min" },
  { value: "1h", label: "1 hour" },
  { value: "6h", label: "6 hours" },
  { value: "24h", label: "24 hours" },
];

const metricGroupOptions = [
  { value: "instance", label: "groupInstance" },
  { value: "service_version", label: "groupVersion" },
  { value: "deployment_environment", label: "groupEnvironment" },
] as const;

const metricSeriesColors = ["#65d9c1", "#8db8e9", "#b6a4eb", "#e6b87d", "#e59c9c"];

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

function parseStepMilliseconds(step?: string): number {
  const match = /^(\\d+)(s|m)$/.exec(step ?? "");
  if (!match) return 60_000;
  return Number(match[1]) * (match[2] === "m" ? 60_000 : 1_000);
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
  const [metricGroupBy, setMetricGroupBy] = useState<string[]>(["instance", "service_version", "deployment_environment"]);
  const [discoveredServices, setDiscoveredServices] = useState<DiscoveredTelemetryService[]>([]);
  const [serviceWarnings, setServiceWarnings] = useState<string[]>([]);
  const [serviceError, setServiceError] = useState("");
  const [serviceLoading, setServiceLoading] = useState(true);
  const [serviceCursor, setServiceCursor] = useState<string | null>(null);
  const [serviceHasMore, setServiceHasMore] = useState(false);
  const [serviceLoadingMore, setServiceLoadingMore] = useState(false);
  const [resourceAttributes, setResourceAttributes] = useState<TelemetryResourceAttributesResponse | null>(null);
  const [resourceLoading, setResourceLoading] = useState(true);
  const [resourceError, setResourceError] = useState("");
  const [metrics, setMetrics] = useState<TelemetryMetricsResponse | null>(null);
  const [logs, setLogs] = useState<TelemetryLogsResponse | null>(null);
  const [traces, setTraces] = useState<TelemetryTracesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [reloadKey, setReloadKey] = useState(0);
  const windowOptions = windows.map((item) => ({ ...item, label: language === "fa" ? (item.value === "15m" ? "۱۵ دقیقه" : item.value === "1h" ? "۱ ساعت" : item.value === "6h" ? "۶ ساعت" : "۲۴ ساعت") : item.label }));


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
        setServiceCursor(response.next_cursor ?? null);
        setServiceHasMore(response.has_more ?? false);
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
          setServiceError(cause instanceof Error ? cause.message : language === "fa" ? "دریافت فهرست سرویس‌ها ناموفق بود." : "Unable to discover telemetry services.");
          setServiceWarnings([]);
          setServiceCursor(null);
          setServiceHasMore(false);
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
  }, [windowRange, apiBaseUrl, reloadKey, language]);

  const loadMoreServices = async () => {
    if (!serviceCursor || !serviceHasMore || serviceLoadingMore) return;
    const controller = new AbortController();
    setServiceLoadingMore(true);
    try {
      const response = await listTelemetryServices(windowRange, controller.signal, apiBaseUrl, serviceCursor, 50);
      setDiscoveredServices((current) => {
        const merged = new Map(current.map((service) => [service.name, service]));
        for (const service of response.data) merged.set(service.name, service);
        return Array.from(merged.values()).sort((a, b) => a.name.localeCompare(b.name));
      });
      setServiceCursor(response.next_cursor ?? null);
      setServiceHasMore(response.has_more ?? false);
      setServiceWarnings(response.warnings ?? []);
    } catch (cause) {
      setServiceError(cause instanceof Error ? cause.message : language === "fa" ? "بارگذاری سرویس‌های بیشتر ناموفق بود." : "Unable to load more services.");
    } finally {
      setServiceLoadingMore(false);
    }
  };

  useEffect(() => {
    const controller = new AbortController();
    let mounted = true;

    const loadAttributes = async () => {
      setResourceLoading(true);
      setResourceError("");
      try {
        const response = await getTelemetryServiceAttributes(
          selectedService,
          windowRange,
          controller.signal,
          apiBaseUrl,
        );
        if (mounted) setResourceAttributes(response);
      } catch (cause) {
        if (controller.signal.aborted) return;
        if (mounted) {
          setResourceAttributes(null);
          setResourceError(cause instanceof Error ? cause.message : tr("resourceAttributeUnavailable"));
        }
      } finally {
        if (mounted) setResourceLoading(false);
      }
    };

    void loadAttributes();
    const interval = window.setInterval(() => void loadAttributes(), 60_000);
    return () => {
      mounted = false;
      controller.abort();
      window.clearInterval(interval);
    };
  }, [selectedService, windowRange, apiBaseUrl, reloadKey, language]);

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
          const groupBy = metricGroupBy.length > 0 ? [...metricGroupBy].sort().join(",") : "none";
          const response = await getTelemetryMetrics(windowRange, controller.signal, apiBaseUrl, selectedService, groupBy);
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
          setError(cause instanceof Error ? cause.message : language === "fa" ? "دریافت داده‌های تله‌متری ناموفق بود." : "Unable to query telemetry.");
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
  }, [view, windowRange, apiBaseUrl, reloadKey, selectedService, language, metricGroupBy]);

  const metricSeries = useMemo(() => {
    if (!metrics) return [];
    return metrics.data.map((series, index) => {
      const points = series.values
        .map(([timestamp, value]) => ({ timestamp, value: Number(value) }))
        .filter((point) => Number.isFinite(point.value))
        .sort((a, b) => a.timestamp - b.timestamp);
      const dimensions = Object.entries(series.metric)
        .filter(([key, value]) => key !== "service_name" && value !== "")
        .sort(([a], [b]) => a.localeCompare(b));
      const label = dimensions.length > 0
        ? dimensions.map(([key, value]) => `${key}=${value}`).join(" · ")
        : tr("groupAggregate");
      return {
        key: dimensions.map(([key, value]) => `${key}=${value}`).join("|") || `aggregate-${index}`,
        label,
        points,
      };
    }).filter((series) => series.points.length > 0);
  }, [metrics, language]);

  const metricSamples = useMemo(
    () => metricSeries.flatMap((series) => series.points).sort((a, b) => a.timestamp - b.timestamp),
    [metricSeries],
  );

  const chartSeries = useMemo(() => {
    if (metricSeries.length === 0) return [];
    const allPoints = metricSeries.flatMap((series) => series.points);
    const minTimestamp = Math.min(...allPoints.map((point) => point.timestamp));
    const maxTimestamp = Math.max(...allPoints.map((point) => point.timestamp));
    const timeSpan = maxTimestamp - minTimestamp || 1;
    const minValue = Math.min(0, ...allPoints.map((point) => point.value));
    const maxValue = Math.max(...allPoints.map((point) => point.value));
    const valueSpan = maxValue - minValue || 1;

    return metricSeries.map((series, index) => ({
      ...series,
      color: metricSeriesColors[index % metricSeriesColors.length],
      points: series.points.map((point) => ({
        x: 8 + ((point.timestamp - minTimestamp) / timeSpan) * 584,
        y: 108 - ((point.value - minValue) / valueSpan) * 92,
      })),
    }));
  }, [metricSeries]);

  const metricQuality = useMemo(() => {
    const allPoints = metricSeries.flatMap((series) => series.points);
    if (allPoints.length === 0) return { stale: false, hasGaps: false };

    const expectedStepMs = parseStepMilliseconds(metrics?.step);
    const thresholdMs = Math.max(expectedStepMs * 2.5, 90_000);
    const latestTimestampMs = Math.max(...allPoints.map((point) => point.timestamp)) * 1_000;
    const stale = Date.now() - latestTimestampMs > thresholdMs;
    const hasGaps = metricSeries.some((series) =>
      series.points.some((point, index) =>
        index > 0 && (point.timestamp - series.points[index - 1].timestamp) * 1_000 > thresholdMs,
      ),
    );
    return { stale, hasGaps };
  }, [metricSeries, metrics?.step]);

  const latestMetricRate = metricSeries.reduce((total, series) => {
    const latest = series.points[series.points.length - 1];
    return total + (latest?.value ?? 0);
  }, 0);

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
        {serviceHasMore ? (
          <button className="telemetry-load-more" type="button" onClick={() => void loadMoreServices()} disabled={serviceLoadingMore || serviceLoading}>
            {serviceLoadingMore ? tr("loadingMoreServices") : tr("loadMoreServices")}
          </button>
        ) : null}
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
      <details className="telemetry-attributes-panel">
        <summary>
          <span>{tr("resourceAttributes")}</span>
          <small>{resourceLoading ? tr("loadingResourceAttributes") : Object.keys(resourceAttributes?.attributes ?? {}).length}</small>
        </summary>
        {resourceError ? (
          <p className="telemetry-attributes-empty" role="status">{resourceError}</p>
        ) : resourceLoading && !resourceAttributes ? (
          <p className="telemetry-attributes-empty">{tr("loadingResourceAttributes")}</p>
        ) : Object.keys(resourceAttributes?.attributes ?? {}).length === 0 ? (
          <p className="telemetry-attributes-empty">{tr("noResourceAttributes")}</p>
        ) : (
          <div className="telemetry-attributes-grid">
            {Object.entries(resourceAttributes?.attributes ?? {}).sort(([a], [b]) => a.localeCompare(b)).slice(0, 16).map(([key, value]) => (
              <div className="telemetry-attribute-item" key={key}>
                <div><code>{key}</code><strong>{typeof value === "string" ? value : JSON.stringify(value)}</strong></div>
                <small>{(resourceAttributes?.attribute_sources[key] ?? []).join(", ")}</small>
              </div>
            ))}
          </div>
        )}
        {resourceAttributes?.warnings && resourceAttributes.warnings.length > 0 ? (
          <p className="telemetry-attributes-warning">{resourceAttributes.warnings.join(" · ")}</p>
        ) : null}
      </details>

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

      {view === "Metrics" && (metricQuality.stale || metricQuality.hasGaps) && (
        <div className="telemetry-freshness-warning" role="status" aria-live="polite">
          <Clock3 size={14} />
          <span>
            <strong>{tr("metricFreshness")}</strong>
            {" "}
            {[
              metricQuality.stale ? tr("staleMetricWarning") : "",
              metricQuality.hasGaps ? tr("metricGapWarning") : "",
            ].filter(Boolean).join(" · ")}
          </span>
        </div>
      )}

      {view === "Metrics" && (
        <div className="telemetry-grouping-panel" role="group" aria-label={tr("groupBy")}>
          <span className="telemetry-kicker">{tr("groupBy")}</span>
          <button
            type="button"
            className={metricGroupBy.length === 0 ? "selected" : ""}
            aria-pressed={metricGroupBy.length === 0}
            onClick={() => setMetricGroupBy([])}
          >
            {tr("groupAggregate")}
          </button>
          <button
            type="button"
            className={metricGroupBy.length === metricGroupOptions.length && metricGroupOptions.every((item) => metricGroupBy.includes(item.value)) ? "selected" : ""}
            aria-pressed={metricGroupBy.length === metricGroupOptions.length && metricGroupOptions.every((item) => metricGroupBy.includes(item.value))}
            onClick={() => setMetricGroupBy(metricGroupOptions.map((item) => item.value))}
          >
            {tr("groupDetailed")}
          </button>
          {metricGroupOptions.map((item) => (
            <button
              key={item.value}
              type="button"
              className={metricGroupBy.includes(item.value) ? "selected" : ""}
              aria-pressed={metricGroupBy.includes(item.value)}
              onClick={() => setMetricGroupBy((current) => current.includes(item.value)
                ? current.filter((value) => value !== item.value)
                : [...current, item.value])}
            >
              {tr(item.label)}
            </button>
          ))}
        </div>
      )}

      {view === "Metrics" && (
        <div className="panel telemetry-content-panel">
          {loading && !metrics ? <TelemetryLoading language={language} /> : metricSamples.length === 0 ? (
            <TelemetryEmpty title={tr("noMetricSamplesTitle")} message={language === "fa" ? tr("noMetricSamplesMessage").replace("این سرویس", selectedService) : tr("noMetricSamplesMessage").replace("this service", selectedService)} />
          ) : (
            <>
              <div className="telemetry-stat-row">
                <div><span className="telemetry-kicker">{tr("requestRate")}</span><strong>{formatRate(latestMetricRate, language)}<small> req/s</small></strong><p>{tr("latestPrometheus")}</p></div>
                <div className="telemetry-stat-secondary"><span className="telemetry-kicker">{tr("samples")}</span><strong>{metricSamples.length}</strong><p>{chartSeries.length} {tr("series")} · {tr("acrossWindow")} {windowRange}</p></div>
                <div className="telemetry-stat-secondary"><span className="telemetry-kicker">{tr("queryStep")}</span><strong>{metrics?.step ?? "—"}</strong><p>{tr("prometheusResolution")}</p></div>
              </div>
              <div className="telemetry-chart">
                {chartSeries.length > 0 ? (
                  <>
                    <svg viewBox="0 0 600 120" role="img" aria-label={`${tr("requestRate")} · ${windowRange}`}>
                      {chartSeries.map((series) => {
                        const points = series.points.map((point) => `${point.x.toFixed(1)},${point.y.toFixed(1)}`).join(" ");
                        return (
                          <g key={series.key}>
                            {series.points.length > 1 ? (
                              <polyline points={points} fill="none" stroke={series.color} strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
                            ) : (
                              <circle cx={series.points[0].x} cy={series.points[0].y} r="4" fill={series.color} />
                            )}
                          </g>
                        );
                      })}
                    </svg>
                    <div className="telemetry-series-legend" aria-label={tr("series")}>
                      {chartSeries.map((series) => (
                        <span key={series.key}>
                          <i style={{ backgroundColor: series.color }} />
                          {series.label}
                        </span>
                      ))}
                    </div>
                  </>
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
                  <span className="telemetry-log-level">{language === "fa" ? "اطلاعات" : "INFO"}</span>
                  <code>{entry.line}</code>
                  <small>{entry.labels.service_name ?? (language === "fa" ? "سرویس نامشخص" : "unknown-service")}</small>
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
                    <strong>{trace.rootTraceName || (language === "fa" ? "ردیابی" : "Trace")}</strong>
                    <span>{trace.rootServiceName || (language === "fa" ? "سرویس نامشخص" : "unknown-service")}</span>
                    <code>{trace.traceID}</code>
                  </div>
                  <div className="telemetry-trace-side">
                    <strong>{Number.isFinite(trace.durationMs) ? `${new Intl.NumberFormat(formatLocale(language), { maximumFractionDigits: 1 }).format(trace.durationMs)} ms` : "—"}</strong>
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

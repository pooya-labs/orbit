"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  AlertCircle,
  Clock3,
  Eye,
  FileText,
  RefreshCw,
  Workflow,
  X,
} from "lucide-react";
import {
  getTelemetryMetrics,
  listTelemetryLogs,
  listTelemetryTraces,
  listTelemetryServices,
  getTelemetryServiceAttributes,
  getWorkspaceMembership,
  getWorkspaceTelemetryMetrics,
  listWorkspaceTelemetryMetricCatalog,
  getWorkspaceTelemetryTrace,
  listWorkspaceTelemetryLogs,
  listWorkspaceTelemetryServices,
  listWorkspaceTelemetryTraces,
  listWorkspaces,
} from "../../lib/orbit/client";
import { formatLocale, navLabel, t, type Language, type TranslationKey } from "../../lib/orbit/i18n";
import { extractTraceIdFromLog } from "../../lib/orbit/trace-correlation.mjs";
import type {
  TelemetryLogsResponse,
  TelemetryMetricsResponse,
  TelemetryMetricDefinition,
  TelemetryMetricID,
  TelemetryTracesResponse,
  TempoTraceSpan,
  TelemetryWindow,
  WorkspaceTraceDetailResponse,
  WorkspaceMembershipSummary,
  WorkspaceSummary,
} from "../../lib/orbit/contracts";
import {
  getCurrentAuthUser,
  isOidcConfigured,
  subscribeToAuth,
  type OrbitAuthUser,
} from "../../lib/orbit/oidc";

export type TelemetryView = "Metrics" | "Logs" | "Traces";

type TelemetryServiceOption = {
  id?: string;
  last_discovered_at?: string;
  name: string;
  sources: string[];
  resource_attributes?: Record<string, unknown>;
  resource_attribute_sources?: Record<string, string[]>;
};

type TelemetryResourceAttributesView = {
  attributes: Record<string, unknown>;
  attribute_sources: Record<string, string[]>;
  warnings?: string[];
};

const windows: { value: TelemetryWindow; label: string }[] = [
  { value: "15m", label: "15 min" },
  { value: "1h", label: "1 hour" },
  { value: "6h", label: "6 hours" },
  { value: "24h", label: "24 hours" },
];

const legacyMetricCatalog: TelemetryMetricDefinition[] = [
  { id: "request_rate", unit: "requests/second", description: "All instrumented HTTP requests per second.", group_by: ["instance", "service_version", "deployment_environment"] },
  { id: "successful_request_rate", unit: "requests/second", description: "Instrumented successful HTTP 2xx and 3xx requests per second.", group_by: ["instance", "service_version", "deployment_environment"] },
  { id: "failed_request_rate", unit: "requests/second", description: "Instrumented failed HTTP requests per second.", group_by: ["instance", "service_version", "deployment_environment"] },
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
  const match = /^(\d+)(s|m)$/.exec(step ?? "");
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
  apiEnvironment,
}: {
  view: TelemetryView;
  apiBaseUrl: string;
  language: Language;
  apiEnvironment: string;
}) {
  const tr = (key: TranslationKey) => t(language, key);
  const [user, setUser] = useState<OrbitAuthUser | null>(() => getCurrentAuthUser());
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([]);
  const [workspaceId, setWorkspaceId] = useState("");
  const [membership, setMembership] = useState<WorkspaceMembershipSummary | null>(null);
  const [membershipWorkspaceId, setMembershipWorkspaceId] = useState("");
  const [workspaceLoading, setWorkspaceLoading] = useState(false);
  const [workspaceError, setWorkspaceError] = useState("");
  const useLegacyDevelopmentReads = apiEnvironment.toLowerCase() === "development" && !isOidcConfigured();
  const [windowRange, setWindowRange] = useState<TelemetryWindow>("1h");
  const [selectedService, setSelectedService] = useState("orbit-telemetry-demo");
  const [metricGroupBy, setMetricGroupBy] = useState<string[]>(["instance", "service_version", "deployment_environment"]);
  const [selectedMetric, setSelectedMetric] = useState<TelemetryMetricID>("request_rate");
  const [metricCatalog, setMetricCatalog] = useState<TelemetryMetricDefinition[]>([]);
  const [metricCatalogLoading, setMetricCatalogLoading] = useState(false);
  const [metricCatalogError, setMetricCatalogError] = useState("");
  const [discoveredServices, setDiscoveredServices] = useState<TelemetryServiceOption[]>([]);
  const [serviceWarnings, setServiceWarnings] = useState<string[]>([]);
  const [serviceError, setServiceError] = useState("");
  const [serviceLoading, setServiceLoading] = useState(true);
  const [serviceCursor, setServiceCursor] = useState<string | null>(null);
  const [serviceHasMore, setServiceHasMore] = useState(false);
  const [serviceLoadingMore, setServiceLoadingMore] = useState(false);
  const servicePageController = useRef<AbortController | null>(null);
  const serviceDiscoveryController = useRef<AbortController | null>(null);
  const [resourceAttributes, setResourceAttributes] = useState<TelemetryResourceAttributesView | null>(null);
  const [resourceLoading, setResourceLoading] = useState(true);
  const [resourceError, setResourceError] = useState("");
  const [metrics, setMetrics] = useState<TelemetryMetricsResponse | null>(null);
  const [logs, setLogs] = useState<TelemetryLogsResponse | null>(null);
  const [traces, setTraces] = useState<TelemetryTracesResponse | null>(null);
  const [selectedTraceId, setSelectedTraceId] = useState("");
  const [traceDetail, setTraceDetail] = useState<WorkspaceTraceDetailResponse | null>(null);
  const [traceDetailLoading, setTraceDetailLoading] = useState(false);
  const [traceDetailError, setTraceDetailError] = useState("");
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [reloadKey, setReloadKey] = useState(0);
  const windowOptions = windows.map((item) => ({ ...item, label: language === "fa" ? (item.value === "15m" ? "۱۵ دقیقه" : item.value === "1h" ? "۱ ساعت" : item.value === "6h" ? "۶ ساعت" : "۲۴ ساعت") : item.label }));


  useEffect(() => subscribeToAuth(() => {
    setUser(getCurrentAuthUser());
    setWorkspaces([]);
    setWorkspaceId("");
    setMembership(null);
    setMembershipWorkspaceId("");
    setDiscoveredServices([]);
    setResourceAttributes(null);
    setSelectedTraceId("");
    setTraceDetail(null);
    setTraceDetailError("");
    setTraceDetailLoading(false);
  }), []);

  useEffect(() => {
    if (useLegacyDevelopmentReads || !user) {
      if (!useLegacyDevelopmentReads) {
        setWorkspaces([]);
        setWorkspaceId("");
        setMembership(null);
        setMembershipWorkspaceId("");
        setServiceLoading(false);
        setResourceLoading(false);
      }
      return;
    }
    const controller = new AbortController();
    let current = true;
    setWorkspaceLoading(true);
    setWorkspaceError("");
    listWorkspaces(controller.signal, apiBaseUrl)
      .then((items) => {
        if (!current) return;
        setWorkspaces(items);
        setWorkspaceId((previous) => (
          previous && items.some((workspace) => workspace.id === previous)
            ? previous
            : items[0]?.id ?? ""
        ));
      })
      .catch((cause) => {
        if (current && !controller.signal.aborted) {
          setWorkspaces([]);
          setWorkspaceId("");
          setWorkspaceError(cause instanceof Error ? cause.message : tr("telemetryWorkspaceLoadFailed"));
        }
      })
      .finally(() => {
        if (current) setWorkspaceLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [apiBaseUrl, language, user?.sub, useLegacyDevelopmentReads]);

  useEffect(() => {
    if (useLegacyDevelopmentReads || !user || !workspaceId) {
      if (!useLegacyDevelopmentReads) {
        setMembership(null);
        setDiscoveredServices([]);
        setServiceLoading(false);
        setServiceError("");
        setServiceWarnings([]);
        setServiceHasMore(false);
        setServiceCursor(null);
        setServiceLoadingMore(false);
        setSelectedService("orbit-telemetry-demo");
      }
      return;
    }
    const controller = new AbortController();
    let current = true;
    setServiceLoading(true);
    setServiceError("");
    setWorkspaceError("");
    setDiscoveredServices([]);
    setServiceHasMore(false);
    setServiceCursor(null);
    setServiceLoadingMore(false);
    setMembership(null);
    setMembershipWorkspaceId("");
    setResourceAttributes(null);
    Promise.all([
      getWorkspaceMembership(workspaceId, controller.signal, apiBaseUrl),
      listWorkspaceTelemetryServices(workspaceId, 100, controller.signal, apiBaseUrl),
    ])
      .then(([role, response]) => {
        if (!current) return;
        const options: TelemetryServiceOption[] = response.data.map((service) => ({
          id: service.id,
          name: service.name,
          sources: service.sources,
          last_discovered_at: service.last_discovered_at,
          resource_attributes: Object.fromEntries(
            Object.entries(service.resource_attributes ?? {}).map(([key, values]) => [
              key,
              values.length === 1 ? values[0] : values,
            ]),
          ),
          resource_attribute_sources: service.resource_attribute_sources,
        }));
        setMembership(role);
        setMembershipWorkspaceId(workspaceId);
        setDiscoveredServices(options);
        setServiceWarnings([]);
        setServiceCursor(null);
        setServiceHasMore(false);
        setSelectedService((currentService) => (
          options.some((service) => service.name === currentService)
            ? currentService
            : options.find((service) => service.name === "orbit-telemetry-demo")?.name
              ?? options[0]?.name
              ?? ""
        ));
      })
      .catch((cause) => {
        if (current && !controller.signal.aborted) {
          setMembership(null);
          setMembershipWorkspaceId("");
          setDiscoveredServices([]);
          setServiceError(cause instanceof Error ? cause.message : tr("telemetryWorkspaceLoadFailed"));
        }
      })
      .finally(() => {
        if (current) setServiceLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [apiBaseUrl, language, user?.sub, workspaceId, reloadKey, useLegacyDevelopmentReads]);

  useEffect(() => {
    if (!useLegacyDevelopmentReads) return;
    let mounted = true;

    const loadServices = async () => {
      servicePageController.current?.abort();
      servicePageController.current = null;
      setServiceLoadingMore(false);

      serviceDiscoveryController.current?.abort();
      const controller = new AbortController();
      serviceDiscoveryController.current = controller;
      setServiceLoading(true);
      setServiceError("");
      try {
        const response = await listTelemetryServices(windowRange, controller.signal, apiBaseUrl);
        if (!mounted || controller.signal.aborted) return;
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
        if (controller.signal.aborted || !mounted) return;
        setServiceError(cause instanceof Error ? cause.message : language === "fa" ? "دریافت فهرست سرویس‌ها ناموفق بود." : "Unable to discover telemetry services.");
        setServiceWarnings([]);
        setServiceCursor(null);
        setServiceHasMore(false);
      } finally {
        if (mounted && serviceDiscoveryController.current === controller) {
          serviceDiscoveryController.current = null;
          setServiceLoading(false);
        }
      }
    };

    void loadServices();
    const interval = window.setInterval(() => void loadServices(), 60_000);
    return () => {
      mounted = false;
      serviceDiscoveryController.current?.abort();
      serviceDiscoveryController.current = null;
      servicePageController.current?.abort();
      servicePageController.current = null;
      window.clearInterval(interval);
    };
  }, [windowRange, apiBaseUrl, reloadKey, language, useLegacyDevelopmentReads]);

  const loadMoreServices = async () => {
    if (!useLegacyDevelopmentReads || !serviceCursor || !serviceHasMore || serviceLoadingMore || serviceLoading) return;
    servicePageController.current?.abort();
    const controller = new AbortController();
    servicePageController.current = controller;
    setServiceLoadingMore(true);
    try {
      const response = await listTelemetryServices(windowRange, controller.signal, apiBaseUrl, serviceCursor, 50);
      if (controller.signal.aborted) return;
      setDiscoveredServices((current) => {
        const merged = new Map(current.map((service) => [service.name, service]));
        for (const service of response.data) merged.set(service.name, service);
        return Array.from(merged.values()).sort((a, b) => a.name.localeCompare(b.name));
      });
      setServiceCursor(response.next_cursor ?? null);
      setServiceHasMore(response.has_more ?? false);
      setServiceWarnings((current) => Array.from(new Set([...current, ...(response.warnings ?? [])])));
    } catch (cause) {
      if (controller.signal.aborted) return;
      setServiceError(cause instanceof Error ? cause.message : language === "fa" ? "بارگذاری سرویس‌های بیشتر ناموفق بود." : "Unable to load more services.");
    } finally {
      if (servicePageController.current === controller) {
        servicePageController.current = null;
        setServiceLoadingMore(false);
      }
    }
  };

  const selectedServiceDiscovered = discoveredServices.some(
    (service) => service.name === selectedService,
  );

  useEffect(() => {
    let mounted = true;
    const controller = new AbortController();

    if (!useLegacyDevelopmentReads) {
      const selected = discoveredServices.find((service) => service.name === selectedService);
      const attributes = selected?.resource_attributes ?? {};
      const sources = selected?.resource_attribute_sources ?? {};
      setResourceAttributes(selected ? {
        attributes,
        attribute_sources: sources,
      } : null);
      setResourceError("");
      setResourceLoading(false);
      return () => {
        mounted = false;
        controller.abort();
      };
    }

    if (serviceLoading || !selectedServiceDiscovered) {
      setResourceAttributes(null);
      setResourceError("");
      setResourceLoading(false);
      return () => {
        mounted = false;
        controller.abort();
      };
    }

    const loadAttributes = async () => {
      setResourceLoading(true);
      setResourceAttributes(null);
      setResourceError("");
      try {
        const response = await getTelemetryServiceAttributes(
          selectedService,
          windowRange,
          controller.signal,
          apiBaseUrl,
        );
        if (mounted && !controller.signal.aborted) setResourceAttributes(response);
      } catch (cause) {
        if (controller.signal.aborted || !mounted) return;
        setResourceAttributes(null);
        setResourceError(cause instanceof Error ? cause.message : tr("resourceAttributeUnavailable"));
      } finally {
        if (mounted && !controller.signal.aborted) setResourceLoading(false);
      }
    };

    void loadAttributes();
    const interval = window.setInterval(() => void loadAttributes(), 60_000);
    return () => {
      mounted = false;
      controller.abort();
      window.clearInterval(interval);
    };
  }, [selectedService, selectedServiceDiscovered, discoveredServices, serviceLoading, windowRange, apiBaseUrl, reloadKey, language, useLegacyDevelopmentReads]);

  useEffect(() => {
    if (useLegacyDevelopmentReads) {
      setMetricCatalog(legacyMetricCatalog);
      setMetricCatalogLoading(false);
      setMetricCatalogError("");
      return;
    }
    if (!user || !workspaceId || !membership || membershipWorkspaceId !== workspaceId) {
      setMetricCatalog([]);
      setMetricCatalogLoading(false);
      setMetricCatalogError("");
      return;
    }

    const controller = new AbortController();
    let mounted = true;
    setMetricCatalogLoading(true);
    setMetricCatalogError("");
    listWorkspaceTelemetryMetricCatalog(workspaceId, controller.signal, apiBaseUrl)
      .then((response) => {
        if (!mounted || controller.signal.aborted) return;
        setMetricCatalog(response.data);
        setSelectedMetric((current) => response.data.some((metric) => metric.id === current)
          ? current
          : response.data[0]?.id ?? "request_rate");
      })
      .catch(() => {
        if (!mounted || controller.signal.aborted) return;
        setMetricCatalog([]);
        setMetricCatalogError(tr("metricCatalogUnavailable"));
      })
      .finally(() => {
        if (mounted && !controller.signal.aborted) setMetricCatalogLoading(false);
      });

    return () => {
      mounted = false;
      controller.abort();
    };
  }, [apiBaseUrl, language, membership, membershipWorkspaceId, user?.sub, useLegacyDevelopmentReads, workspaceId]);

  useEffect(() => {
    const controller = new AbortController();
    let mounted = true;

    if (!useLegacyDevelopmentReads && (!user || !workspaceId || !membership || membershipWorkspaceId !== workspaceId || !selectedService)) {
      setLoading(false);
      setRefreshing(false);
      setMetrics(null);
      setLogs(null);
      setTraces(null);
      return () => {
        mounted = false;
        controller.abort();
      };
    }

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
          const response = useLegacyDevelopmentReads
            ? await getTelemetryMetrics(windowRange, controller.signal, apiBaseUrl, selectedService, groupBy, selectedMetric)
            : await getWorkspaceTelemetryMetrics(workspaceId, windowRange, selectedService, groupBy, controller.signal, apiBaseUrl, selectedMetric);
          if (mounted) setMetrics(response);
        } else if (view === "Logs") {
          const options = { window: windowRange, service: selectedService, limit: 50 };
          const response = useLegacyDevelopmentReads
            ? await listTelemetryLogs(options, controller.signal, apiBaseUrl)
            : await listWorkspaceTelemetryLogs(workspaceId, options, controller.signal, apiBaseUrl);
          if (mounted) setLogs(response);
        } else {
          const options = { window: windowRange, service: selectedService, limit: 50 };
          const response = useLegacyDevelopmentReads
            ? await listTelemetryTraces(options, controller.signal, apiBaseUrl)
            : await listWorkspaceTelemetryTraces(workspaceId, options, controller.signal, apiBaseUrl);
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
  }, [view, windowRange, apiBaseUrl, reloadKey, selectedService, language, metricGroupBy, selectedMetric, useLegacyDevelopmentReads, user?.sub, workspaceId, membership, membershipWorkspaceId]);

  useEffect(() => {
    if (!selectedTraceId) {
      setTraceDetail(null);
      setTraceDetailError("");
      setTraceDetailLoading(false);
      return;
    }
    if (useLegacyDevelopmentReads || !user || !workspaceId || !membership || membershipWorkspaceId !== workspaceId) {
      setTraceDetail(null);
      setTraceDetailError(tr("traceDetailsFailed"));
      setTraceDetailLoading(false);
      return;
    }

    const controller = new AbortController();
    let mounted = true;
    setTraceDetail(null);
    setTraceDetailError("");
    setTraceDetailLoading(true);
    getWorkspaceTelemetryTrace(workspaceId, selectedTraceId, controller.signal, apiBaseUrl)
      .then((response) => {
        if (mounted && !controller.signal.aborted) setTraceDetail(response);
      })
      .catch(() => {
        if (mounted && !controller.signal.aborted) {
          setTraceDetail(null);
          setTraceDetailError(tr("traceDetailsFailed"));
        }
      })
      .finally(() => {
        if (mounted && !controller.signal.aborted) setTraceDetailLoading(false);
      });

    return () => {
      mounted = false;
      controller.abort();
    };
  }, [apiBaseUrl, language, membership, membershipWorkspaceId, selectedTraceId, user?.sub, useLegacyDevelopmentReads, workspaceId]);

  useEffect(() => {
    if (view !== "Traces" && view !== "Logs" && selectedTraceId) {
      setSelectedTraceId("");
      setTraceDetail(null);
      setTraceDetailError("");
      setTraceDetailLoading(false);
    }
  }, [view, selectedTraceId]);

  const metricSeries = useMemo(() => {
    if (!metrics) return [];
    return metrics.data.map((series, index) => {
      const points = series.values
        .map(([timestamp, value]) => ({ timestamp, value: Number(value) }))
        .filter((point) => Number.isFinite(point.value))
        .sort((a, b) => a.timestamp - b.timestamp);
      const dimensions = Object.entries(series.metric)
        .filter(([key, value]) => key !== "service_name" && key !== "orbit_workspace_id" && value !== "")
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

  const selectedMetricLabel = selectedMetric === "successful_request_rate"
    ? tr("successfulRequestRate")
    : selectedMetric === "failed_request_rate"
      ? tr("failedRequestRate")
      : tr("requestRate");
  const selectedMetricDescription = selectedMetric === "successful_request_rate"
    ? tr("successfulRequestRateDescription")
    : selectedMetric === "failed_request_rate"
      ? tr("failedRequestRateDescription")
      : tr("requestRateDescription");

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

  const accessibleMetricRows = useMemo(
    () => metricSeries.flatMap((series) =>
      series.points.map((point) => ({
        key: series.key,
        series: series.label,
        timestamp: point.timestamp,
        value: point.value,
      })),
    ).sort((a, b) => a.timestamp - b.timestamp || a.series.localeCompare(b.series)),
    [metricSeries],
  );

  const metricQuality = useMemo(() => {
    const allPoints = metricSeries.flatMap((series) => series.points);
    if (allPoints.length === 0) return { stale: false, hasGaps: false };

    const expectedStepMs = parseStepMilliseconds(metrics?.step);
    const thresholdMs = Math.max(expectedStepMs * 2.5, 90_000);
    const stale = metricSeries.some((series) => {
      const latest = series.points[series.points.length - 1];
      return latest !== undefined && Date.now() - latest.timestamp * 1_000 > thresholdMs;
    });
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
        traceId: extractTraceIdFromLog(line, stream.stream),
      })),
    ).sort((a, b) => Number(b.timestamp) - Number(a.timestamp));
  }, [logs]);

  const traceRows = traces?.data ?? [];
  const traceSpanRows = useMemo(() => {
    const spans: TempoTraceSpan[] = traceDetail?.data.batches.flatMap((batch) =>
      (batch.scopeSpans ?? []).flatMap((scope) => scope.spans ?? []),
    ) ?? [];
    const byId = new Map(spans.map((span) => [span.spanId, span]));
    const parseNanos = (value: string): bigint | null => {
      if (!/^\d+$/.test(value)) return null;
      try {
        return BigInt(value);
      } catch {
        return null;
      }
    };
    const ordered = [...spans].sort((left, right) => {
      const leftStart = parseNanos(left.startTimeUnixNano);
      const rightStart = parseNanos(right.startTimeUnixNano);
      if (leftStart === null) return 1;
      if (rightStart === null) return -1;
      return leftStart < rightStart ? -1 : leftStart > rightStart ? 1 : 0;
    });

    return ordered.map((span) => {
      let depth = 0;
      let parentSpanId = span.parentSpanId;
      const visited = new Set<string>();
      while (parentSpanId && byId.has(parentSpanId) && !visited.has(parentSpanId) && depth < 6) {
        visited.add(parentSpanId);
        depth += 1;
        parentSpanId = byId.get(parentSpanId)?.parentSpanId;
      }
      const start = parseNanos(span.startTimeUnixNano);
      const end = parseNanos(span.endTimeUnixNano);
      const durationMs = start !== null && end !== null && end >= start
        ? Number(end - start) / 1_000_000
        : Number.NaN;
      return { span, depth, durationMs };
    });
  }, [traceDetail]);
  const renderTraceDetails = (panelId: string) => (
    <section id={panelId} className="telemetry-trace-detail" hidden={!selectedTraceId} aria-labelledby={`${panelId}-title`} aria-live="polite">
        <header className="telemetry-trace-detail-header">
          <div>
            <h3 id={`${panelId}-title`}>{tr("traceDetails")}</h3>
            <code>{traceDetail?.trace_id ?? selectedTraceId}</code>
          </div>
          <button type="button" onClick={() => setSelectedTraceId("")} aria-label={tr("closeTraceDetails")}>
            <X size={14} />
            <span>{tr("closeTraceDetails")}</span>
          </button>
        </header>
        {traceDetailLoading ? (
          <p className="telemetry-trace-detail-state" role="status">{tr("traceDetailsLoading")}</p>
        ) : traceDetailError ? (
          <p className="telemetry-trace-detail-state" role="alert">{traceDetailError}</p>
        ) : !traceDetail || traceSpanRows.length === 0 ? (
          <p className="telemetry-trace-detail-state">{tr("traceDetailsEmpty")}</p>
        ) : (
          <ol className="telemetry-span-list">
            {traceSpanRows.map(({ span, depth, durationMs }) => (
              <li className="telemetry-span-row" key={span.spanId} style={{ paddingInlineStart: `${depth * 16}px` }}>
                <div className="telemetry-span-main">
                  <strong>{span.name || (language === "fa" ? "بازهٔ بدون نام" : "Unnamed span")}</strong>
                  <span className={`telemetry-span-status ${span.status?.code === 2 ? "error" : span.status?.code === 1 ? "ok" : "unset"}`}>
                    {span.status?.code === 2 ? tr("spanStatusError") : span.status?.code === 1 ? tr("spanStatusOk") : tr("spanStatusUnset")}
                  </span>
                </div>
                <div className="telemetry-span-meta">
                  <code>{span.spanId}</code>
                  <span>{Number.isFinite(durationMs) ? `${new Intl.NumberFormat(formatLocale(language), { maximumFractionDigits: 2 }).format(durationMs)} ${tr("spanDurationUnit")}` : "—"}</span>
                  <time>{span.startTimeUnixNano ? formatClock(nanoTimestamp(span.startTimeUnixNano), language) : t(language, "timeUnavailable")}</time>
                </div>
              </li>
            ))}
          </ol>
        )}
    </section>
  );

  const queriedAt =
    view === "Metrics" ? metrics?.queried_at :
    view === "Logs" ? logs?.queried_at :
    traces?.queried_at;
  const backend =
    view === "Metrics"
      ? metrics?.source === "workers-analytics-engine" || !useLegacyDevelopmentReads
        ? "WORKERS ANALYTICS ENGINE"
        : "PROMETHEUS"
      : !useLegacyDevelopmentReads
        ? "UNAVAILABLE"
        : view === "Logs" ? "LOKI" : "TEMPO";
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
            <p>{tr("liveQuery")} · {backend} · {selectedService}{!useLegacyDevelopmentReads && membership ? ` · ${membership.role}` : ""}</p>
          </div>
        </div>
        {!useLegacyDevelopmentReads && user && (
          <label className="incident-workspace-select telemetry-workspace-select">
            <span>{tr("incidentSelectWorkspace")}</span>
            <select
              value={workspaceId}
              onChange={(event) => {
                setMembership(null);
                setMembershipWorkspaceId("");
                setSelectedTraceId("");
                setTraceDetail(null);
                setTraceDetailError("");
                setWorkspaceId(event.target.value);
              }}
              disabled={workspaceLoading || workspaces.length === 0}
              aria-label={tr("incidentSelectWorkspace")}
            >
              {workspaces.length === 0 && <option value="">{tr("incidentNoWorkspaces")}</option>}
              {workspaces.map((workspace) => (
                <option key={workspace.id} value={workspace.id}>{workspace.name}</option>
              ))}
            </select>
          </label>
        )}
        <label className="telemetry-service-select">
          <span>{tr("service")}</span>
          <select
            value={selectedService}
            onChange={(event) => {
              setSelectedTraceId("");
              setTraceDetail(null);
              setTraceDetailError("");
              setSelectedService(event.target.value);
            }}
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
        {useLegacyDevelopmentReads && serviceHasMore ? (
          <button className="telemetry-load-more" type="button" onClick={() => void loadMoreServices()} disabled={serviceLoadingMore || serviceLoading}>
            {serviceLoadingMore ? tr("loadingMoreServices") : tr("loadMoreServices")}
          </button>
        ) : null}
        <div className="telemetry-controls">
          <div className="range-switcher" role="group" aria-label={tr("windowLabel")}>
            {windowOptions.map((item) => (
              <button key={item.value} className={windowRange === item.value ? "selected" : ""} aria-pressed={windowRange === item.value} onClick={() => {
                setSelectedTraceId("");
                setTraceDetail(null);
                setTraceDetailError("");
                setWindowRange(item.value);
              }}>
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

      {!useLegacyDevelopmentReads && !user && (
        <div className="incident-inline-notice" role="status">
          <AlertCircle size={15} />
          {isOidcConfigured() ? tr("telemetrySignInRequired") : tr("telemetryAuthNotConfigured")}
        </div>
      )}
      {!useLegacyDevelopmentReads && user && !workspaceLoading && workspaces.length === 0 && !workspaceError && (
        <div className="incident-inline-notice" role="status">{tr("incidentNoWorkspaces")}</div>
      )}
      {workspaceError && <div className="telemetry-error" role="alert"><AlertCircle size={15} /><span>{workspaceError}</span></div>}
      {view === "Metrics" && (
        <label className="telemetry-service-select telemetry-metric-select">
          <span>{tr("metricLabel")}</span>
          <select
            aria-label={tr("metricLabel")}
            value={selectedMetric}
            onChange={(event) => setSelectedMetric(event.target.value as TelemetryMetricID)}
            disabled={metricCatalogLoading || metricCatalog.length === 0 || (!useLegacyDevelopmentReads && (!user || !membership || membershipWorkspaceId !== workspaceId))}
          >
            {metricCatalog.map((metric) => (
              <option key={metric.id} value={metric.id}>
                {metric.id === "successful_request_rate" ? tr("successfulRequestRate") : metric.id === "failed_request_rate" ? tr("failedRequestRate") : tr("requestRate")}
              </option>
            ))}
          </select>
          <small>{metricCatalogLoading ? tr("metricCatalogLoading") : metricCatalogError || selectedMetricDescription}</small>
        </label>
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
            <TelemetryEmpty title={tr("noMetricSamplesTitle")} message={tr("noMetricSamplesMessage").replace("{metric}", selectedMetricLabel).replace("this service", selectedService).replace("این سرویس", selectedService)} />
          ) : (
            <>
              <div className="telemetry-stat-row">
                <div><span className="telemetry-kicker">{selectedMetricLabel}</span><strong>{formatRate(latestMetricRate, language)}<small>{tr("requestRateUnit")}</small></strong><p>{tr("latestPrometheus")}</p></div>
                <div className="telemetry-stat-secondary"><span className="telemetry-kicker">{tr("samples")}</span><strong>{metricSamples.length}</strong><p>{chartSeries.length} {tr("series")} · {tr("acrossWindow")} {windowRange}</p></div>
                <div className="telemetry-stat-secondary"><span className="telemetry-kicker">{tr("queryStep")}</span><strong>{metrics?.step ?? "—"}</strong><p>{tr("prometheusResolution")}</p></div>
              </div>
              <div className="telemetry-chart">
                {chartSeries.length > 0 ? (
                  <>
                    <svg viewBox="0 0 600 120" role="img" aria-label={`${selectedMetricLabel} · ${windowRange}`}>
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
                    <details className="telemetry-data-table">
                      <summary>{tr("showChartData")}</summary>
                      <div className="telemetry-data-table-scroll" role="region" aria-label={tr("chartDataCaption").replace("{metric}", selectedMetricLabel).replace("{service}", selectedService).replace("{window}", windowRange)} tabIndex={0}>
                        <table>
                          <caption>{tr("chartDataCaption").replace("{metric}", selectedMetricLabel).replace("{service}", selectedService).replace("{window}", windowRange)}</caption>
                          <thead>
                            <tr>
                              <th scope="col">{tr("chartTime")}</th>
                              <th scope="col">{tr("chartSeries")}</th>
                              <th scope="col">{tr("chartValue")}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {accessibleMetricRows.map((row, index) => (
                              <tr key={`${row.key}-${row.timestamp}-${index}`}>
                                <td>{formatClock(row.timestamp * 1000, language)}</td>
                                <td>{row.series}</td>
                                <td>{formatRate(row.value, language)}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </details>
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
                  <div className="telemetry-log-message">
                    <code>{entry.line}</code>
                    {entry.traceId && !useLegacyDevelopmentReads && user && membership && membershipWorkspaceId === workspaceId && (
                      <button
                        type="button"
                        className="telemetry-log-trace-action"
                        onClick={() => {
                          setSelectedTraceId(selectedTraceId === entry.traceId ? "" : entry.traceId!);
                          setTraceDetail(null);
                          setTraceDetailError("");
                        }}
                        aria-expanded={selectedTraceId === entry.traceId}
                        aria-controls="telemetry-log-trace-detail"
                        aria-label={(selectedTraceId === entry.traceId ? tr("closeTraceDetails") : tr("openTraceFromLog")) + ": " + entry.traceId}
                      >
                        {selectedTraceId === entry.traceId ? <X size={12} /> : <Eye size={12} />}
                        <span>{selectedTraceId === entry.traceId ? tr("closeTraceDetails") : tr("openTraceFromLog")}</span>
                      </button>
                    )}
                  </div>
                  <small>{entry.labels.service_name ?? (language === "fa" ? "سرویس نامشخص" : "unknown-service")}</small>
                </article>
              ))}
            </div>
          )}
          {renderTraceDetails("telemetry-log-trace-detail")}
        </div>
      )}

      {view === "Traces" && (
        <div className="panel telemetry-content-panel">
          {loading && !traces ? <TelemetryLoading language={language} /> : traceRows.length === 0 ? (
            <TelemetryEmpty title={tr("noTracesTitle")} message={language === "fa" ? tr("noTracesMessage").replace("این سرویس", selectedService) : tr("noTracesMessage").replace("this service", selectedService)} />
          ) : (
            <>
              <div className="telemetry-trace-list">
                {traceRows.map((trace) => {
                  const selected = selectedTraceId === trace.traceID;
                  return (
                    <article className={`telemetry-trace-row${selected ? " selected" : ""}`} key={trace.traceID}>
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
                      {!useLegacyDevelopmentReads && user && membership && membershipWorkspaceId === workspaceId && (
                        <div className="telemetry-trace-actions">
                          <button
                            type="button"
                            onClick={() => {
                              setSelectedTraceId(selected ? "" : trace.traceID);
                              setTraceDetail(null);
                              setTraceDetailError("");
                            }}
                            aria-expanded={selected}
                            aria-controls="telemetry-trace-detail"
                            aria-label={`${selected ? tr("closeTraceDetails") : tr("viewTraceDetails")}: ${trace.rootTraceName || trace.traceID}`}
                          >
                            {selected ? <X size={13} /> : <Eye size={13} />}
                            <span>{selected ? tr("closeTraceDetails") : tr("viewTraceDetails")}</span>
                          </button>
                        </div>
                      )}
                    </article>
                  );
                })}
              </div>
              {renderTraceDetails("telemetry-trace-detail")}
            </>
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

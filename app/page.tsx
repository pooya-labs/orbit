"use client";

import { useEffect, useMemo, useState } from "react";
import { getMetadata, listIncidents, listServices } from "../lib/orbit/client";
import type { ApiMetadata, IncidentSummary, ServiceSummary } from "../lib/orbit/contracts";
import { chartArea, chartPoints, demoIncidents, demoServices } from "../lib/orbit/demo";
import { formatLocale, navLabel, t, type Language, type TranslationKey } from "../lib/orbit/i18n";
import type { ReactNode } from "react";
import TelemetryWorkspace, { type TelemetryView } from "./components/TelemetryWorkspace";
import type { LucideIcon } from "lucide-react";
import {
  Activity,
  AlertTriangle,
  ArrowDownRight,
  ArrowUpRight,
  Bell,
  Boxes,
  Check,
  ChevronDown,
  CircleHelp,
  Clock3,
  Command,
  Database,
  ExternalLink,
  FileText,
  GitBranch,
  Globe2,
  Gauge,
  Menu,
  MoreHorizontal,
  Network,
  Pause,
  Play,
  Radio,
  RefreshCw,
  Search,
  Server,
  Settings2,
  ShieldCheck,
  Signal,
  TerminalSquare,
  Workflow,
  X,
  Zap,
} from "lucide-react";

type ServiceState = "Operational" | "Degraded" | "Incident" | "Unknown";

type Service = {
  name: string;
  description: string;
  region: string;
  latency: string;
  errorRate: string;
  state: ServiceState;
  initials: string;
  tone: string;
};


type SourceMode = "demo" | "api";
type IncidentRow = {
  id: string;
  title: string;
  serviceName: string;
  region: string;
  severityLabel: string;
  severityTone: "critical" | "warning";
  statusLabel: string;
  timeAgo: string;
  source: string;
};

const serviceTones = ["blue", "violet", "amber", "teal", "rose"];

function mapApiService(item: ServiceSummary, index: number): Service {
  const state: ServiceState =
    item.status === "operational" ? "Operational" :
    item.status === "degraded" ? "Degraded" :
    item.status === "incident" ? "Incident" : "Unknown";
  const initials = item.name.split(/[-_ ]+/).slice(0, 2)
    .map((part) => part.slice(0, 1).toUpperCase()).join("");
  return {
    name: item.name,
    description: item.description || item.environment,
    region: item.region || "—",
    latency: item.latency_p95_ms === null ? "—" : `${Math.round(item.latency_p95_ms)} ms`,
    errorRate: item.error_rate === null ? "—" : `${(item.error_rate * 100).toFixed(2)}%`,
    state,
    initials: initials || "SV",
    tone: serviceTones[index % serviceTones.length],
  };
}

function formatAge(timestamp: string, language: Language): string {
  const elapsed = Math.max(0, Date.now() - new Date(timestamp).getTime());
  if (!Number.isFinite(elapsed)) return t(language, "timeUnavailable");
  const minutes = Math.floor(elapsed / 60000);
  const relative = new Intl.RelativeTimeFormat(formatLocale(language), { numeric: "auto" });
  if (minutes < 1) return t(language, "justNow");
  if (minutes < 60) return relative.format(-minutes, "minute");
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return relative.format(-hours, "hour");
  return relative.format(-Math.floor(hours / 24), "day");
}

function toIncidentRow(
  item: IncidentSummary,
  services: ServiceSummary[],
  language: Language,
): IncidentRow {
  const service = services.find((candidate) => candidate.id === item.service_id);
  return {
    id: item.id,
    title: item.title,
    serviceName: item.service_name,
    region: service?.region ?? t(language, "regionUnavailable"),
    severityLabel: item.severity.toUpperCase(),
    severityTone: item.severity === "critical" || item.severity === "high" ? "critical" : "warning",
    statusLabel: item.status.replace(/_/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase()),
    timeAgo: formatAge(item.started_at, language),
    source: item.source,
  };
}

function toDemoIncidentRow(item: (typeof demoIncidents)[number], language: Language): IncidentRow {
  return {
    id: item.id,
    title: item.title,
    serviceName: item.serviceName,
    region: item.region,
    severityLabel: item.severity.toUpperCase(),
    severityTone: item.severity === "critical" ? "critical" : "warning",
    statusLabel: item.status.replace(/_/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase()),
    timeAgo: formatAge(item.startedAt, language),
    source: item.source,
  };
}

type NavItem = { label: string; icon: LucideIcon; count?: string; alert?: boolean };
type NavGroup = { label: string; items: NavItem[] };

const navGroups: NavGroup[] = [
  {
    label: "OBSERVE",
    items: [
      { label: "Overview", icon: Gauge },
      { label: "Service map", icon: Network },
      { label: "Services", icon: Boxes },
      { label: "Infrastructure", icon: Server },
    ],
  },
  {
    label: "INVESTIGATE",
    items: [
      { label: "Metrics", icon: Activity },
      { label: "Traces", icon: Workflow },
      { label: "Logs", icon: TerminalSquare },
      { label: "Incidents", icon: AlertTriangle },
    ],
  },
  {
    label: "DELIVER",
    items: [
      { label: "Deployments", icon: GitBranch },
      { label: "Alert rules", icon: Bell },
    ],
  },
];

function StatusDot({ state }: { state: ServiceState }) {
  return <span className={`status-dot ${state.toLowerCase()}`} role="img" aria-label={state} />;
}

function MetricCard({
  label,
  value,
  change,
  direction,
  detail,
  icon: Icon,
  accent,
}: {
  label: string;
  value: ReactNode;
  change: string;
  direction: "up" | "down";
  detail: string;
  icon: LucideIcon;
  accent: string;
}) {
  return (
    <article className="metric-card">
      <div className="metric-top">
        <span className="metric-label">{label}</span>
        <span className={`metric-icon ${accent}`}><Icon size={16} strokeWidth={1.8} /></span>
      </div>
      <div className="metric-value-row">
        <strong className="metric-value">{value}</strong>
        <span className={`metric-change ${direction === "down" ? "positive" : "negative"}`}>
          {direction === "down" ? <ArrowDownRight size={14} /> : <ArrowUpRight size={14} />}
          {change}
        </span>
      </div>
      <p className="metric-detail">{detail}</p>
      <div className={`metric-sparkline ${accent}`} aria-hidden="true">
        <svg viewBox="0 0 120 28" preserveAspectRatio="none">
          <path d="M0 21 C12 18 14 22 24 15 S38 17 48 11 S63 16 72 9 S88 13 96 6 S109 9 120 3" />
        </svg>
      </div>
    </article>
  );
}

export default function Home() {
  const [activeNav, setActiveNav] = useState("Overview");
  const [language, setLanguage] = useState<Language>("en");
  const tr = (key: TranslationKey) => t(language, key);
  const [range, setRange] = useState("1 hour");
  const [search, setSearch] = useState("");
  const [refreshing, setRefreshing] = useState(true);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [showAllServices, setShowAllServices] = useState(false);
  const [sourceMode, setSourceMode] = useState<SourceMode>("demo");
  const [sourcePanelOpen, setSourcePanelOpen] = useState(false);
  const [apiUrlInput, setApiUrlInput] = useState(
    process.env.NEXT_PUBLIC_ORBIT_API_URL ?? "http://localhost:8080",
  );
  const [apiStatus, setApiStatus] = useState<"idle" | "connecting" | "connected" | "error">("idle");
  const [apiError, setApiError] = useState("");
  const [apiMetadata, setApiMetadata] = useState<ApiMetadata | null>(null);
  const [apiServices, setApiServices] = useState<ServiceSummary[]>([]);
  const [apiIncidents, setApiIncidents] = useState<IncidentSummary[]>([]);

  useEffect(() => {
    try {
      const savedLanguage = window.localStorage.getItem("orbit.language");
      if (savedLanguage === "en" || savedLanguage === "fa") setLanguage(savedLanguage);
      else if (window.navigator.language.toLowerCase().startsWith("fa")) setLanguage("fa");
    } catch {
      // Language selection remains available when storage is disabled.
    }
  }, []);

  useEffect(() => {
    try {
      const savedUrl = window.localStorage.getItem("orbit.apiBaseUrl");
      if (savedUrl) setApiUrlInput(savedUrl);
    } catch {
      // Storage may be unavailable in a restricted browser context.
    }
  }, []);

  const changeLanguage = (next: Language) => {
    setLanguage(next);
    try { window.localStorage.setItem("orbit.language", next); } catch { /* State still changes for this session. */ }
  };

  const connectSource = async () => {
    let endpoint: string;
    try {
      const parsed = new URL(apiUrlInput.trim());
      if (!["http:", "https:"].includes(parsed.protocol) || parsed.username || parsed.password ||
          parsed.search || parsed.hash || (parsed.pathname !== "/" && parsed.pathname !== "")) {
        throw new Error("Enter an API base URL such as http://localhost:8080.");
      }
      endpoint = parsed.origin;
    } catch (error) {
      setApiStatus("error");
      setApiError(error instanceof Error ? error.message : "Enter a valid API URL.");
      return;
    }

    setApiStatus("connecting");
    setApiError("");
    try {
      const [metadata, serviceItems, incidentItems] = await Promise.all([
        getMetadata(undefined, endpoint),
        listServices(undefined, endpoint),
        listIncidents(undefined, undefined, endpoint),
      ]);
      setApiMetadata(metadata);
      setApiServices(serviceItems);
      setApiIncidents(incidentItems);
      setApiUrlInput(endpoint);
      setSourceMode("api");
      setApiStatus("connected");
      try {
        window.localStorage.setItem("orbit.apiBaseUrl", endpoint);
      } catch {
        // A working connection should not depend on local storage support.
      }
      setSourcePanelOpen(false);
    } catch (error) {
      setApiStatus("error");
      setApiError(error instanceof Error ? error.message : "Unable to connect to the ORBIT API.");
    }
  };

  const sourceServices = sourceMode === "api" ? apiServices.map(mapApiService) : demoServices;
  const filteredServices = useMemo(() => {
    const query = search.trim().toLowerCase();
    const matching = sourceServices.filter((service) =>
      `${service.name} ${service.description} ${service.region}`.toLowerCase().includes(query),
    );
    return showAllServices ? matching : matching.slice(0, 4);
  }, [search, showAllServices, sourceServices]);

  const incidentRows = useMemo(() => {
    if (sourceMode === "api") {
      return apiIncidents
        .filter((incident) => incident.status !== "resolved")
        .map((incident) => toIncidentRow(incident, apiServices, language));
    }
    return demoIncidents.map((incident) => toDemoIncidentRow(incident, language));
  }, [sourceMode, apiIncidents, apiServices, language]);

  const disconnectSource = () => {
    setSourceMode("demo");
    setApiStatus("idle");
    setApiError("");
  };

  const chooseNav = (label: string) => {
    setActiveNav(label);
    setMobileNavOpen(false);
  };

  const telemetryView: TelemetryView | null =
    activeNav === "Metrics" || activeNav === "Logs" || activeNav === "Traces"
      ? activeNav
      : null;

  return (
    <main className="app-shell" dir={language === "fa" ? "rtl" : "ltr"} lang={language}>
      {mobileNavOpen && <button className="mobile-scrim" aria-label={tr("closeNavigation")} onClick={() => setMobileNavOpen(false)} />}
      <aside className={`sidebar ${mobileNavOpen ? "sidebar-open" : ""}`}>
        <div className="brand-lockup">
          <div className="brand-mark"><span /><span /><span /></div>
          <div className="brand-wordmark">ORBIT<span>.</span><small>OBSERVABILITY</small></div>
          <button className="icon-button sidebar-close" aria-label={tr("closeNavigation")} onClick={() => setMobileNavOpen(false)}><X size={18} /></button>
        </div>

        <button className="workspace-switcher">
          <div className="workspace-avatar">N</div>
          <div className="workspace-copy"><strong>ORBIT</strong><span>{language === "fa" ? "فضای کاری شخصی" : "Personal workspace"}</span></div>
          <ChevronDown size={15} className="muted-icon" />
        </button>

        <div className="sidebar-scroll">
          {navGroups.map((group) => (
            <div className="nav-group" key={group.label}>
              <div className="nav-group-label">{group.label === "OBSERVE" ? tr("observe") : group.label === "INVESTIGATE" ? tr("investigate") : tr("deliver")}</div>
              {group.items.map(({ label, icon: Icon, count, alert }) => (
                <button key={label} className={`nav-item ${activeNav === label ? "active" : ""}`} onClick={() => chooseNav(label)}>
                  <Icon size={17} strokeWidth={1.75} />
                  <span>{navLabel(language, label)}</span>
                  {count && <span className={`nav-count ${alert ? "nav-alert" : ""}`}>{count}</span>}
                </button>
              ))}
            </div>
          ))}
        </div>

        <div className="sidebar-bottom">
          <div className="usage-card">
            <div className="usage-heading"><span>{tr("telemetryVolume")}</span><span>68%</span></div>
            <div className="usage-track"><span /></div>
            <p>6.8 GB <span>of 10 GB monthly</span></p>
          </div>
          <button className="nav-item"><Settings2 size={17} /><span>{tr("settings")}</span></button>
          <button className="nav-item"><CircleHelp size={17} /><span>{tr("helpSupport")}</span><ExternalLink size={12} className="nav-external" /></button>
          <div className="profile-row">
            <div className="profile-avatar">PG</div>
            <div className="profile-copy"><strong>Pooya</strong><a href="https://github.com/pooya16pg-spec" target="_blank" rel="noreferrer">@pooya16pg-spec</a></div>
            <MoreHorizontal size={18} className="muted-icon" />
          </div>
        </div>
      </aside>

      <section className="main-panel">
        <header className="topbar">
          <div className="topbar-left">
            <button className="icon-button mobile-menu" aria-label={tr("openNavigation")} onClick={() => setMobileNavOpen(true)}><Menu size={19} /></button>
            <div className="breadcrumbs"><span>{tr("workspace")}</span><span className="crumb-separator">/</span><strong>{navLabel(language, activeNav)}</strong></div>
            <span className="environment-pill"><span /> {tr("production")}</span>
          </div>
          <div className="topbar-actions">
            <label className="global-search">
              <Search size={15} />
              <input aria-label={tr("searchServicesLabel")} placeholder={tr("searchServices")} value={search} onChange={(event) => setSearch(event.target.value)} />
              <kbd>/</kbd>
            </label>
            <button
              className={`icon-button refresh-button ${sourceMode === "api" ? "is-on" : refreshing ? "is-on" : ""}`}
              aria-label={sourceMode === "api" ? tr("refreshApi") : refreshing ? tr("pauseRefresh") : tr("resumeRefresh")}
              title={sourceMode === "api" ? tr("refreshApi") : refreshing ? tr("pauseRefresh") : tr("resumeRefresh")}
              onClick={() => sourceMode === "api" ? void connectSource() : setRefreshing(!refreshing)}
            >
              {sourceMode === "api" ? <RefreshCw size={16} /> : refreshing ? <Pause size={16} /> : <Play size={16} />}
            </button>
            <button className="icon-button notification-button" aria-label={tr("notifications")}><Bell size={17} /><i /></button>
            <div className="topbar-divider" />
            <div className="language-switch" role="group" aria-label={tr("language")}>
              <button type="button" className={language === "en" ? "selected" : ""} aria-pressed={language === "en"} onClick={() => changeLanguage("en")}>EN</button>
              <button type="button" className={language === "fa" ? "selected" : ""} aria-pressed={language === "fa"} onClick={() => changeLanguage("fa")}>فارسی</button>
            </div>
            <button className="help-button"><CircleHelp size={16} /><span>{tr("docs")}</span></button>
          </div>
        </header>

        <div className="content-wrap">
          <div className="page-heading">
            <div>
              <div className="eyebrow"><span className="eyebrow-line" /> {telemetryView ? `${language === "fa" ? "زنده" : "LIVE"} ${navLabel(language, telemetryView).toUpperCase()}` : tr("systemOverview")} <span className="demo-tag">{sourceMode === "api" ? tr("apiConnected") : tr("demoEnvironment")}</span></div>
              <h1>{activeNav === "Overview" ? tr("goodMorning") : navLabel(language, activeNav)}</h1>
              <p className="page-subtitle">{telemetryView ? `Query ${telemetryView.toLowerCase()} from the connected observability pipeline.` : "وضعیت سرویس‌ها و زیرساخت را در یک نگاه ببین."}</p>
            </div>
            <div className="heading-actions">
              <div className="last-updated"><span className={refreshing ? "pulse-dot" : "idle-dot"} /> {sourceMode === "api" ? `${tr("connected")} · ${apiMetadata?.environment ?? "API"}` : refreshing ? tr("autoRefreshPaused") : tr("manualRefresh")}</div>
              <button className="button-secondary" onClick={() => setRange("24 hours")}><Clock3 size={15} /> {tr("last24Hours")} <ChevronDown size={14} /></button>
              <button className="button-primary" onClick={() => chooseNav("Service map")}><Network size={15} /> {tr("exploreTopology")}</button>
            </div>
          </div>

          <div className="notice-strip">
            <div className="notice-icon"><Radio size={16} /></div>
            {sourceMode === "demo" ? (
              <div><strong>در حال مشاهدهٔ یک محیط نمونه هستی.</strong><span> شاخص‌ها، سرویس‌ها و رخدادها داده‌های نمایشی هستند و به هیچ سامانهٔ عملیاتی متصل نیستند.</span></div>
            ) : (
              <div><strong>{tr("connectedTo")} {apiMetadata?.name ?? "ORBIT API"}.</strong><span> {language === "fa" ? "سرویس‌ها و رخدادها از" : "Services and incidents are read from"} {apiMetadata?.environment ?? (language === "fa" ? "API انتخاب‌شده" : "the selected API")} {language === "fa" ? "خوانده می‌شوند؛ منبع هر رکورد جداگانه مشخص است." : "; each record keeps its own data-source label."}</span></div>
            )}
            <button onClick={() => setSourcePanelOpen(!sourcePanelOpen)}>{sourceMode === "api" ? tr("manageSource") : tr("connectSource")} <ArrowUpRight size={14} /></button>
          </div>

          {sourcePanelOpen && (
            <section className="panel source-setup" aria-labelledby="source-setup-title">
              <div className="source-setup-copy">
                <h2 id="source-setup-title">{tr("connectApiTitle")}</h2>
                <p>از یک نشانی فقط‌خواندنی API استفاده کن. برای اجرای محلی، Docker Compose را اجرا و http://localhost:8080 را وارد کن.</p>
              </div>
              <form className="source-setup-form" onSubmit={(event) => { event.preventDefault(); void connectSource(); }}>
                <label htmlFor="orbit-api-url">{tr("apiBaseUrl")}</label>
                <input id="orbit-api-url" type="url" autoComplete="url" spellCheck={false} value={apiUrlInput} onChange={(event) => setApiUrlInput(event.target.value)} placeholder="http://localhost:8080" />
                <div className="source-setup-actions">
                  <button className="button-primary" type="submit" disabled={apiStatus === "connecting"}>{apiStatus === "connecting" ? tr("connecting") : tr("testConnect")}</button>
                  {sourceMode === "api" && <button className="button-secondary" type="button" onClick={disconnectSource}>{tr("switchToDemo")}</button>}
                </div>
              </form>
              {apiError && <p className="source-error" role="alert">{apiError}</p>}
              {apiStatus === "connected" && <p className="source-success" role="status">{tr("connectionVerified")} {apiServices.length} services and {apiIncidents.length} incidents loaded.</p>}
            </section>
          )}

{telemetryView ? (
            sourceMode === "api" ? (
              <TelemetryWorkspace view={telemetryView} apiBaseUrl={apiUrlInput} />
            ) : (
              <section className="panel telemetry-connect-prompt">
                <div className="source-placeholder-icon"><Activity size={18} /></div>
                <div>
                  <h2>{tr("connectTelemetryTitle")}</h2>
                  <p>پس از اتصال به ORBIT API متصل به زیرساخت پایش محلی، امکان دریافت زندهٔ {navLabel(language, telemetryView).toLowerCase()} فراهم می‌شود. داده‌های نمونه هرگز به‌عنوان دادهٔ زنده نمایش داده نمی‌شوند.</p>
                </div>
                <button className="button-primary" onClick={() => setSourcePanelOpen(true)}>{tr("connectApiSource")} <ArrowUpRight size={14} /></button>
              </section>
            )
          ) : (
            <>
{sourceMode === "demo" ? (
          <section className="metrics-grid" aria-label="Key metrics">
            <MetricCard label="REQUESTS / MIN" value="24.8k" change="12.6%" direction="down" detail="vs. previous period" icon={Activity} accent="cyan" />
            <MetricCard label="P95 LATENCY" value={<>142<span>ms</span></>} change="8.2%" direction="down" detail="vs. previous period" icon={Zap} accent="violet" />
            <MetricCard label="ERROR RATE" value={<>0.24<span>%</span></>} change="0.04%" direction="up" detail="vs. previous period" icon={AlertTriangle} accent="amber" />
            <MetricCard label="ACTIVE SERVICES" value={<>12<span>/ 14</span></>} change="2 degraded" direction="up" detail="across 3 regions" icon={Boxes} accent="green" />
          </section>
          ) : null}

{sourceMode === "demo" ? (
          <section className="panel traffic-panel">
            <div className="panel-header">
              <div><div className="panel-title-line"><h2>Traffic & performance</h2><span className="live-indicator"><i /> SAMPLE</span></div><p>Incoming request volume across all services</p></div>
              <div className="range-switcher" role="group" aria-label="Chart time range">
                {["1 hour", "6 hours", "24 hours"].map((item) => <button key={item} className={range === item ? "selected" : ""} onClick={() => setRange(item)}>{item}</button>)}
              </div>
            </div>
            <div className="chart-summary">
              <div><strong>18,429</strong><span>req / min</span></div>
              <div className="chart-summary-change"><ArrowUpRight size={14} /> 6.4% <span>compared to prior period</span></div>
            </div>
            <div className="chart-wrap">
              <div className="chart-y-labels"><span>30k</span><span>20k</span><span>10k</span><span>0</span></div>
              <div className="chart-plot">
                <div className="chart-grid-lines"><i /><i /><i /><i /></div>
                <svg className="traffic-chart" viewBox="0 0 598 140" preserveAspectRatio="none" role="img" aria-label="Illustrative request volume trend">
                  <defs><linearGradient id="trafficFill" x1="0" x2="0" y1="0" y2="1"><stop offset="0%" stopColor="#65d9c1" stopOpacity=".22" /><stop offset="100%" stopColor="#65d9c1" stopOpacity="0" /></linearGradient></defs>
                  <polygon points={chartArea} fill="url(#trafficFill)" />
                  <polyline points={chartPoints} fill="none" stroke="#65d9c1" strokeWidth="2.2" strokeLinejoin="round" strokeLinecap="round" vectorEffect="non-scaling-stroke" />
                  <circle cx="494" cy="20" r="4" fill="#65d9c1" stroke="#101820" strokeWidth="2" vectorEffect="non-scaling-stroke" />
                </svg>
                <div className="chart-x-labels"><span>09:00</span><span>09:10</span><span>09:20</span><span>09:30</span><span>09:40</span><span>09:50</span><span>10:00</span></div>
              </div>
            </div>
            <div className="chart-legend"><span><i className="legend-line" /> Requests / min</span><span><i className="legend-dash" /> Previous period</span><span className="chart-footnote">Illustrative telemetry · {range}</span></div>
          </section>
          ) : null}

          <div className="lower-grid">
            <section className="panel services-panel">
              <div className="panel-header compact">
                <div><div className="panel-title-line"><h2>Service health</h2><span className="subtle-count">{sourceServices.length} monitored</span></div><p>Health across your distributed services</p></div>
                <button className="text-action" onClick={() => setShowAllServices(!showAllServices)}>{showAllServices ? "Show less" : "View all"} <ArrowUpRight size={14} /></button>
              </div>
              <div className="service-table">
                <div className="service-table-head"><span>SERVICE</span><span>LATENCY</span><span>ERROR RATE</span><span>STATUS</span></div>
                {filteredServices.map((service) => (
                  <div className="service-row" key={service.name}>
                    <div className="service-name-cell"><div className={`service-avatar ${service.tone}`}>{service.initials}</div><div><strong>{service.name}</strong><span>{service.description}</span></div></div>
                    <span className="service-metric">{service.latency}</span>
                    <span className={`service-metric ${service.state === "Degraded" || service.state === "Incident" ? "metric-warning" : ""}`}>{service.errorRate}</span>
                    <span className="service-status"><StatusDot state={service.state} />{service.state}</span>
                  </div>
                ))}
                {filteredServices.length === 0 && <div className="empty-state">No services match “{search}”.</div>}
              </div>
              <div className="panel-footer"><span>Showing {filteredServices.length} of {sourceServices.length} services</span><span className="demo-footnote">{sourceMode === "api" ? "API DATA" : "DEMO DATA"}</span></div>
            </section>

            <section className="panel incidents-panel">
              <div className="panel-header compact">
                <div><div className="panel-title-line"><h2>Active incidents</h2><span className="incident-count">{incidentRows.length}</span></div><p>Issues that may need attention</p></div>
                <button className="icon-button subtle-more" aria-label="Incident options"><MoreHorizontal size={18} /></button>
              </div>
              <div className="incident-list">
                {incidentRows.length > 0 ? incidentRows.map((incident) => (
                  <article className="incident-item" key={incident.id}>
                    <div className={`incident-severity ${incident.severityTone}`}><AlertTriangle size={15} /></div>
                    <div className="incident-copy">
                      <div className="incident-title-row"><strong>{incident.title}</strong><span className={`severity-label ${incident.severityTone === "critical" ? "critical-text" : "warning-text"}`}>{incident.severityLabel}</span></div>
                      <p>{incident.serviceName} · {incident.region}</p>
                      <div className="incident-meta"><Clock3 size={12} /> {incident.timeAgo} <span>·</span> <span className="incident-open">{incident.statusLabel}</span></div>
                    </div>
                  </article>
                )) : <div className="empty-state">No active incidents reported by this source.</div>}
              </div>
              <button className="incident-link" onClick={() => chooseNav("Incidents")}>View incident timeline <ArrowUpRight size={14} /></button>
            </section>
          </div>

{sourceMode === "demo" ? (
          <section className="bottom-grid">
            <div className="bottom-stat"><div className="bottom-icon green"><ShieldCheck size={17} /></div><div><span>AVAILABILITY SLO</span><strong>99.94%</strong></div><span className="bottom-note good"><Check size={13} /> Within target</span></div>
            <div className="bottom-stat"><div className="bottom-icon violet"><GitBranch size={17} /></div><div><span>DEPLOYMENTS TODAY</span><strong>08</strong></div><span className="bottom-note">Across 4 services</span></div>
            <div className="bottom-stat"><div className="bottom-icon blue"><Database size={17} /></div><div><span>DATA INGESTION</span><strong>2.4 MB/s</strong></div><span className="bottom-note"><Signal size={13} /> Sample stream</span></div>
          </section>
          ) : null}

            </>
          )}

          <footer className="page-footer"><span>ORBIT <b>·</b> SYSTEMS OBSERVATORY</span><span><Globe2 size={12} /> {sourceMode === "api" ? "Service and incident inventory from connected API" : "All systems are illustrative"} <i /> <span className="footer-version">v0.8.0</span></span></footer>
        </div>
      </section>
    </main>
  );
}

"use client";

import { useEffect, useMemo, useState } from "react";
import { getMetadata, listWorkspaceIncidents, listWorkspaceServices, listWorkspaces } from "../lib/orbit/client";
import type { ApiMetadata, IncidentSummary, ServiceSummary } from "../lib/orbit/contracts";
import { chartArea, chartPoints, demoIncidents, demoServices } from "../lib/orbit/demo";
import { formatLocale, navLabel, t, type Language, type TranslationKey } from "../lib/orbit/i18n";
import type { ReactNode } from "react";
import TelemetryWorkspace, { type TelemetryView } from "./components/TelemetryWorkspace";
import AuthControls from "./components/AuthControls";
import IncidentWorkspace from "./components/IncidentWorkspace";
import AlertWorkspace from "./components/AlertWorkspace";
import NotificationWorkspace from "./components/NotificationWorkspace";
import DeploymentWorkspace from "./components/DeploymentWorkspace";
import SLOWorkspace from "./components/SLOWorkspace";
import AuditWorkspace from "./components/AuditWorkspace";
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
  Sun,
  Moon,
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

function mapApiService(item: ServiceSummary, index: number, language: Language): Service {
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
    latency: item.latency_p95_ms === null ? "—" : `${new Intl.NumberFormat(formatLocale(language), { maximumFractionDigits: 0 }).format(Math.round(item.latency_p95_ms))} ms`,
    errorRate: item.error_rate === null ? "—" : `${new Intl.NumberFormat(formatLocale(language), { maximumFractionDigits: 2, minimumFractionDigits: 2 }).format(item.error_rate * 100)}%`,
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
    severityLabel: t(language, item.severity),
    severityTone: item.severity === "critical" || item.severity === "high" ? "critical" : "warning",
    statusLabel: t(language, item.status),
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
    severityLabel: t(language, item.severity),
    severityTone: item.severity === "critical" ? "critical" : "warning",
    statusLabel: t(language, item.status),
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
      { label: "Notifications", icon: Radio },
      { label: "SLOs", icon: Gauge },
      { label: "Audit log", icon: FileText },
    ],
  },
];

function serviceStateLabel(state: ServiceState, language: Language): string {
  const key: Record<ServiceState, TranslationKey> = { Operational: "operational", Degraded: "degraded", Incident: "incident", Unknown: "unknownStatus" };
  return t(language, key[state]);
}

function StatusDot({ state, language }: { state: ServiceState; language: Language }) {
  return <span className={`status-dot ${state.toLowerCase()}`} role="img" aria-label={serviceStateLabel(state, language)} />;
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
  const [theme, setTheme] = useState<"dark" | "light">("dark");
  const [themeReady, setThemeReady] = useState(false);
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
  const [apiWorkspaceId, setApiWorkspaceId] = useState("");
  const [apiWorkspaceName, setApiWorkspaceName] = useState("");
  const [apiConnectedBaseUrl, setApiConnectedBaseUrl] = useState("");
  const [apiServices, setApiServices] = useState<ServiceSummary[]>([]);
  const [apiIncidents, setApiIncidents] = useState<IncidentSummary[]>([]);

  useEffect(() => {
    try {
      const savedLanguage = window.localStorage.getItem("orbit.language");
      const savedTheme = window.localStorage.getItem("orbit.theme");
      if (savedLanguage === "en" || savedLanguage === "fa") setLanguage(savedLanguage);
      if (savedTheme === "light" || savedTheme === "dark") setTheme(savedTheme);
      else if (window.navigator.language.toLowerCase().startsWith("fa")) setLanguage("fa");
    } catch {
      // Language and theme controls remain available when storage is disabled.
    } finally {
      setThemeReady(true);
    }
  }, []);

  useEffect(() => {
    const root = document.documentElement;
    root.lang = language;
    root.dir = language === "fa" ? "rtl" : "ltr";
  }, [language]);

  useEffect(() => {
    if (!themeReady) return;
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
    try {
      window.localStorage.setItem("orbit.theme", theme);
    } catch {
      // Theme still applies for the current session when storage is unavailable.
    }
  }, [theme, themeReady]);

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
        throw new Error(tr("invalidApiUrl"));
      }
      endpoint = parsed.origin;
    } catch (error) {
      setApiStatus("error");
      setApiError(error instanceof Error ? error.message : tr("validApiUrl"));
      return;
    }

    setApiStatus("connecting");
    setApiError("");
    try {
      const [metadata, workspaceItems] = await Promise.all([
        getMetadata(undefined, endpoint),
        listWorkspaces(undefined, endpoint),
      ]);
      const firstWorkspace = workspaceItems[0];
      const [serviceItems, incidentItems]: [ServiceSummary[], IncidentSummary[]] = firstWorkspace
        ? await Promise.all([
            listWorkspaceServices(firstWorkspace.id, undefined, endpoint),
            listWorkspaceIncidents(firstWorkspace.id, undefined, undefined, endpoint),
          ])
        : [[], []];
      setApiMetadata(metadata);
      setApiWorkspaceId(firstWorkspace?.id ?? "");
      setApiWorkspaceName(firstWorkspace?.name ?? "");
      setApiConnectedBaseUrl(endpoint);
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
      setApiError(error instanceof Error ? error.message : tr("connectFailed"));
    }
  };

  const sourceServices = sourceMode === "api" ? apiServices.map((item, index) => mapApiService(item, index, language)) : demoServices;
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
    setApiWorkspaceId("");
    setApiWorkspaceName("");
    setApiConnectedBaseUrl("");
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
    <main className="app-shell" data-theme={theme} dir={language === "fa" ? "rtl" : "ltr"} lang={language}>
      {mobileNavOpen && <button className="mobile-scrim" aria-label={tr("closeNavigation")} onClick={() => setMobileNavOpen(false)} />}
      <aside className={`sidebar ${mobileNavOpen ? "sidebar-open" : ""}`}>
        <div className="brand-lockup">
          <div className="brand-mark"><span /><span /><span /></div>
          <div className="brand-wordmark">ORBIT<span>.</span><small>OBSERVABILITY</small></div>
          <button className="icon-button sidebar-close" aria-label={tr("closeNavigation")} onClick={() => setMobileNavOpen(false)}><X size={18} /></button>
        </div>

        <button className="workspace-switcher">
          <div className="workspace-avatar">O</div>
          <div className="workspace-copy"><strong>{apiStatus === "connected" && apiWorkspaceName ? apiWorkspaceName : "ORBIT"}</strong><span>{apiStatus === "connected" ? (language === "fa" ? "فضای کاری متصل" : "Connected workspace") : (language === "fa" ? "فضای کاری شخصی" : "Personal workspace")}</span></div>
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
            <p>6.8 GB <span>{language === "fa" ? "از ۱۰ گیگابایت در ماه" : "of 10 GB monthly"}</span></p>
          </div>
          <button className="nav-item"><Settings2 size={17} /><span>{tr("settings")}</span></button>
          <button className="nav-item"><CircleHelp size={17} /><span>{tr("helpSupport")}</span><ExternalLink size={12} className="nav-external" /></button>
          <div className="profile-row">
            <div className="profile-avatar">P</div>
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
            <AuthControls language={language} />
            <div className="topbar-divider" />
            <button
              type="button"
              className="icon-button theme-toggle"
              aria-label={theme === "dark" ? (language === "fa" ? "فعال‌کردن حالت روشن" : "Switch to light theme") : (language === "fa" ? "فعال‌کردن حالت تیره" : "Switch to dark theme")}
              title={theme === "dark" ? (language === "fa" ? "حالت روشن" : "Light theme") : (language === "fa" ? "حالت تیره" : "Dark theme")}
              aria-pressed={theme === "light"}
              onClick={() => setTheme((current) => current === "dark" ? "light" : "dark")}
            >
              {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
            </button>
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
              <p className="page-subtitle">{telemetryView ? tr("liveQuerySubtitle").replace("{view}", navLabel(language, telemetryView).toLowerCase()) : tr("overviewSubtitle")}</p>
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
              <div><strong>{tr("sampleEnvironmentTitle")}</strong><span>{tr("sampleEnvironmentBody")}</span></div>
            ) : (
              <div><strong>{tr("connectedTo")} {apiMetadata?.name ?? "ORBIT API"}.</strong><span>{tr("connectedInventory").replace("{environment}", apiMetadata?.environment ?? (language === "fa" ? "API انتخاب‌شده" : "the selected API"))}</span></div>
            )}
            <button onClick={() => setSourcePanelOpen(!sourcePanelOpen)}>{sourceMode === "api" ? tr("manageSource") : tr("connectSource")} <ArrowUpRight size={14} /></button>
          </div>

          {sourcePanelOpen && (
            <section className="panel source-setup" aria-labelledby="source-setup-title">
              <div className="source-setup-copy">
                <h2 id="source-setup-title">{tr("connectApiTitle")}</h2>
                <p>{tr("apiHelp")}</p>
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
              {apiStatus === "connected" && <p className="source-success" role="status">{tr("connectionVerified")} {apiServices.length} {tr("services")} · {apiIncidents.length} {tr("incidentsLoaded")}</p>}
            </section>
          )}

{activeNav === "Audit log" ? (
              <AuditWorkspace apiBaseUrl={apiUrlInput} language={language} sourceMode={sourceMode} />
            ) : activeNav === "SLOs" ? (
              <SLOWorkspace apiBaseUrl={apiUrlInput} language={language} sourceMode={sourceMode} />
            ) : activeNav === "Deployments" ? (
              <DeploymentWorkspace apiBaseUrl={apiUrlInput} language={language} sourceMode={sourceMode} />
            ) : activeNav === "Notifications" ? (
              <NotificationWorkspace apiBaseUrl={apiUrlInput} language={language} sourceMode={sourceMode} />
            ) : activeNav === "Alert rules" ? (
              <AlertWorkspace apiBaseUrl={apiUrlInput} language={language} sourceMode={sourceMode} />
            ) : activeNav === "Incidents" ? (
              <IncidentWorkspace apiBaseUrl={apiUrlInput} language={language} sourceMode={sourceMode} />
            ) : telemetryView ? (
            sourceMode === "api" ? (
              <TelemetryWorkspace view={telemetryView} apiBaseUrl={apiConnectedBaseUrl || apiUrlInput} language={language} workspaceId={apiWorkspaceId} />
            ) : (
              <section className="panel telemetry-connect-prompt">
                <div className="source-placeholder-icon"><Activity size={18} /></div>
                <div>
                  <h2>{tr("connectTelemetryTitle")}</h2>
                  <p>{tr("connectTelemetryBody").replace("{view}", navLabel(language, telemetryView).toLowerCase())}</p>
                </div>
                <button className="button-primary" onClick={() => setSourcePanelOpen(true)}>{tr("connectApiSource")} <ArrowUpRight size={14} /></button>
              </section>
            )
          ) : (
            <>
{sourceMode === "demo" ? (
          <section className="metrics-grid" aria-label={tr("keyMetrics")}>
            <MetricCard label={tr("requestsPerMinute")} value="24.8k" change="12.6%" direction="down" detail={tr("vsPrevious")} icon={Activity} accent="cyan" />
            <MetricCard label={tr("p95Latency")} value={<>142<span>ms</span></>} change="8.2%" direction="down" detail={tr("vsPrevious")} icon={Zap} accent="violet" />
            <MetricCard label={tr("errorRate")} value={<>0.24<span>%</span></>} change="0.04%" direction="up" detail={tr("vsPrevious")} icon={AlertTriangle} accent="amber" />
            <MetricCard label={tr("activeServices")} value={<>12<span>/ 14</span></>} change={"2 " + tr("degraded")} direction="up" detail={tr("acrossRegions")} icon={Boxes} accent="green" />
          </section>
          ) : null}

{sourceMode === "demo" ? (
          <section className="panel traffic-panel">
            <div className="panel-header">
              <div><div className="panel-title-line"><h2>{tr("trafficPerformance")}</h2><span className="live-indicator"><i /> {tr("sample")}</span></div><p>{tr("incomingRequests")}</p></div>
              <div className="range-switcher" role="group" aria-label={tr("last24Hours")}>
                {["1 hour", "6 hours", "24 hours"].map((item) => <button key={item} className={range === item ? "selected" : ""} onClick={() => setRange(item)}>{language === "fa" ? (item === "1 hour" ? "۱ ساعت" : item === "6 hours" ? "۶ ساعت" : "۲۴ ساعت") : item}</button>)}
              </div>
            </div>
            <div className="chart-summary">
              <div><strong>18,429</strong><span>{tr("requestPerMinute")}</span></div>
              <div className="chart-summary-change"><ArrowUpRight size={14} /> 6.4% <span>{tr("comparedPrior")}</span></div>
            </div>
            <div className="chart-wrap">
              <div className="chart-y-labels"><span>30k</span><span>20k</span><span>10k</span><span>0</span></div>
              <div className="chart-plot">
                <div className="chart-grid-lines"><i /><i /><i /><i /></div>
                <svg className="traffic-chart" viewBox="0 0 598 140" preserveAspectRatio="none" role="img" aria-label={tr("illustrativeTrend")}>
                  <defs><linearGradient id="trafficFill" x1="0" x2="0" y1="0" y2="1"><stop offset="0%" stopColor="#65d9c1" stopOpacity=".22" /><stop offset="100%" stopColor="#65d9c1" stopOpacity="0" /></linearGradient></defs>
                  <polygon points={chartArea} fill="url(#trafficFill)" />
                  <polyline points={chartPoints} fill="none" stroke="#65d9c1" strokeWidth="2.2" strokeLinejoin="round" strokeLinecap="round" vectorEffect="non-scaling-stroke" />
                  <circle cx="494" cy="20" r="4" fill="#65d9c1" stroke="#101820" strokeWidth="2" vectorEffect="non-scaling-stroke" />
                </svg>
                <div className="chart-x-labels"><span>09:00</span><span>09:10</span><span>09:20</span><span>09:30</span><span>09:40</span><span>09:50</span><span>10:00</span></div>
              </div>
            </div>
            <div className="chart-legend"><span><i className="legend-line" /> {tr("requestsPerMin")}</span><span><i className="legend-dash" /> {tr("previousPeriod")}</span><span className="chart-footnote">{tr("illustrativeTelemetry")} · {range}</span></div>
          </section>
          ) : null}

          <div className="lower-grid">
            <section className="panel services-panel">
              <div className="panel-header compact">
                <div><div className="panel-title-line"><h2>{tr("serviceHealth")}</h2><span className="subtle-count">{sourceServices.length} {tr("monitored")}</span></div><p>{tr("serviceHealthSubtitle")}</p></div>
                <button className="text-action" onClick={() => setShowAllServices(!showAllServices)}>{showAllServices ? tr("showLess") : tr("viewAll")} <ArrowUpRight size={14} /></button>
              </div>
              <div className="service-table">
                <div className="service-table-head"><span>{tr("service")}</span><span>{tr("latency")}</span><span>{tr("errorRate")}</span><span>{tr("status")}</span></div>
                {filteredServices.map((service) => (
                  <div className="service-row" key={service.name}>
                    <div className="service-name-cell"><div className={`service-avatar ${service.tone}`}>{service.initials}</div><div><strong>{service.name}</strong><span>{service.description}</span></div></div>
                    <span className="service-metric">{service.latency}</span>
                    <span className={`service-metric ${service.state === "Degraded" || service.state === "Incident" ? "metric-warning" : ""}`}>{service.errorRate}</span>
                    <span className="service-status"><StatusDot state={service.state} language={language} />{serviceStateLabel(service.state, language)}</span>
                  </div>
                ))}
                {filteredServices.length === 0 && <div className="empty-state">{tr("noServicesMatch")} “{search}”.</div>}
              </div>
              <div className="panel-footer"><span>{tr("showing")} {filteredServices.length} {tr("of")} {sourceServices.length} {tr("services").toLowerCase()}</span><span className="demo-footnote">{sourceMode === "api" ? tr("apiData") : tr("demoData")}</span></div>
            </section>

            <section className="panel incidents-panel">
              <div className="panel-header compact">
                <div><div className="panel-title-line"><h2>{tr("activeIncidents")}</h2><span className="incident-count">{incidentRows.length}</span></div><p>{tr("issuesAttention")}</p></div>
                <button className="icon-button subtle-more" aria-label={tr("incidentOptions")}><MoreHorizontal size={18} /></button>
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
                )) : <div className="empty-state">{tr("noActiveIncidents")}</div>}
              </div>
              <button className="incident-link" onClick={() => chooseNav("Incidents")}>{tr("incidentTimeline")} <ArrowUpRight size={14} /></button>
            </section>
          </div>

{sourceMode === "demo" ? (
          <section className="bottom-grid">
            <div className="bottom-stat"><div className="bottom-icon green"><ShieldCheck size={17} /></div><div><span>{tr("availabilitySlo")}</span><strong>99.94%</strong></div><span className="bottom-note good"><Check size={13} /> {tr("withinTarget")}</span></div>
            <div className="bottom-stat"><div className="bottom-icon violet"><GitBranch size={17} /></div><div><span>{tr("deploymentsToday")}</span><strong>08</strong></div><span className="bottom-note">{tr("acrossFourServices")}</span></div>
            <div className="bottom-stat"><div className="bottom-icon blue"><Database size={17} /></div><div><span>{tr("dataIngestion")}</span><strong>2.4 MB/s</strong></div><span className="bottom-note"><Signal size={13} /> {tr("sampleStream")}</span></div>
          </section>
          ) : null}

            </>
          )}

          <footer className="page-footer"><span>ORBIT <b>·</b> {tr("systemsObservatory")}</span><span><Globe2 size={12} /> {sourceMode === "api" ? tr("inventoryFromApi") : tr("allIllustrative")} <i /> <span className="footer-version">v0.9.0</span></span><span className="owner-credit"><strong>{tr("projectOwner")}:</strong> <a href="https://github.com/pooya16pg-spec" target="_blank" rel="noreferrer">@pooya16pg-spec</a> <span>·</span> <a href="https://github.com/pooya16pg-spec/orbit" target="_blank" rel="noreferrer">{tr("createdMaintained")}</a></span></footer>
        </div>
      </section>
    </main>
  );
}

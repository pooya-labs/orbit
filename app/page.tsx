"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { getMetadata, listWorkspaceIncidents, listWorkspaceServices, listWorkspaces } from "../lib/orbit/client";
import { getCurrentAuthUser, subscribeToAuth } from "../lib/orbit/oidc";
import { isAppsScriptRuntime } from "../lib/orbit/apps-script-bridge";
import type { ApiMetadata, IncidentSummary, ServiceSummary } from "../lib/orbit/contracts";
import { chartArea, chartPoints, chartSamples, demoIncidents, demoServices } from "../lib/orbit/demo";
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
const ownerMonogramSrc = `${process.env.NEXT_PUBLIC_ORBIT_BASE_PATH ?? ""}/brand/pooya-monogram.webp`;

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

function formatElapsedAge(elapsedMs: number, language: Language): string {
  if (!Number.isFinite(elapsedMs)) return t(language, "timeUnavailable");
  const minutes = Math.floor(Math.max(0, elapsedMs) / 60000);
  const relative = new Intl.RelativeTimeFormat(formatLocale(language), { numeric: "auto" });
  if (minutes < 1) return t(language, "justNow");
  if (minutes < 60) return relative.format(-minutes, "minute");
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return relative.format(-hours, "hour");
  return relative.format(-Math.floor(hours / 24), "day");
}

function formatAge(timestamp: string, language: Language): string {
  return formatElapsedAge(Date.now() - new Date(timestamp).getTime(), language);
}

function formatDemoAge(ageMinutes: number, language: Language): string {
  return formatElapsedAge(ageMinutes * 60000, language);
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
    timeAgo: formatDemoAge(item.ageMinutes, language),
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

function GitHubMark({ size = 13 }: { size?: number }) {
  return (
    <svg aria-hidden="true" width={size} height={size} viewBox="0 0 24 24" fill="currentColor" focusable="false">
      <path d="M12 .5C5.65.5.5 5.75.5 12.24c0 5.2 3.36 9.6 8.02 11.16.59.11.82-.26.82-.58v-2.23c-3.26.72-3.95-1.42-3.95-1.42-.54-1.39-1.3-1.76-1.3-1.76-1.06-.75.08-.74.08-.74 1.17.08 1.79 1.22 1.79 1.22 1.04 1.82 2.73 1.3 3.4.99.11-.77.41-1.3.74-1.6-2.6-.3-5.34-1.33-5.34-5.91 0-1.3.46-2.36 1.21-3.19-.12-.3-.52-1.51.11-3.15 0 0 .99-.32 3.25 1.22a11.1 11.1 0 0 1 5.92 0c2.26-1.54 3.24-1.22 3.24-1.22.64 1.64.24 2.85.12 3.15.75.83 1.2 1.89 1.2 3.19 0 4.59-2.74 5.6-5.35 5.9.42.37.79 1.1.79 2.22v3.3c0 .32.22.69.83.57a11.75 11.75 0 0 0 8-11.15C23.5 5.75 18.35.5 12 .5z" />
    </svg>
  );
}

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
  // Static export and the browser must render the same initial clock labels.
  // Switch to the current UTC clock only after hydration, then refresh each minute.
  const [demoChartNow, setDemoChartNow] = useState(Date.UTC(2026, 10, 5, 12, 0, 0));
  const demoChartWindowMinutes = range === "6 hours" ? 360 : range === "24 hours" ? 1440 : 60;
  const demoChartTimeAt = (elapsedFraction: number) =>
    new Intl.DateTimeFormat(formatLocale(language), {
      hour: "2-digit", minute: "2-digit", timeZone: "UTC",
    }).format(demoChartNow - ((1 - elapsedFraction) * demoChartWindowMinutes * 60_000));
  const demoChartTickLabels = Array.from({ length: 7 }, (_, index) => demoChartTimeAt(index / 6));
  const demoChartRangeLabel = language === "fa"
    ? range === "1 hour" ? "۱ ساعت" : range === "6 hours" ? "۶ ساعت" : "۲۴ ساعت"
    : range;
  const [search, setSearch] = useState("");
  const [refreshing, setRefreshing] = useState(true);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const mobileMenuButtonRef = useRef<HTMLButtonElement>(null);
  const sidebarCloseButtonRef = useRef<HTMLButtonElement>(null);
  const sidebarRef = useRef<HTMLElement>(null);
  const [utilityDialog, setUtilityDialog] = useState<"settings" | "help" | null>(null);
  const utilityDialogRef = useRef<HTMLDivElement>(null);
  const utilityDialogCloseButtonRef = useRef<HTMLButtonElement>(null);
  const settingsButtonRef = useRef<HTMLButtonElement>(null);
  const helpButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!mobileNavOpen) return;

    const sidebar = sidebarRef.current;
    if (!sidebar) return;

    sidebarCloseButtonRef.current?.focus();

    const getFocusableElements = () =>
      Array.from(sidebar.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      )).filter((element) => element.getClientRects().length > 0);

    const handleNavigationKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setMobileNavOpen(false);
        return;
      }
      if (event.key !== "Tab") return;

      const focusable = getFocusableElements();
      if (focusable.length === 0) {
        event.preventDefault();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", handleNavigationKeyDown);
    return () => {
      document.removeEventListener("keydown", handleNavigationKeyDown);
      window.requestAnimationFrame(() => mobileMenuButtonRef.current?.focus());
    };
  }, [mobileNavOpen]);

  useEffect(() => {
    if (!utilityDialog) return;
    const dialog = utilityDialogRef.current;
    if (!dialog) return;

    utilityDialogCloseButtonRef.current?.focus();

    const getFocusableElements = () =>
      Array.from(dialog.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      )).filter((element) => element.getClientRects().length > 0);

    const handleDialogKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setUtilityDialog(null);
        return;
      }
      if (event.key !== "Tab") return;

      const focusable = getFocusableElements();
      if (focusable.length === 0) {
        event.preventDefault();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", handleDialogKeyDown);
    return () => {
      document.removeEventListener("keydown", handleDialogKeyDown);
      window.requestAnimationFrame(() => {
        const trigger = utilityDialog === "settings" ? settingsButtonRef.current : helpButtonRef.current;
        trigger?.focus();
      });
    };
  }, [utilityDialog]);

  const [showAllServices, setShowAllServices] = useState(false);
  const [sourceMode, setSourceMode] = useState<SourceMode>(process.env.NEXT_PUBLIC_ORBIT_LOCAL_WORK === "true" ? "api" : "demo");
  const [sourcePanelOpen, setSourcePanelOpen] = useState(false);
  // Keep the initial state identical during static HTML hydration. The
  // Apps Script runtime is detected after mount in the effect below.
  const [apiUrlInput, setApiUrlInput] = useState(
    process.env.NEXT_PUBLIC_ORBIT_API_URL ?? "http://localhost:8080",
  );
  const [apiStatus, setApiStatus] = useState<"idle" | "connecting" | "connected" | "error">("idle");
  const [apiError, setApiError] = useState("");
  const [apiMetadata, setApiMetadata] = useState<ApiMetadata | null>(null);
  const [apiServices, setApiServices] = useState<ServiceSummary[]>([]);
  const [apiIncidents, setApiIncidents] = useState<IncidentSummary[]>([]);
  const [authUser, setAuthUser] = useState<ReturnType<typeof getCurrentAuthUser>>(null);

  useEffect(() => {
    setAuthUser(getCurrentAuthUser());
    return subscribeToAuth(() => setAuthUser(getCurrentAuthUser()));
  }, []);

  useEffect(() => {
    if (!sourcePanelOpen) return;
    const frame = window.requestAnimationFrame(() => document.getElementById("orbit-api-url")?.focus());
    return () => window.cancelAnimationFrame(frame);
  }, [sourcePanelOpen]);

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
    const updateDemoClock = () => setDemoChartNow(Date.now());
    updateDemoClock();
    const clock = window.setInterval(updateDemoClock, 60_000);
    return () => window.clearInterval(clock);
  }, []);

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
    if (isAppsScriptRuntime()) {
      setApiUrlInput("apps-script://bridge");
      return;
    }
    const runtime = new URLSearchParams(window.location.search).get("orbitRuntime");
    if (runtime === "cloudflare") {
      setApiUrlInput(
        (process.env.NEXT_PUBLIC_ORBIT_CLOUDFLARE_API_URL || "https://orbit-runtime.pooyagame141.workers.dev").trim(),
      );
      setSourcePanelOpen(true);
      return;
    }
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
    if (isAppsScriptRuntime()) {
      endpoint = "apps-script://bridge";
    } else {
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
    }

    setApiStatus("connecting");
    setApiError("");
    try {
      const metadata = await getMetadata(undefined, endpoint);
      let serviceItems: ServiceSummary[] = [];
      let incidentItems: IncidentSummary[] = [];
      // Connecting an API is a metadata handshake, not a workspace authorization
      // check. Let unauthenticated users connect so the workspace views can
      // present the sign-in requirement without falling back to global data.
      if (getCurrentAuthUser()) {
        const workspaceItems = await listWorkspaces(undefined, endpoint);
        const firstWorkspace = workspaceItems[0];
        if (firstWorkspace) {
          [serviceItems, incidentItems] = await Promise.all([
            listWorkspaceServices(firstWorkspace.id, undefined, endpoint),
            listWorkspaceIncidents(firstWorkspace.id, undefined, undefined, endpoint),
          ]);
        }
      }
      setApiMetadata(metadata);
      setApiServices(serviceItems);
      setApiIncidents(incidentItems);
      setApiUrlInput(endpoint);
      setSourceMode("api");
      setApiStatus("connected");
      if (!isAppsScriptRuntime()) {
        try {
          window.localStorage.setItem("orbit.apiBaseUrl", endpoint);
        } catch {
          // A working connection should not depend on local storage support.
        }
      }
      setSourcePanelOpen(false);
    } catch (error) {
      setApiStatus("error");
      setApiError(error instanceof Error ? error.message : tr("connectFailed"));
    }
  };

  const localWorkAutoConnectUser = useRef("");
  useEffect(() => {
    if (process.env.NEXT_PUBLIC_ORBIT_LOCAL_WORK !== "true") return;
    if (!authUser) {
      setSourceMode("api");
      setApiUrlInput(process.env.NEXT_PUBLIC_ORBIT_API_URL || "http://localhost:8080");
      setApiMetadata(null);
      setApiServices([]);
      setApiIncidents([]);
      setApiStatus("idle");
      return;
    }
    if (localWorkAutoConnectUser.current === authUser.sub) return;
    localWorkAutoConnectUser.current = authUser.sub;
    void connectSource();
  }, [authUser?.sub]);

  useEffect(() => {
    if (authUser) return;
    localWorkAutoConnectUser.current = "";
  }, [authUser?.sub]);

  const appsScriptAutoConnectStarted = useRef(false);
  useEffect(() => {
    if (!isAppsScriptRuntime() || appsScriptAutoConnectStarted.current) return;
    appsScriptAutoConnectStarted.current = true;
    void connectSource();
  }, []);

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
    <div className="app-shell" data-theme={theme} dir={language === "fa" ? "rtl" : "ltr"} lang={language}>
      <a className="skip-link" href="#main-content">{tr("skipToMainContent")}</a>
      {mobileNavOpen && <button type="button" className="mobile-scrim" aria-hidden="true" tabIndex={-1} onClick={() => setMobileNavOpen(false)} />}
      <aside ref={sidebarRef} className={`sidebar ${mobileNavOpen ? "sidebar-open" : ""}`}>
        <div className="brand-lockup">
          <div className="brand-mark"><span /><span /><span /></div>
          <div className="brand-wordmark">ORBIT<span>.</span><small>OBSERVABILITY</small></div>
          <button ref={sidebarCloseButtonRef} type="button" className="icon-button sidebar-close" aria-label={tr("closeNavigation")} onClick={() => setMobileNavOpen(false)}><X size={18} /></button>
        </div>

        <button type="button" className="workspace-switcher preview-unavailable" disabled title={tr("notAvailableInPreview")}>
          <div className="workspace-avatar" aria-hidden="true">O</div>
          <div className="workspace-copy"><strong>ORBIT</strong><span>{language === "fa" ? "فضای کاری شخصی" : "Personal workspace"}</span></div>
          <ChevronDown size={15} className="muted-icon" />
        </button>

        <nav className="sidebar-scroll" id="primary-navigation" aria-label={tr("primaryNavigation")}>
          {navGroups.map((group) => (
            <div className="nav-group" key={group.label}>
              <div className="nav-group-label" role="heading" aria-level={2}>{group.label === "OBSERVE" ? tr("observe") : group.label === "INVESTIGATE" ? tr("investigate") : tr("deliver")}</div>
              {group.items.map(({ label, icon: Icon, count, alert }) => (
                <button type="button" key={label} className={`nav-item ${activeNav === label ? "active" : ""}`} aria-current={activeNav === label ? "page" : undefined} onClick={() => chooseNav(label)}>
                  <Icon size={17} strokeWidth={1.75} />
                  <span>{navLabel(language, label)}</span>
                  {count && <span className={`nav-count ${alert ? "nav-alert" : ""}`}>{count}</span>}
                </button>
              ))}
            </div>
          ))}
        </nav>

        <div className="sidebar-bottom">
          {sourceMode === "demo" && (
            <div className="usage-card">
              <div className="usage-heading"><span>{tr("demoVolume")}</span><span>68%</span></div>
              <div className="usage-track"><span /></div>
              <p>6.8 GB <span>{tr("illustrativeUsage")}</span></p>
            </div>
          )}
          <button ref={settingsButtonRef} type="button" className="nav-item" aria-haspopup="dialog" aria-expanded={utilityDialog === "settings"} onClick={() => setUtilityDialog("settings")}><Settings2 size={17} /><span>{tr("settings")}</span></button>
          <button ref={helpButtonRef} type="button" className="nav-item" aria-haspopup="dialog" aria-expanded={utilityDialog === "help"} onClick={() => setUtilityDialog("help")}><CircleHelp size={17} /><span>{tr("helpSupport")}</span></button>
          <div className="profile-row">
            <a className="profile-brand-link" href="https://pooya-labs.github.io/pooya-labs-platform/" target="_blank" rel="noreferrer" aria-label="Open Pooya Labs Platform website">
              <span className="profile-avatar" aria-hidden="true"><img src={ownerMonogramSrc} alt="" width={29} height={29} /></span>
              <span className="profile-copy"><span>Pooya Labs Platform</span></span>
            </a>
          </div>
        </div>
      </aside>

      <section className="main-panel">
        <header className="topbar">
          <div className="topbar-left">
            <button ref={mobileMenuButtonRef} type="button" className="icon-button mobile-menu" aria-label={tr("openNavigation")} aria-expanded={mobileNavOpen} aria-controls="primary-navigation" onClick={() => setMobileNavOpen(true)}><Menu size={19} /></button>
            <div className="breadcrumbs"><span>{tr("workspace")}</span><span className="crumb-separator">/</span><strong>{navLabel(language, activeNav)}</strong></div>
            <span className="environment-pill"><span /> {sourceMode === "api" ? (apiMetadata?.environment ?? tr("apiConnected")) : tr("demoEnvironment")}</span>
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
            <button type="button" className="icon-button notification-button" aria-label={tr("notifications")} onClick={() => chooseNav("Notifications")}><Bell size={17} /><i /></button>
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
            <a className="help-button" href="https://github.com/pooya-labs/orbit/tree/main/docs" target="_blank" rel="noreferrer"><CircleHelp size={16} /><span>{tr("docs")}</span></a>
          </div>
        </header>

        <main className="content-wrap" id="main-content" tabIndex={-1}>
          <div className="page-heading">
            <div>
              <div className="eyebrow"><span className="eyebrow-line" /> {telemetryView ? `${language === "fa" ? "زنده" : "LIVE"} ${navLabel(language, telemetryView).toUpperCase()}` : tr("systemOverview")} <span className="demo-tag">{sourceMode === "api" ? tr("apiConnected") : tr("demoEnvironment")}</span></div>
              <h1 aria-live="polite" aria-atomic="true">{activeNav === "Overview" ? tr("goodMorning") : navLabel(language, activeNav)}</h1>
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
              <TelemetryWorkspace view={telemetryView} apiBaseUrl={apiUrlInput} language={language} apiEnvironment={apiMetadata?.environment ?? "unknown"} />
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
              <div className="range-switcher" role="group" aria-label={tr("timeRange")}>
                {["1 hour", "6 hours", "24 hours"].map((item) => <button key={item} className={range === item ? "selected" : ""} aria-pressed={range === item} onClick={() => setRange(item)}>{language === "fa" ? (item === "1 hour" ? "۱ ساعت" : item === "6 hours" ? "۶ ساعت" : "۲۴ ساعت") : item}</button>)}
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
                <div className="chart-x-labels">{demoChartTickLabels.map((label, index) => <span key={index}>{label}</span>)}</div>
              </div>
            </div>
            <div className="chart-legend"><span><i className="legend-line" /> {tr("requestsPerMin")}</span><span><i className="legend-dash" /> {tr("previousPeriod")}</span><span className="chart-footnote">{tr("illustrativeTelemetry")} · {demoChartRangeLabel}</span></div>
            <details className="telemetry-data-table overview-chart-data-table">
              <summary>{tr("showOverviewChartData")}</summary>
              <div className="telemetry-data-table-scroll" role="region" aria-label={tr("overviewChartDataCaption").replace("{range}", demoChartRangeLabel)} tabIndex={0}>
                <table>
                  <caption>{tr("overviewChartDataCaption").replace("{range}", demoChartRangeLabel)}</caption>
                  <thead>
                    <tr>
                      <th scope="col">{tr("chartTime")}</th>
                      <th scope="col">{tr("overviewChartValue")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {chartSamples.map((sample) => (
                      <tr key={sample.id}>
                        <td>{demoChartTimeAt(sample.elapsedFraction)}</td>
                        <td>{new Intl.NumberFormat(formatLocale(language), { maximumFractionDigits: 0 }).format(sample.requestsPerMinute)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </details>
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
                <button type="button" className="icon-button subtle-more preview-unavailable" disabled aria-label={`${tr("incidentOptions")} — ${tr("notAvailableInPreview")}`} title={tr("notAvailableInPreview")}><MoreHorizontal size={18} /></button>
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

          <footer className="page-footer"><span>ORBIT <b>·</b> {tr("systemsObservatory")}</span><span><Globe2 size={12} /> {sourceMode === "api" ? tr("inventoryFromApi") : tr("allIllustrative")} <i /> <span className="footer-version">v1.0.0</span></span><span className="owner-credit"><a href="https://github.com/pooya-labs/orbit" target="_blank" rel="noreferrer">{tr("createdMaintained")}</a> <span>·</span> <strong>{tr("projectOwner")}:</strong> <a className="owner-github-link" href="https://pooya-labs.github.io/pooya-labs-platform/" target="_blank" rel="noreferrer" aria-label="Open Pooya Labs Platform website"><img className="owner-brand-mark-footer" src={ownerMonogramSrc} alt="" width={16} height={16} /><span>Pooya Labs Platform</span></a></span></footer>
        </main>
      </section>
      {utilityDialog && (
        <div
          className="utility-dialog-backdrop"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setUtilityDialog(null);
          }}
        >
          <div
            ref={utilityDialogRef}
            className="utility-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="utility-dialog-title"
            aria-describedby="utility-dialog-description"
          >
            <div className="utility-dialog-header">
              <div className="utility-dialog-heading">
                <span className="utility-dialog-icon">
                  {utilityDialog === "settings" ? <Settings2 size={18} /> : <CircleHelp size={18} />}
                </span>
                <div>
                  <h2 id="utility-dialog-title">{utilityDialog === "settings" ? tr("settingsDialogTitle") : tr("helpDialogTitle")}</h2>
                  <p id="utility-dialog-description">{utilityDialog === "settings" ? tr("settingsDialogDescription") : tr("helpDialogDescription")}</p>
                </div>
              </div>
              <button
                ref={utilityDialogCloseButtonRef}
                type="button"
                className="icon-button utility-dialog-close"
                aria-label={tr("closeDialog")}
                onClick={() => setUtilityDialog(null)}
              >
                <X size={17} />
              </button>
            </div>

            {utilityDialog === "settings" ? (
              <div className="utility-dialog-content">
                <section className="utility-setting-card" aria-labelledby="appearance-title">
                  <div className="utility-section-heading">
                    <div>
                      <h3 id="appearance-title">{tr("appearance")}</h3>
                      <p>{tr("appearanceDescription")}</p>
                    </div>
                  </div>
                  <div className="utility-choice-group" role="group" aria-label={tr("appearance")}>
                    <button type="button" className={theme === "dark" ? "selected" : ""} aria-pressed={theme === "dark"} onClick={() => setTheme("dark")}><Moon size={15} />{tr("darkAppearance")}</button>
                    <button type="button" className={theme === "light" ? "selected" : ""} aria-pressed={theme === "light"} onClick={() => setTheme("light")}><Sun size={15} />{tr("lightAppearance")}</button>
                  </div>
                </section>

                <section className="utility-setting-card" aria-labelledby="language-title">
                  <div className="utility-section-heading">
                    <div>
                      <h3 id="language-title">{tr("languagePreference")}</h3>
                      <p>{tr("languagePreferenceDescription")}</p>
                    </div>
                  </div>
                  <div className="utility-choice-group" role="group" aria-label={tr("languagePreference")}>
                    <button type="button" className={language === "en" ? "selected" : ""} aria-pressed={language === "en"} onClick={() => changeLanguage("en")}>{tr("english")}</button>
                    <button type="button" className={language === "fa" ? "selected" : ""} aria-pressed={language === "fa"} onClick={() => changeLanguage("fa")}>{tr("persian")}</button>
                  </div>
                </section>

                <section className="utility-setting-card utility-source-setting" aria-labelledby="source-title">
                  <div className="utility-section-heading">
                    <div>
                      <h3 id="source-title">{tr("sourceSection")}</h3>
                      <p>{sourceMode === "api" ? tr("apiSourceStatus") : tr("demoSourceStatus")}</p>
                    </div>
                    <span className={sourceMode === "api" ? "utility-source-indicator connected" : "utility-source-indicator demo"} />
                  </div>
                  <button type="button" className="button-secondary" onClick={() => { setUtilityDialog(null); setSourcePanelOpen(true); }}>{tr("manageSource")}</button>
                </section>

                <p className="utility-dialog-footnote">{tr("preferencesSaved")}</p>
              </div>
            ) : (
              <div className="utility-dialog-content">
                <section className="utility-help-section" aria-labelledby="quick-links-title">
                  <h3 id="quick-links-title">{tr("quickLinks")}</h3>
                  <div className="utility-help-links">
                    <a className="utility-help-link" href="https://github.com/pooya-labs/orbit/blob/main/README.md" target="_blank" rel="noreferrer"><FileText size={15} /><span>{tr("projectReadme")}</span><ExternalLink size={12} /></a>
                    <a className="utility-help-link" href="https://github.com/pooya-labs/orbit/blob/main/docs/roadmap.md" target="_blank" rel="noreferrer"><Workflow size={15} /><span>{tr("roadmap")}</span><ExternalLink size={12} /></a>
                    <a className="utility-help-link" href="https://github.com/pooya-labs/orbit/blob/main/docs/architecture.md" target="_blank" rel="noreferrer"><Network size={15} /><span>{tr("architecture")}</span><ExternalLink size={12} /></a>
                    <a className="utility-help-link" href="https://github.com/pooya-labs/orbit/blob/main/apps-script/README.md" target="_blank" rel="noreferrer"><Database size={15} /><span>{tr("appsScriptSetup")}</span><ExternalLink size={12} /></a>
                    <a className="utility-help-link" href="https://github.com/pooya-labs/orbit/blob/main/docs/manual-acceptance-guide.md" target="_blank" rel="noreferrer"><ShieldCheck size={15} /><span>{tr("accessibilityGuide")}</span><ExternalLink size={12} /></a>
                    <a className="utility-help-link" href="https://github.com/pooya-labs/orbit/releases/tag/v1.0.0" target="_blank" rel="noreferrer"><FileText size={15} /><span>{tr("releaseNotes")}</span><ExternalLink size={12} /></a>
                    <a className="utility-help-link" href="https://github.com/pooya-labs" target="_blank" rel="noreferrer"><GitHubMark size={15} /><span>{tr("githubProfile")}</span><ExternalLink size={12} /></a>
                    <a className="utility-help-link" href="https://github.com/pooya-labs/orbit/issues" target="_blank" rel="noreferrer"><CircleHelp size={15} /><span>{tr("reportIssue")}</span><ExternalLink size={12} /></a>
                  </div>
                </section>
                <p className="utility-dialog-footnote">{tr("helpDemoNotice")}</p>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

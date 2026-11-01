"use client";

import { useMemo, useState } from "react";
import type { ReactNode } from "react";
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

type ServiceState = "Operational" | "Degraded" | "Incident";

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

const services: Service[] = [
  { name: "api-gateway", description: "Edge request router", region: "eu-central-1", latency: "42 ms", errorRate: "0.03%", state: "Operational", initials: "AG", tone: "blue" },
  { name: "identity-core", description: "Authentication service", region: "eu-central-1", latency: "68 ms", errorRate: "0.08%", state: "Operational", initials: "ID", tone: "violet" },
  { name: "billing-engine", description: "Payments & invoicing", region: "eu-west-1", latency: "184 ms", errorRate: "1.42%", state: "Degraded", initials: "BE", tone: "amber" },
  { name: "event-stream", description: "Async event processing", region: "eu-central-1", latency: "31 ms", errorRate: "0.01%", state: "Operational", initials: "ES", tone: "teal" },
  { name: "search-indexer", description: "Search infrastructure", region: "us-east-1", latency: "—", errorRate: "—", state: "Incident", initials: "SI", tone: "rose" },
];

const navGroups = [
  {
    label: "OBSERVE",
    items: [
      { label: "Overview", icon: Gauge },
      { label: "Service map", icon: Network },
      { label: "Services", icon: Boxes, count: "12" },
      { label: "Infrastructure", icon: Server },
    ],
  },
  {
    label: "INVESTIGATE",
    items: [
      { label: "Metrics", icon: Activity },
      { label: "Traces", icon: Workflow },
      { label: "Logs", icon: TerminalSquare },
      { label: "Incidents", icon: AlertTriangle, count: "2", alert: true },
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

const chartPoints = "0,118 26,110 52,114 78,92 104,99 130,77 156,84 182,72 208,88 234,62 260,70 286,55 312,68 338,45 364,51 390,37 416,48 442,31 468,39 494,20 520,34 546,23 572,29 598,12";
const chartArea = `0,136 ${chartPoints} 598,136`;

function StatusDot({ state }: { state: ServiceState }) {
  return <span className={`status-dot ${state.toLowerCase()}`} aria-label={state} />;
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
  const [range, setRange] = useState("1 hour");
  const [search, setSearch] = useState("");
  const [refreshing, setRefreshing] = useState(true);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [showAllServices, setShowAllServices] = useState(false);

  const filteredServices = useMemo(() => {
    const query = search.trim().toLowerCase();
    const matching = services.filter((service) =>
      `${service.name} ${service.description} ${service.region}`.toLowerCase().includes(query),
    );
    return showAllServices ? matching : matching.slice(0, 4);
  }, [search, showAllServices]);

  const chooseNav = (label: string) => {
    setActiveNav(label);
    setMobileNavOpen(false);
  };

  return (
    <main className="app-shell">
      {mobileNavOpen && <button className="mobile-scrim" aria-label="Close navigation" onClick={() => setMobileNavOpen(false)} />}
      <aside className={`sidebar ${mobileNavOpen ? "sidebar-open" : ""}`}>
        <div className="brand-lockup">
          <div className="brand-mark"><span /><span /><span /></div>
          <div className="brand-wordmark">ORBIT<span>.</span><small>OBSERVABILITY</small></div>
          <button className="icon-button sidebar-close" aria-label="Close navigation" onClick={() => setMobileNavOpen(false)}><X size={18} /></button>
        </div>

        <button className="workspace-switcher">
          <div className="workspace-avatar">N</div>
          <div className="workspace-copy"><strong>Northstar Systems</strong><span>Production workspace</span></div>
          <ChevronDown size={15} className="muted-icon" />
        </button>

        <div className="sidebar-scroll">
          {navGroups.map((group) => (
            <div className="nav-group" key={group.label}>
              <div className="nav-group-label">{group.label}</div>
              {group.items.map(({ label, icon: Icon, count, alert }) => (
                <button key={label} className={`nav-item ${activeNav === label ? "active" : ""}`} onClick={() => chooseNav(label)}>
                  <Icon size={17} strokeWidth={1.75} />
                  <span>{label}</span>
                  {count && <span className={`nav-count ${alert ? "nav-alert" : ""}`}>{count}</span>}
                </button>
              ))}
            </div>
          ))}
        </div>

        <div className="sidebar-bottom">
          <div className="usage-card">
            <div className="usage-heading"><span>Telemetry volume</span><span>68%</span></div>
            <div className="usage-track"><span /></div>
            <p>6.8 GB <span>of 10 GB monthly</span></p>
          </div>
          <button className="nav-item"><Settings2 size={17} /><span>Settings</span></button>
          <button className="nav-item"><CircleHelp size={17} /><span>Help & support</span><ExternalLink size={12} className="nav-external" /></button>
          <div className="profile-row">
            <div className="profile-avatar">PG</div>
            <div className="profile-copy"><strong>Platform team</strong><span>platform@northstar.dev</span></div>
            <MoreHorizontal size={18} className="muted-icon" />
          </div>
        </div>
      </aside>

      <section className="main-panel">
        <header className="topbar">
          <div className="topbar-left">
            <button className="icon-button mobile-menu" aria-label="Open navigation" onClick={() => setMobileNavOpen(true)}><Menu size={19} /></button>
            <div className="breadcrumbs"><span>Workspace</span><span className="crumb-separator">/</span><strong>{activeNav}</strong></div>
            <span className="environment-pill"><span /> PRODUCTION</span>
          </div>
          <div className="topbar-actions">
            <label className="global-search">
              <Search size={15} />
              <input aria-label="Search services" placeholder="Search services..." value={search} onChange={(event) => setSearch(event.target.value)} />
              <kbd>/</kbd>
            </label>
            <button className={`icon-button refresh-button ${refreshing ? "is-on" : ""}`} aria-label={refreshing ? "Pause auto refresh" : "Resume auto refresh"} title={refreshing ? "Pause auto refresh" : "Resume auto refresh"} onClick={() => setRefreshing(!refreshing)}>
              {refreshing ? <Pause size={16} /> : <Play size={16} />}
            </button>
            <button className="icon-button notification-button" aria-label="Notifications"><Bell size={17} /><i /></button>
            <div className="topbar-divider" />
            <button className="help-button"><CircleHelp size={16} /><span>Docs</span></button>
          </div>
        </header>

        <div className="content-wrap">
          <div className="page-heading">
            <div>
              <div className="eyebrow"><span className="eyebrow-line" /> SYSTEM OVERVIEW <span className="demo-tag">DEMO ENVIRONMENT</span></div>
              <h1>{activeNav === "Overview" ? "Good morning, team." : activeNav}</h1>
              <p className="page-subtitle">Here&apos;s what&apos;s happening across your infrastructure.</p>
            </div>
            <div className="heading-actions">
              <div className="last-updated"><span className={refreshing ? "pulse-dot" : "idle-dot"} /> {refreshing ? "Auto-refresh paused in demo" : "Manual refresh mode"}</div>
              <button className="button-secondary" onClick={() => setRange("24 hours")}><Clock3 size={15} /> Last 24 hours <ChevronDown size={14} /></button>
              <button className="button-primary" onClick={() => chooseNav("Service map")}><Network size={15} /> Explore topology</button>
            </div>
          </div>

          <div className="notice-strip">
            <div className="notice-icon"><Radio size={16} /></div>
            <div><strong>You're viewing a sample environment.</strong><span> Metrics, services, and incidents below are illustrative demo data — no production systems are connected.</span></div>
            <button onClick={() => chooseNav("Settings")}>Connect a source <ArrowUpRight size={14} /></button>
          </div>

          <section className="metrics-grid" aria-label="Key metrics">
            <MetricCard label="REQUESTS / MIN" value="24.8k" change="12.6%" direction="down" detail="vs. previous period" icon={Activity} accent="cyan" />
            <MetricCard label="P95 LATENCY" value={<>142<span>ms</span></>} change="8.2%" direction="down" detail="vs. previous period" icon={Zap} accent="violet" />
            <MetricCard label="ERROR RATE" value={<>0.24<span>%</span></>} change="0.04%" direction="up" detail="vs. previous period" icon={AlertTriangle} accent="amber" />
            <MetricCard label="ACTIVE SERVICES" value={<>12<span>/ 14</span></>} change="2 degraded" direction="up" detail="across 3 regions" icon={Boxes} accent="green" />
          </section>

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

          <div className="lower-grid">
            <section className="panel services-panel">
              <div className="panel-header compact">
                <div><div className="panel-title-line"><h2>Service health</h2><span className="subtle-count">12 monitored</span></div><p>Health across your distributed services</p></div>
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
              <div className="panel-footer"><span>Showing {filteredServices.length} of 12 services</span><span className="demo-footnote">DEMO DATA</span></div>
            </section>

            <section className="panel incidents-panel">
              <div className="panel-header compact">
                <div><div className="panel-title-line"><h2>Active incidents</h2><span className="incident-count">2</span></div><p>Issues that may need attention</p></div>
                <button className="icon-button subtle-more" aria-label="Incident options"><MoreHorizontal size={18} /></button>
              </div>
              <div className="incident-list">
                <article className="incident-item">
                  <div className="incident-severity critical"><AlertTriangle size={15} /></div>
                  <div className="incident-copy"><div className="incident-title-row"><strong>Search indexing stalled</strong><span className="severity-label critical-text">CRITICAL</span></div><p>search-indexer · us-east-1</p><div className="incident-meta"><Clock3 size={12} /> 14 min ago <span>·</span> <span className="incident-open">Investigating</span></div></div>
                </article>
                <article className="incident-item">
                  <div className="incident-severity warning"><AlertTriangle size={15} /></div>
                  <div className="incident-copy"><div className="incident-title-row"><strong>Elevated payment latency</strong><span className="severity-label warning-text">WARNING</span></div><p>billing-engine · eu-west-1</p><div className="incident-meta"><Clock3 size={12} /> 38 min ago <span>·</span> <span className="incident-open">Monitoring</span></div></div>
                </article>
              </div>
              <button className="incident-link" onClick={() => chooseNav("Incidents")}>View incident timeline <ArrowUpRight size={14} /></button>
            </section>
          </div>

          <section className="bottom-grid">
            <div className="bottom-stat"><div className="bottom-icon green"><ShieldCheck size={17} /></div><div><span>AVAILABILITY SLO</span><strong>99.94%</strong></div><span className="bottom-note good"><Check size={13} /> Within target</span></div>
            <div className="bottom-stat"><div className="bottom-icon violet"><GitBranch size={17} /></div><div><span>DEPLOYMENTS TODAY</span><strong>08</strong></div><span className="bottom-note">Across 4 services</span></div>
            <div className="bottom-stat"><div className="bottom-icon blue"><Database size={17} /></div><div><span>DATA INGESTION</span><strong>2.4 MB/s</strong></div><span className="bottom-note"><Signal size={13} /> Sample stream</span></div>
          </section>

          <footer className="page-footer"><span>ORBIT <b>·</b> SYSTEMS OBSERVATORY</span><span><Globe2 size={12} /> All systems are illustrative <i /> <span className="footer-version">v0.1.0</span></span></footer>
        </div>
      </section>
    </main>
  );
}

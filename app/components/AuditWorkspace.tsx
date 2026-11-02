"use client";

import { useEffect, useMemo, useState } from "react";
import { Clock3, FileText, RefreshCw, ShieldCheck } from "lucide-react";
import {
  getWorkspaceMembership,
  listWorkspaceAuditEvents,
  listWorkspaces,
} from "../../lib/orbit/client";
import type {
  WorkspaceAuditEvent,
  WorkspaceMembershipSummary,
  WorkspaceSummary,
} from "../../lib/orbit/contracts";
import { formatLocale, t, type Language, type TranslationKey } from "../../lib/orbit/i18n";
import {
  getCurrentAuthUser,
  isOidcConfigured,
  subscribeToAuth,
  type OrbitAuthUser,
} from "../../lib/orbit/oidc";

type Props = {
  apiBaseUrl: string;
  language: Language;
  sourceMode: "demo" | "api";
};

type AuditCategory = "all" | "incidents" | "alerts" | "notifications" | "deployments" | "slos" | "workspace";

const filters: { value: AuditCategory; label: TranslationKey }[] = [
  { value: "all", label: "auditFilterAll" },
  { value: "incidents", label: "auditFilterIncidents" },
  { value: "alerts", label: "auditFilterAlerts" },
  { value: "notifications", label: "auditFilterNotifications" },
  { value: "deployments", label: "auditFilterDeployments" },
  { value: "slos", label: "auditFilterSLOs" },
  { value: "workspace", label: "auditFilterWorkspace" },
];

function categoryFor(event: WorkspaceAuditEvent): Exclude<AuditCategory, "all"> {
  switch (event.resource_type) {
    case "incident": return "incidents";
    case "alert_rule":
    case "alert_instance": return "alerts";
    case "alert_notification_channel":
    case "alert_notification_delivery": return "notifications";
    case "deployment_annotation": return "deployments";
    case "slo_definition": return "slos";
    default: return "workspace";
  }
}

function categoryKey(category: Exclude<AuditCategory, "all">): TranslationKey {
  switch (category) {
    case "incidents": return "auditFilterIncidents";
    case "alerts": return "auditFilterAlerts";
    case "notifications": return "auditFilterNotifications";
    case "deployments": return "auditFilterDeployments";
    case "slos": return "auditFilterSLOs";
    case "workspace": return "auditFilterWorkspace";
  }
}

function dateTime(value: string, language: Language): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat(formatLocale(language), {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function actionLabel(action: string): string {
  return action
    .split(/[._]/g)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" · ");
}

function sanitizeValue(value: unknown): unknown {
  if (typeof value === "string") return value.length > 500 ? value.slice(0, 500) + "…" : value;
  if (Array.isArray(value)) return value.slice(0, 10).map(sanitizeValue);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .filter(([key]) => !/(secret|token|credential|password|authorization|webhook|url)/i.test(key))
        .slice(0, 10)
        .map(([key, item]) => [key, sanitizeValue(item)]),
    );
  }
  return value;
}

function displayValue(value: unknown): string {
  const safe = sanitizeValue(value);
  if (typeof safe === "string") return safe;
  try {
    return JSON.stringify(safe) ?? "—";
  } catch {
    return "—";
  }
}

export default function AuditWorkspace({ apiBaseUrl, language, sourceMode }: Props) {
  const [user, setUser] = useState<OrbitAuthUser | null>(() => getCurrentAuthUser());
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([]);
  const [workspaceId, setWorkspaceId] = useState("");
  const [membership, setMembership] = useState<WorkspaceMembershipSummary | null>(null);
  const [events, setEvents] = useState<WorkspaceAuditEvent[]>([]);
  const [filter, setFilter] = useState<AuditCategory>("all");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [reloadVersion, setReloadVersion] = useState(0);

  useEffect(() => subscribeToAuth(() => {
    setUser(getCurrentAuthUser());
    setWorkspaces([]);
    setWorkspaceId("");
    setMembership(null);
    setEvents([]);
  }), []);

  useEffect(() => {
    if (sourceMode !== "api" || !user || !isOidcConfigured()) {
      setWorkspaces([]);
      setWorkspaceId("");
      setMembership(null);
      setEvents([]);
      return;
    }
    const controller = new AbortController();
    let current = true;
    setLoading(true);
    setError("");
    listWorkspaces(controller.signal, apiBaseUrl)
      .then((items) => {
        if (!current) return;
        setWorkspaces(items);
        setWorkspaceId((previous) =>
          previous && items.some((workspace) => workspace.id === previous) ? previous : items[0]?.id ?? "",
        );
      })
      .catch(() => {
        if (current && !controller.signal.aborted) setError(t(language, "auditLoadFailed"));
      })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; controller.abort(); };
  }, [apiBaseUrl, language, sourceMode, user?.sub]);

  useEffect(() => {
    if (!workspaceId || sourceMode !== "api" || !user || !isOidcConfigured()) return;
    const controller = new AbortController();
    let current = true;
    setLoading(true);
    setError("");
    Promise.all([
      getWorkspaceMembership(workspaceId, controller.signal, apiBaseUrl),
      listWorkspaceAuditEvents(workspaceId, 100, controller.signal, apiBaseUrl),
    ])
      .then(([role, items]) => {
        if (!current) return;
        setMembership(role);
        setEvents(items);
      })
      .catch(() => {
        if (current && !controller.signal.aborted) {
          setMembership(null);
          setEvents([]);
          setError(t(language, "auditLoadFailed"));
        }
      })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; controller.abort(); };
  }, [apiBaseUrl, language, sourceMode, user?.sub, workspaceId, reloadVersion]);

  useEffect(() => {
    if (sourceMode !== "api" || !user || !workspaceId || !isOidcConfigured()) return;
    const timer = window.setInterval(() => setReloadVersion((version) => version + 1), 30_000);
    return () => window.clearInterval(timer);
  }, [sourceMode, user?.sub, workspaceId]);

  const visibleEvents = useMemo(
    () => filter === "all" ? events : events.filter((event) => categoryFor(event) === filter),
    [events, filter],
  );

  if (sourceMode === "demo") {
    return (
      <section className="panel audit-empty-state">
        <div className="audit-empty-icon"><ShieldCheck size={20} /></div>
        <h2>{t(language, "auditWorkspaceTitle")}</h2>
        <p>{t(language, "auditDemoReadOnly")}</p>
      </section>
    );
  }

  if (!user) {
    return (
      <section className="panel audit-empty-state">
        <div className="audit-empty-icon"><ShieldCheck size={20} /></div>
        <h2>{t(language, "auditWorkspaceTitle")}</h2>
        <p>{t(language, "auditSignInRequired")}</p>
      </section>
    );
  }

  if (!isOidcConfigured()) {
    return (
      <section className="panel audit-empty-state">
        <div className="audit-empty-icon"><ShieldCheck size={20} /></div>
        <h2>{t(language, "auditWorkspaceTitle")}</h2>
        <p>{t(language, "auditAuthNotConfigured")}</p>
      </section>
    );
  }

  return (
    <section className="audit-workspace">
      <div className="audit-toolbar">
        <label className="audit-workspace-select">
          <span>{t(language, "auditSelectWorkspace")}</span>
          <select value={workspaceId} onChange={(event) => setWorkspaceId(event.target.value)} disabled={loading || workspaces.length === 0}>
            {workspaces.map((workspace) => (
              <option key={workspace.id} value={workspace.id}>{workspace.name}</option>
            ))}
          </select>
        </label>
        <div className="audit-toolbar-actions">
          {membership && <span className="audit-role"><ShieldCheck size={13} /> {membership.role}</span>}
          <button type="button" className="button-secondary" onClick={() => setReloadVersion((version) => version + 1)} disabled={loading || !workspaceId}>
            <RefreshCw size={14} /> {t(language, "auditRefresh")}
          </button>
        </div>
      </div>

      {error && <div className="audit-error" role="alert">{error}</div>}

      {workspaces.length === 0 && !loading ? (
        <section className="panel audit-empty-state">
          <div className="audit-empty-icon"><FileText size={20} /></div>
          <h2>{t(language, "auditWorkspaceTitle")}</h2>
          <p>{t(language, "auditNoWorkspaces")}</p>
        </section>
      ) : (
        <>
          <div className="audit-summary">
            <div><strong>{new Intl.NumberFormat(formatLocale(language)).format(events.length)}</strong><span>{t(language, "auditEventsLoaded")}</span></div>
            <div><strong>{t(language, "auditImmutable")}</strong><span>{t(language, "auditRetentionNote")}</span></div>
            <div className="audit-updated"><Clock3 size={14} /><span>{t(language, "auditLatest100")}</span></div>
          </div>

          <div className="audit-filters" role="group" aria-label={t(language, "auditFilterLabel")}>
            {filters.map((item) => (
              <button
                type="button"
                key={item.value}
                className={filter === item.value ? "audit-filter active" : "audit-filter"}
                aria-pressed={filter === item.value}
                onClick={() => setFilter(item.value)}
              >
                {t(language, item.label)}
              </button>
            ))}
          </div>

          {loading && events.length === 0 ? (
            <section className="panel audit-empty-state"><p>{t(language, "loading")}</p></section>
          ) : visibleEvents.length === 0 ? (
            <section className="panel audit-empty-state">
              <div className="audit-empty-icon"><FileText size={20} /></div>
              <h2>{t(language, "auditNoEvents")}</h2>
              <p>{t(language, "auditNoEventsHelp")}</p>
            </section>
          ) : (
            <div className="audit-events-list">
              {visibleEvents.map((event) => {
                const category = categoryFor(event);
                const detailEntries = Object.entries(event.details ?? {})
                  .filter(([key]) => !/(secret|token|credential|password|authorization|webhook|url)/i.test(key))
                  .slice(0, 8);
                return (
                  <article className="audit-event-card" key={event.id}>
                    <div className="audit-event-icon"><FileText size={16} /></div>
                    <div className="audit-event-content">
                      <div className="audit-event-heading">
                        <strong>{actionLabel(event.action)}</strong>
                        <time dateTime={event.created_at}>{dateTime(event.created_at, language)}</time>
                      </div>
                      <div className="audit-event-meta">
                        <span>{t(language, categoryKey(category))}</span>
                        <span>{t(language, "auditResource")}: <b>{event.resource_type}</b></span>
                        {event.resource_id && <code>{event.resource_id.length > 14 ? event.resource_id.slice(0, 8) + "…" + event.resource_id.slice(-4) : event.resource_id}</code>}
                      </div>
                      {event.request_id && (
                        <div className="audit-request-id">{t(language, "auditRequestId")}: <code>{event.request_id}</code></div>
                      )}
                      {detailEntries.length > 0 && (
                        <details className="audit-event-details">
                          <summary>{t(language, "auditDetails")} ({detailEntries.length})</summary>
                          <dl>
                            {detailEntries.map(([key, value]) => (
                              <div key={key}><dt>{key.replace(/[_-]/g, " ")}</dt><dd>{displayValue(value)}</dd></div>
                            ))}
                          </dl>
                        </details>
                      )}
                    </div>
                  </article>
                );
              })}
            </div>
          )}
        </>
      )}
    </section>
  );
}

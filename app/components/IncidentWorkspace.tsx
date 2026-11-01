"use client";

import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import {
  Activity,
  AlertCircle,
  Check,
  Clock3,
  History,
  Plus,
  RefreshCw,
  ShieldCheck,
  X,
} from "lucide-react";
import {
  createWorkspaceIncident,
  getWorkspaceMembership,
  listIncidentTimeline,
  listWorkspaceIncidents,
  listWorkspaceServices,
  listWorkspaces,
  transitionWorkspaceIncident,
} from "../../lib/orbit/client";
import type {
  IncidentSeverity,
  IncidentStatus,
  IncidentSummary,
  IncidentTimelineEvent,
  ServiceSummary,
  WorkspaceMembershipSummary,
  WorkspaceSummary,
} from "../../lib/orbit/contracts";
import { formatLocale, t, type Language } from "../../lib/orbit/i18n";
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

type IncidentFilter = "all" | "active" | "resolved";

const transitions: Record<IncidentStatus, IncidentStatus[]> = {
  open: ["investigating", "monitoring", "resolved"],
  investigating: ["monitoring", "resolved"],
  monitoring: ["investigating", "resolved"],
  resolved: ["investigating"],
};

const severities: IncidentSeverity[] = ["critical", "high", "medium", "low"];

function dateTime(value: string, language: Language): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return t(language, "unknown");
  return new Intl.DateTimeFormat(formatLocale(language), {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function severityLabel(value: IncidentSeverity, language: Language): string {
  return t(language, value);
}

function statusLabel(value: IncidentStatus, language: Language): string {
  return t(language, value);
}

function eventLabel(event: IncidentTimelineEvent, language: Language): string {
  if (event.event_type === "incident.created") return t(language, "incidentCreatedEvent");
  if (event.event_type === "incident.reopened") return t(language, "incidentReopenedEvent");
  return t(language, "incidentStatusChangedEvent");
}

export default function IncidentWorkspace({ apiBaseUrl, language, sourceMode }: Props) {
  const [user, setUser] = useState<OrbitAuthUser | null>(() => getCurrentAuthUser());
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([]);
  const [workspaceId, setWorkspaceId] = useState("");
  const [membership, setMembership] = useState<WorkspaceMembershipSummary | null>(null);
  const [services, setServices] = useState<ServiceSummary[]>([]);
  const [incidents, setIncidents] = useState<IncidentSummary[]>([]);
  const [selectedIncidentId, setSelectedIncidentId] = useState("");
  const [timeline, setTimeline] = useState<IncidentTimelineEvent[]>([]);
  const [filter, setFilter] = useState<IncidentFilter>("active");
  const [loading, setLoading] = useState(false);
  const [timelineLoading, setTimelineLoading] = useState(false);
  const [timelineVersion, setTimelineVersion] = useState(0);
  const [saving, setSaving] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [serviceId, setServiceId] = useState("");
  const [severity, setSeverity] = useState<IncidentSeverity>("high");
  const [nextStatus, setNextStatus] = useState<IncidentStatus | "">("");
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => subscribeToAuth(() => {
    setUser(getCurrentAuthUser());
    setWorkspaces([]);
    setWorkspaceId("");
    setMembership(null);
    setIncidents([]);
    setServices([]);
    setTimeline([]);
  }), []);

  useEffect(() => {
    if (sourceMode !== "api" || !user) {
      setWorkspaces([]);
      setWorkspaceId("");
      setMembership(null);
      setServices([]);
      setIncidents([]);
      setSelectedIncidentId("");
      setTimeline([]);
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
          previous && items.some((workspace) => workspace.id === previous)
            ? previous
            : items[0]?.id ?? "",
        );
      })
      .catch(() => {
        if (current) setError(t(language, "incidentWorkspaceLoadFailed"));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [apiBaseUrl, language, sourceMode, user?.issuer, user?.sub]);

  useEffect(() => {
    if (!workspaceId || sourceMode !== "api" || !user) return;
    const controller = new AbortController();
    let current = true;
    setLoading(true);
    setError("");
    setSelectedIncidentId("");
    setTimeline([]);
    Promise.all([
      getWorkspaceMembership(workspaceId, controller.signal, apiBaseUrl),
      listWorkspaceServices(workspaceId, controller.signal, apiBaseUrl),
      listWorkspaceIncidents(workspaceId, undefined, controller.signal, apiBaseUrl),
    ])
      .then(([role, serviceItems, incidentItems]) => {
        if (!current) return;
        setMembership(role);
        setServices(serviceItems);
        setIncidents(incidentItems);
        setServiceId((previous) =>
          previous && serviceItems.some((service) => service.id === previous)
            ? previous
            : serviceItems[0]?.id ?? "",
        );
        setSelectedIncidentId(incidentItems[0]?.id ?? "");
      })
      .catch(() => {
        if (current) {
          setMembership(null);
          setServices([]);
          setIncidents([]);
          setError(t(language, "incidentWorkspaceLoadFailed"));
        }
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [apiBaseUrl, language, sourceMode, user?.issuer, user?.sub, workspaceId]);

  useEffect(() => {
    if (!workspaceId || !selectedIncidentId || sourceMode !== "api" || !user) {
      setTimeline([]);
      return;
    }
    const controller = new AbortController();
    let current = true;
    setTimelineLoading(true);
    listIncidentTimeline(workspaceId, selectedIncidentId, 50, controller.signal, apiBaseUrl)
      .then((items) => {
        if (current) setTimeline(items);
      })
      .catch(() => {
        if (current) setTimeline([]);
      })
      .finally(() => {
        if (current) setTimelineLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [apiBaseUrl, sourceMode, user?.issuer, user?.sub, workspaceId, selectedIncidentId, timelineVersion]);

  const selectedIncident = incidents.find((incident) => incident.id === selectedIncidentId) ?? null;
  const nextStatuses = selectedIncident ? transitions[selectedIncident.status] : [];
  const canManage = Boolean(
    isOidcConfigured() &&
    user &&
    membership &&
    ["owner", "administrator", "operator"].includes(membership.role),
  );

  const visibleIncidents = useMemo(() => {
    if (filter === "resolved") return incidents.filter((incident) => incident.status === "resolved");
    if (filter === "active") return incidents.filter((incident) => incident.status !== "resolved");
    return incidents;
  }, [filter, incidents]);

  useEffect(() => {
    if (!selectedIncident) {
      setNextStatus("");
      return;
    }
    setNextStatus(transitions[selectedIncident.status][0] ?? "");
    setComment("");
  }, [selectedIncident?.id, selectedIncident?.status]);

  const reloadWorkspace = async () => {
    if (!workspaceId) return;
    setLoading(true);
    try {
      const [serviceItems, incidentItems] = await Promise.all([
        listWorkspaceServices(workspaceId, undefined, apiBaseUrl),
        listWorkspaceIncidents(workspaceId, undefined, undefined, apiBaseUrl),
      ]);
      setServices(serviceItems);
      setIncidents(incidentItems);
      if (!selectedIncidentId || !incidentItems.some((incident) => incident.id === selectedIncidentId)) {
        setSelectedIncidentId(incidentItems[0]?.id ?? "");
      }
      setTimelineVersion((version) => version + 1);
    } finally {
      setLoading(false);
    }
  };

  const submitIncident = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!workspaceId || !canManage || !serviceId || !title.trim()) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const created = await createWorkspaceIncident(workspaceId, {
        title: title.trim(),
        description: description.trim(),
        service_id: serviceId,
        severity,
      }, apiBaseUrl);
      await reloadWorkspace();
      setSelectedIncidentId(created.id);
      setTitle("");
      setDescription("");
      setShowCreate(false);
      setNotice(t(language, "incidentCreatedNotice"));
    } catch {
      setError(t(language, "incidentMutationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const submitTransition = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!workspaceId || !selectedIncident || !nextStatus || !canManage) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const changed = await transitionWorkspaceIncident(workspaceId, selectedIncident.id, {
        status: nextStatus,
        comment: comment.trim(),
      }, apiBaseUrl);
      await reloadWorkspace();
      setSelectedIncidentId(changed.id);
      setNotice(t(language, "incidentUpdatedNotice"));
    } catch {
      setError(t(language, "incidentMutationFailed"));
    } finally {
      setSaving(false);
    }
  };

  if (sourceMode === "demo") {
    return (
      <section className="panel incident-workspace incident-demo-notice">
        <div className="incident-heading-icon"><ShieldCheck size={18} /></div>
        <div>
          <h2>{t(language, "incidentWorkspaceTitle")}</h2>
          <p>{t(language, "incidentDemoReadOnly")}</p>
        </div>
      </section>
    );
  }

  if (!user) {
    return (
      <section className="panel incident-workspace incident-auth-notice">
        <div className="incident-heading-icon"><ShieldCheck size={18} /></div>
        <div>
          <h2>{t(language, "incidentWorkspaceTitle")}</h2>
          <p>{t(language, isOidcConfigured() ? "incidentSignInRequired" : "incidentAuthNotConfigured")}</p>
        </div>
      </section>
    );
  }

  return (
    <section className="incident-workspace" aria-labelledby="incident-workspace-title">
      <div className="incident-workspace-toolbar">
        <div>
          <div className="incident-kicker"><Activity size={13} /> {t(language, "incidentOperations")}</div>
          <h2 id="incident-workspace-title">{t(language, "incidentWorkspaceTitle")}</h2>
          <p>{t(language, "incidentWorkspaceSubtitle")}</p>
        </div>
        <div className="incident-workspace-actions">
          <label className="incident-workspace-select">
            <span>{t(language, "incidentSelectWorkspace")}</span>
            <select value={workspaceId} onChange={(event) => setWorkspaceId(event.target.value)} disabled={loading || workspaces.length === 0}>
              {workspaces.length === 0 && <option value="">{t(language, "incidentNoWorkspaces")}</option>}
              {workspaces.map((workspace) => <option value={workspace.id} key={workspace.id}>{workspace.name}</option>)}
            </select>
          </label>
          <button className="button-secondary incident-refresh" type="button" onClick={() => { void reloadWorkspace().catch(() => setError(t(language, "incidentWorkspaceLoadFailed"))); }} disabled={loading || !workspaceId}>
            <RefreshCw size={14} /> {t(language, "refresh")}
          </button>
          {canManage && (
            <button className="button-primary" type="button" onClick={() => setShowCreate((value) => !value)} disabled={loading || services.length === 0}>
              {showCreate ? <X size={14} /> : <Plus size={14} />} {showCreate ? t(language, "cancel") : t(language, "incidentCreate")}
            </button>
          )}
        </div>
      </div>

      {!isOidcConfigured() && (
        <div className="incident-inline-notice"><ShieldCheck size={15} /> {t(language, "incidentAuthNotConfigured")}</div>
      )}
      {membership && isOidcConfigured() && !canManage && (
        <div className="incident-inline-notice"><ShieldCheck size={15} /> {t(language, "incidentRoleReadOnly")}</div>
      )}
      {error && <div className="incident-feedback error" role="alert"><AlertCircle size={15} /> {error}</div>}
      {notice && <div className="incident-feedback success" role="status"><Check size={15} /> {notice}</div>}

      {showCreate && canManage && (
        <form className="panel incident-create-panel" onSubmit={(event) => void submitIncident(event)}>
          <div className="incident-panel-heading">
            <div><h3>{t(language, "incidentCreateTitle")}</h3><p>{t(language, "incidentCreateSubtitle")}</p></div>
            <button className="icon-button" type="button" aria-label={t(language, "cancel")} onClick={() => setShowCreate(false)}><X size={16} /></button>
          </div>
          <div className="incident-form-grid">
            <label><span>{t(language, "incidentTitleLabel")}</span><input maxLength={160} required value={title} onChange={(event) => setTitle(event.target.value)} /></label>
            <label><span>{t(language, "incidentServiceLabel")}</span>
              <select value={serviceId} onChange={(event) => setServiceId(event.target.value)} required>
                {services.map((service) => <option key={service.id} value={service.id}>{service.name}</option>)}
              </select>
            </label>
            <label><span>{t(language, "incidentSeverityLabel")}</span>
              <select value={severity} onChange={(event) => setSeverity(event.target.value as IncidentSeverity)}>
                {severities.map((item) => <option key={item} value={item}>{severityLabel(item, language)}</option>)}
              </select>
            </label>
            <label className="incident-form-wide"><span>{t(language, "incidentDescriptionLabel")}</span><textarea maxLength={8000} rows={3} value={description} onChange={(event) => setDescription(event.target.value)} /></label>
          </div>
          <div className="incident-form-actions">
            <button className="button-secondary" type="button" onClick={() => setShowCreate(false)}>{t(language, "cancel")}</button>
            <button className="button-primary" type="submit" disabled={saving || !title.trim() || !serviceId}>{saving ? t(language, "saving") : t(language, "incidentCreate")}</button>
          </div>
        </form>
      )}

      <div className="incident-summary-row">
        <div className="incident-summary-stat"><span>{t(language, "incidentTotal")}</span><strong>{incidents.length}</strong></div>
        <div className="incident-summary-stat"><span>{t(language, "incidentActive")}</span><strong>{incidents.filter((incident) => incident.status !== "resolved").length}</strong></div>
        <div className="incident-summary-stat"><span>{t(language, "incidentResolved")}</span><strong>{incidents.filter((incident) => incident.status === "resolved").length}</strong></div>
        <div className="incident-role-indicator"><ShieldCheck size={14} /> {membership?.role ?? t(language, "unknown")}</div>
      </div>

      <div className="incident-operations-grid">
        <section className="panel incident-list-panel" aria-label={t(language, "incidents")}>
          <div className="incident-panel-heading">
            <div><h3>{t(language, "incidentRegister")}</h3><p>{t(language, "incidentRegisterSubtitle")}</p></div>
            <div className="incident-filter">
              <button type="button" className={filter === "active" ? "selected" : ""} onClick={() => setFilter("active")}>{t(language, "incidentActive")}</button>
              <button type="button" className={filter === "all" ? "selected" : ""} onClick={() => setFilter("all")}>{t(language, "all")}</button>
              <button type="button" className={filter === "resolved" ? "selected" : ""} onClick={() => setFilter("resolved")}>{t(language, "resolved")}</button>
            </div>
          </div>
          {loading ? <div className="incident-empty"><span className="telemetry-spinner" />{t(language, "incidentLoading")}</div> :
            visibleIncidents.length === 0 ? <div className="incident-empty">{t(language, "incidentEmpty")}</div> :
            <div className="incident-register">
              {visibleIncidents.map((incident) => (
                <button
                  type="button"
                  key={incident.id}
                  className={`incident-register-row ${selectedIncidentId === incident.id ? "selected" : ""}`}
                  onClick={() => setSelectedIncidentId(incident.id)}
                >
                  <span className={`incident-register-severity severity-${incident.severity}`} />
                  <span className="incident-register-copy">
                    <strong>{incident.title}</strong>
                    <small>{incident.service_name} · {dateTime(incident.updated_at, language)}</small>
                  </span>
                  <span className={`incident-status-pill status-${incident.status}`}>{statusLabel(incident.status, language)}</span>
                </button>
              ))}
            </div>
          }
        </section>

        <section className="panel incident-detail-panel" aria-label={t(language, "incidentDetails")}>
          {!selectedIncident ? (
            <div className="incident-empty incident-detail-empty"><History size={20} />{t(language, "incidentSelectToView")}</div>
          ) : (
            <>
              <div className="incident-panel-heading">
                <div><div className="incident-kicker"><Clock3 size={12} /> {t(language, "incidentDetails")}</div><h3>{selectedIncident.title}</h3><p>{selectedIncident.service_name} · {dateTime(selectedIncident.started_at, language)}</p></div>
                <span className={`incident-status-pill status-${selectedIncident.status}`}>{statusLabel(selectedIncident.status, language)}</span>
              </div>
              {selectedIncident.description && <p className="incident-description">{selectedIncident.description}</p>}
              {canManage && nextStatuses.length > 0 && (
                <form className="incident-transition-form" onSubmit={(event) => void submitTransition(event)}>
                  <label><span>{t(language, "incidentNextStatus")}</span>
                    <select value={nextStatus} onChange={(event) => setNextStatus(event.target.value as IncidentStatus)}>
                      {nextStatuses.map((status) => <option key={status} value={status}>{statusLabel(status, language)}</option>)}
                    </select>
                  </label>
                  <label><span>{t(language, "incidentStatusComment")}</span><textarea rows={2} maxLength={4000} value={comment} onChange={(event) => setComment(event.target.value)} /></label>
                  <button className="button-primary" type="submit" disabled={saving || !nextStatus}>{saving ? t(language, "saving") : t(language, "incidentApplyStatus")}</button>
                </form>
              )}
              <div className="incident-timeline-heading"><History size={15} /><h4>{t(language, "incidentTimeline")}</h4></div>
              {timelineLoading ? <div className="incident-empty">{t(language, "incidentTimelineLoading")}</div> :
                timeline.length === 0 ? <div className="incident-empty">{t(language, "incidentTimelineEmpty")}</div> :
                <ol className="incident-timeline">
                  {timeline.map((event) => (
                    <li key={event.id}>
                      <span className="incident-timeline-marker" />
                      <div className="incident-timeline-copy">
                        <strong>{eventLabel(event, language)}</strong>
                        {event.from_status && event.to_status && <span>{statusLabel(event.from_status, language)} → {statusLabel(event.to_status, language)}</span>}
                        {event.comment && <p>{event.comment}</p>}
                        <time dateTime={event.created_at}>{dateTime(event.created_at, language)}</time>
                      </div>
                    </li>
                  ))}
                </ol>
              }
            </>
          )}
        </section>
      </div>
    </section>
  );
}

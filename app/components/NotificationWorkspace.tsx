"use client";

import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import {
  Activity, BellRing, Check, Clock3, History, Pause, Play, Plus, RefreshCw, Send, ShieldCheck, X,
} from "lucide-react";
import {
  createWorkspaceAlertNotificationChannel,
  getWorkspaceMembership,
  listWorkspaceAlertNotificationAttempts,
  listWorkspaceAlertNotificationChannels,
  listWorkspaceAlertNotificationDeliveries,
  listWorkspaces,
  setWorkspaceAlertNotificationChannelEnabled,
} from "../../lib/orbit/client";
import type {
  AlertNotificationDeliveryStatus,
  WorkspaceAlertNotificationAttempt,
  WorkspaceAlertNotificationChannel,
  WorkspaceAlertNotificationDelivery,
  WorkspaceMembershipSummary,
  WorkspaceSummary,
} from "../../lib/orbit/contracts";
import { formatLocale, t, type Language, type TranslationKey } from "../../lib/orbit/i18n";
import {
  getCurrentAuthUser, isOidcConfigured, subscribeToAuth, type OrbitAuthUser,
} from "../../lib/orbit/oidc";

type Props = { apiBaseUrl: string; language: Language; sourceMode: "demo" | "api" };
type DeliveryFilter = "all" | AlertNotificationDeliveryStatus;

const deliveryFilters: { value: DeliveryFilter; label: TranslationKey }[] = [
  { value: "all", label: "notificationFilterAll" },
  { value: "pending", label: "notificationFilterPending" },
  { value: "processing", label: "notificationFilterProcessing" },
  { value: "delivered", label: "notificationFilterDelivered" },
  { value: "dead", label: "notificationFilterDead" },
];

function dateTime(value: string | null | undefined, language: Language): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat(formatLocale(language), { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function number(value: number, language: Language, maximumFractionDigits = 2): string {
  return new Intl.NumberFormat(formatLocale(language), { maximumFractionDigits }).format(value);
}

function deliveryStatusKey(status: AlertNotificationDeliveryStatus): TranslationKey {
  switch (status) {
    case "pending": return "notificationFilterPending";
    case "processing": return "notificationFilterProcessing";
    case "delivered": return "notificationFilterDelivered";
    case "dead": return "notificationFilterDead";
  }
}

function attemptOutcomeKey(outcome: WorkspaceAlertNotificationAttempt["outcome"]): TranslationKey {
  switch (outcome) {
    case "delivered": return "notificationAttemptSuccess";
    case "retry_scheduled": return "notificationAttemptRetry";
    case "dead": return "notificationAttemptDead";
  }
}

function eventTypeKey(type: WorkspaceAlertNotificationDelivery["event_type"]): TranslationKey {
  return type === "alert.firing" ? "notificationEventFiring" : "notificationEventResolved";
}

export default function NotificationWorkspace({ apiBaseUrl, language, sourceMode }: Props) {
  const [user, setUser] = useState<OrbitAuthUser | null>(() => getCurrentAuthUser());
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([]);
  const [workspaceId, setWorkspaceId] = useState("");
  const [membership, setMembership] = useState<WorkspaceMembershipSummary | null>(null);
  const [channels, setChannels] = useState<WorkspaceAlertNotificationChannel[]>([]);
  const [deliveries, setDeliveries] = useState<WorkspaceAlertNotificationDelivery[]>([]);
  const [filter, setFilter] = useState<DeliveryFilter>("all");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [channelName, setChannelName] = useState("");
  const [secretRef, setSecretRef] = useState("");
  const [expandedDeliveryId, setExpandedDeliveryId] = useState("");
  const [attempts, setAttempts] = useState<Record<string, WorkspaceAlertNotificationAttempt[]>>({});
  const [attemptsLoadingId, setAttemptsLoadingId] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [reloadVersion, setReloadVersion] = useState(0);

  useEffect(() => subscribeToAuth(() => {
    setUser(getCurrentAuthUser());
    setWorkspaces([]);
    setWorkspaceId("");
    setMembership(null);
    setChannels([]);
    setDeliveries([]);
    setAttempts({});
    setExpandedDeliveryId("");
  }), []);

  useEffect(() => {
    if (sourceMode !== "api" || !user) {
      setWorkspaces([]);
      setWorkspaceId("");
      setMembership(null);
      setChannels([]);
      setDeliveries([]);
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
      .catch(() => { if (current) setError(t(language, "notificationLoadFailed")); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; controller.abort(); };
  }, [apiBaseUrl, language, sourceMode, user?.sub]);

  useEffect(() => {
    setAttempts({});
    setExpandedDeliveryId("");
    setAttemptsLoadingId("");
  }, [workspaceId]);

  useEffect(() => {
    if (!workspaceId || sourceMode !== "api" || !user) return;
    const controller = new AbortController();
    let current = true;
    setLoading(true);
    setError("");
    Promise.all([
      getWorkspaceMembership(workspaceId, controller.signal, apiBaseUrl),
      listWorkspaceAlertNotificationChannels(workspaceId, controller.signal, apiBaseUrl),
      listWorkspaceAlertNotificationDeliveries(workspaceId, undefined, controller.signal, apiBaseUrl),
    ])
      .then(([role, channelItems, deliveryItems]) => {
        if (!current) return;
        setMembership(role);
        setChannels(channelItems);
        setDeliveries(deliveryItems);
      })
      .catch(() => {
        if (current) {
          setMembership(null);
          setChannels([]);
          setDeliveries([]);
          setError(t(language, "notificationLoadFailed"));
        }
      })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; controller.abort(); };
  }, [apiBaseUrl, language, sourceMode, user?.sub, workspaceId, reloadVersion]);

  useEffect(() => {
    if (sourceMode !== "api" || !user || !workspaceId) return;
    const timer = window.setInterval(() => setReloadVersion((version) => version + 1), 30_000);
    return () => window.clearInterval(timer);
  }, [sourceMode, user?.sub, workspaceId]);

  const canManage = Boolean(
    isOidcConfigured() && user && membership &&
    ["owner", "administrator", "operator"].includes(membership.role),
  );
  const visibleDeliveries = useMemo(
    () => filter === "all" ? deliveries : deliveries.filter((item) => item.status === filter),
    [deliveries, filter],
  );
  const channelEnabledCount = useMemo(() => channels.filter((channel) => channel.enabled).length, [channels]);
  const pendingCount = useMemo(
    () => deliveries.filter((item) => item.status === "pending" || item.status === "processing").length,
    [deliveries],
  );
  const deadCount = useMemo(() => deliveries.filter((item) => item.status === "dead").length, [deliveries]);

  const closeForm = () => { setShowForm(false); setChannelName(""); setSecretRef(""); };

  const submitChannel = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!workspaceId || !canManage || !channelName.trim() || !secretRef.trim()) return;
    setSaving(true); setError(""); setNotice("");
    try {
      await createWorkspaceAlertNotificationChannel(workspaceId, {
        name: channelName.trim(), secret_ref: secretRef.trim(),
      }, apiBaseUrl);
      closeForm();
      setNotice(t(language, "notificationCreated"));
      setReloadVersion((version) => version + 1);
    } catch {
      setError(t(language, "notificationMutationFailed"));
    } finally { setSaving(false); }
  };

  const toggleChannel = async (channel: WorkspaceAlertNotificationChannel) => {
    if (!workspaceId || !canManage) return;
    setSaving(true); setError(""); setNotice("");
    try {
      await setWorkspaceAlertNotificationChannelEnabled(workspaceId, channel.id, !channel.enabled, apiBaseUrl);
      setNotice(t(language, channel.enabled ? "notificationChannelDisabled" : "notificationChannelEnabled"));
      setReloadVersion((version) => version + 1);
    } catch {
      setError(t(language, "notificationMutationFailed"));
    } finally { setSaving(false); }
  };

  const toggleAttempts = async (delivery: WorkspaceAlertNotificationDelivery) => {
    if (expandedDeliveryId === delivery.id) { setExpandedDeliveryId(""); return; }
    setExpandedDeliveryId(delivery.id);
    if (attempts[delivery.id]) return;
    setAttemptsLoadingId(delivery.id);
    try {
      const items = await listWorkspaceAlertNotificationAttempts(workspaceId, delivery.id, undefined, apiBaseUrl);
      setAttempts((previous) => ({ ...previous, [delivery.id]: items }));
    } catch {
      setError(t(language, "notificationLoadFailed"));
    } finally { setAttemptsLoadingId(""); }
  };

  if (sourceMode === "demo") {
    return (
      <section className="notification-workspace">
        <div className="panel notification-notice">
          <span className="notification-heading-icon"><ShieldCheck size={18} /></span>
          <div><h2>{t(language, "notificationWorkspaceTitle")}</h2><p>{t(language, "notificationDemoReadOnly")}</p></div>
        </div>
      </section>
    );
  }

  if (!user) {
    return (
      <section className="notification-workspace">
        <div className="panel notification-notice">
          <span className="notification-heading-icon"><ShieldCheck size={18} /></span>
          <div>
            <h2>{t(language, "notificationWorkspaceTitle")}</h2>
            <p>{isOidcConfigured() ? t(language, "notificationSignInRequired") : t(language, "notificationAuthNotConfigured")}</p>
          </div>
        </div>
      </section>
    );
  }

  return (
    <section className="notification-workspace">
      <div className="notification-toolbar">
        <div>
          <div className="incident-kicker"><BellRing size={12} /> {t(language, "notificationOperations")}</div>
          <h2>{t(language, "notificationWorkspaceTitle")}</h2>
          <p>{t(language, "notificationWorkspaceSubtitle")}</p>
        </div>
        <div className="notification-toolbar-actions">
          <label className="notification-workspace-select">
            <span>{t(language, "incidentSelectWorkspace")}</span>
            <select value={workspaceId} onChange={(event) => setWorkspaceId(event.target.value)}>
              {workspaces.map((workspace) => <option key={workspace.id} value={workspace.id}>{workspace.name}</option>)}
            </select>
          </label>
          <button type="button" className="button-secondary notification-refresh"
            onClick={() => setReloadVersion((version) => version + 1)} disabled={loading}>
            <RefreshCw size={14} /> {t(language, "retry")}
          </button>
          {canManage && (
            <button type="button" className="button-primary" onClick={() => setShowForm(true)} disabled={saving}>
              <Plus size={14} /> {t(language, "notificationChannelCreate")}
            </button>
          )}
        </div>
      </div>

      {error && <div className="incident-feedback error" role="alert">{error}</div>}
      {notice && <div className="incident-feedback success" role="status"><Check size={13} /> {notice}</div>}
      {membership && !canManage && (
        <div className="incident-inline-notice"><ShieldCheck size={14} /> {t(language, "notificationRoleReadOnly")}</div>
      )}

      {!workspaces.length && !loading ? (
        <div className="panel notification-empty">{t(language, "incidentNoWorkspaces")}</div>
      ) : (
        <>
          <div className="notification-summary">
            <div className="notification-summary-card"><span>{t(language, "notificationChannelTitle")}</span><strong>{number(channels.length, language, 0)}</strong></div>
            <div className="notification-summary-card"><span>{t(language, "notificationEnabled")}</span><strong>{number(channelEnabledCount, language, 0)}</strong></div>
            <div className="notification-summary-card"><span>{t(language, "notificationFilterPending")}</span><strong>{number(pendingCount, language, 0)}</strong></div>
            <div className="notification-summary-card"><span>{t(language, "notificationFilterDead")}</span><strong className="notification-count-dead">{number(deadCount, language, 0)}</strong></div>
          </div>

          {showForm && (
            <section className="panel notification-form-panel">
              <div className="notification-panel-heading">
                <div><h3>{t(language, "notificationChannelCreate")}</h3><p>{t(language, "notificationSecretRefHelp")}</p></div>
                <button type="button" className="icon-button" onClick={closeForm} aria-label={t(language, "cancel")}><X size={15} /></button>
              </div>
              <form className="notification-form-grid" onSubmit={(event) => void submitChannel(event)}>
                <label>
                  <span>{t(language, "notificationChannelName")}</span>
                  <input required maxLength={80} value={channelName}
                    onChange={(event) => setChannelName(event.target.value)} autoComplete="off" />
                </label>
                <label>
                  <span>{t(language, "notificationSecretRef")}</span>
                  <input required minLength={1} maxLength={64} pattern="[A-Z][A-Z0-9_]{0,63}"
                    placeholder="OPS_PRIMARY" value={secretRef}
                    onChange={(event) => setSecretRef(event.target.value.toUpperCase())}
                    autoComplete="off" spellCheck={false} />
                </label>
                <div className="notification-form-actions">
                  <button type="button" className="button-secondary" onClick={closeForm}>{t(language, "cancel")}</button>
                  <button type="submit" className="button-primary" disabled={saving}>
                    {saving ? t(language, "saving") : t(language, "notificationCreate")}
                  </button>
                </div>
              </form>
            </section>
          )}

          <div className="notification-grid">
            <section className="panel notification-panel">
              <div className="notification-panel-heading">
                <div><h3>{t(language, "notificationChannelTitle")}</h3><p>{t(language, "notificationChannelSubtitle")}</p></div>
                {loading && <span className="notification-loading"><span className="telemetry-spinner" /></span>}
              </div>
              {channels.length === 0 && !loading ? (
                <div className="notification-empty">{t(language, "notificationNoChannels")}</div>
              ) : (
                <div className="notification-channel-list">
                  {channels.map((channel) => (
                    <article className="notification-channel-card" key={channel.id}>
                      <div className="notification-channel-copy">
                        <div className="notification-channel-title-row">
                          <strong>{channel.name}</strong>
                          <span className={"notification-channel-pill " + (channel.enabled ? "is-enabled" : "is-disabled")}>
                            {t(language, channel.enabled ? "notificationEnabled" : "notificationDisabled")}
                          </span>
                        </div>
                        <p><Send size={11} /> {channel.adapter.toUpperCase()} · {channel.secret_configured
                          ? t(language, "notificationSecretConfigured") : t(language, "notificationSecretMissing")}</p>
                        <small>{dateTime(channel.created_at, language)}</small>
                      </div>
                      {canManage && (
                        <button type="button" className="notification-icon-action"
                          title={t(language, channel.enabled ? "notificationChannelDisable" : "notificationChannelEnable")}
                          aria-label={t(language, channel.enabled ? "notificationChannelDisable" : "notificationChannelEnable")}
                          onClick={() => void toggleChannel(channel)} disabled={saving}>
                          {channel.enabled ? <Pause size={14} /> : <Play size={14} />}
                        </button>
                      )}
                    </article>
                  ))}
                </div>
              )}
            </section>

            <section className="panel notification-panel">
              <div className="notification-panel-heading">
                <div><h3>{t(language, "notificationDeliveriesTitle")}</h3><p>{t(language, "notificationDeliveriesSubtitle")}</p></div>
                <div className="incident-filter notification-filter">
                  {deliveryFilters.map((item) => (
                    <button key={item.value} type="button" className={filter === item.value ? "selected" : ""}
                      onClick={() => setFilter(item.value)}>{t(language, item.label)}</button>
                  ))}
                </div>
              </div>
              {visibleDeliveries.length === 0 && !loading ? (
                <div className="notification-empty">{t(language, "notificationNoDeliveries")}</div>
              ) : (
                <div className="notification-delivery-list">
                  {visibleDeliveries.map((delivery) => (
                    <article className="notification-delivery-card" key={delivery.id}>
                      <div className="notification-delivery-top">
                        <span className={"notification-status-pill status-" + delivery.status}>
                          {t(language, deliveryStatusKey(delivery.status))}
                        </span>
                        <span className={"alert-severity severity-" + delivery.severity}>{t(language, delivery.severity)}</span>
                      </div>
                      <div className="notification-delivery-title">
                        <strong>{delivery.rule_name}</strong><span>{t(language, eventTypeKey(delivery.event_type))}</span>
                      </div>
                      <p>{delivery.channel_name} · {delivery.service_name} · {delivery.metric}</p>
                      <div className="notification-delivery-values">
                        <span>{t(language, "alertCurrentValue")} <strong>{number(delivery.value, language, 3)} req/s</strong></span>
                        <span>{t(language, "alertThreshold")} <strong>{delivery.operator} {number(delivery.threshold, language, 3)} req/s</strong></span>
                        <span>{t(language, "notificationAttempt")} <strong>{number(delivery.attempts, language, 0)}</strong></span>
                      </div>
                      <div className="notification-delivery-meta">
                        <small><Clock3 size={11} /> {t(language, "notificationOccurredAt")} {dateTime(delivery.occurred_at, language)}</small>
                        {delivery.last_attempt_at && <small><Activity size={11} /> {t(language, "notificationLastAttempt")} {dateTime(delivery.last_attempt_at, language)}</small>}
                        {delivery.next_attempt_at && delivery.status === "pending" && <small><Clock3 size={11} /> {t(language, "notificationNextAttempt")} {dateTime(delivery.next_attempt_at, language)}</small>}
                        {delivery.last_error && <small className="notification-error-code">{t(language, "notificationLastError")}: {delivery.last_error}</small>}
                      </div>
                      <div className="notification-delivery-actions">
                        <button type="button" className="button-secondary"
                          onClick={() => void toggleAttempts(delivery)} disabled={attemptsLoadingId === delivery.id}>
                          <History size={13} />
                          {attemptsLoadingId === delivery.id ? t(language, "notificationAttemptsLoading") : t(language, "notificationAttempts")}
                        </button>
                      </div>
                      {expandedDeliveryId === delivery.id && (
                        <div className="notification-attempt-list">
                          {!attempts[delivery.id] || attempts[delivery.id].length === 0 ? (
                            <div className="notification-empty">
                              {attemptsLoadingId === delivery.id ? t(language, "notificationAttemptsLoading") : t(language, "notificationEmptyAttempts")}
                            </div>
                          ) : attempts[delivery.id].map((attempt) => (
                            <div className="notification-attempt-row" key={attempt.id}>
                              <div>
                                <strong>{t(language, "notificationAttempt")} {number(attempt.attempt_number, language, 0)}</strong>
                                <span className={"notification-attempt-outcome outcome-" + attempt.outcome}>
                                  {t(language, attemptOutcomeKey(attempt.outcome))}
                                </span>
                              </div>
                              <small>{dateTime(attempt.attempted_at, language)}</small>
                              {attempt.http_status && <small>HTTP {attempt.http_status}</small>}
                              {attempt.error_code && <small className="notification-error-code">{attempt.error_code}</small>}
                            </div>
                          ))}
                        </div>
                      )}
                    </article>
                  ))}
                </div>
              )}
            </section>
          </div>
          <p className="notification-footnote"><Activity size={12} /> {t(language, "notificationAutoRefresh")}</p>
        </>
      )}
    </section>
  );
}

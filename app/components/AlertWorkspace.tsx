"use client";

import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import {
  Activity,
  Bell,
  Check,
  Clock3,
  Pencil,
  Pause,
  Play,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  X,
} from "lucide-react";
import {
  createWorkspaceAlertRule,
  deleteWorkspaceAlertRule,
  getWorkspaceMembership,
  listWorkspaceAlertRules,
  listWorkspaceAlerts,
  listWorkspaceServices,
  listWorkspaces,
  setWorkspaceAlertRuleEnabled,
  updateWorkspaceAlertRule,
  type WorkspaceAlertRuleInput,
} from "../../lib/orbit/client";
import type {
  AlertInstanceStatus,
  AlertRuleOperator,
  ServiceSummary,
  WorkspaceAlertInstance,
  WorkspaceAlertRule,
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

type AlertFilter = "active" | "all" | "history";
type RuleDraft = {
  name: string;
  description: string;
  service_name: string;
  operator: AlertRuleOperator;
  threshold: string;
  window_seconds: number;
  pending_seconds: string;
  severity: WorkspaceAlertRule["severity"];
};

const emptyDraft = (): RuleDraft => ({
  name: "",
  description: "",
  service_name: "",
  operator: "gt",
  threshold: "250",
  window_seconds: 60,
  pending_seconds: "60",
  severity: "high",
});

const operators: { value: AlertRuleOperator; label: string }[] = [
  { value: "gt", label: ">" },
  { value: "gte", label: "≥" },
  { value: "lt", label: "<" },
  { value: "lte", label: "≤" },
];

function dateTime(value: string | null | undefined, language: Language): string {
  if (!value) return t(language, "unknown");
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return t(language, "unknown");
  return new Intl.DateTimeFormat(formatLocale(language), {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function number(value: number, language: Language, maximumFractionDigits = 2): string {
  return new Intl.NumberFormat(formatLocale(language), {
    maximumFractionDigits,
  }).format(value);
}

function statusKey(status: AlertInstanceStatus): TranslationKey {
  switch (status) {
    case "pending": return "alertPending";
    case "firing": return "alertFiring";
    case "resolved": return "alertResolved";
    case "cancelled": return "alertCancelled";
  }
}

export default function AlertWorkspace({ apiBaseUrl, language, sourceMode }: Props) {
  const [user, setUser] = useState<OrbitAuthUser | null>(() => getCurrentAuthUser());
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([]);
  const [workspaceId, setWorkspaceId] = useState("");
  const [membership, setMembership] = useState<WorkspaceMembershipSummary | null>(null);
  const [services, setServices] = useState<ServiceSummary[]>([]);
  const [rules, setRules] = useState<WorkspaceAlertRule[]>([]);
  const [instances, setInstances] = useState<WorkspaceAlertInstance[]>([]);
  const [filter, setFilter] = useState<AlertFilter>("active");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [editingRule, setEditingRule] = useState<WorkspaceAlertRule | null>(null);
  const [draft, setDraft] = useState<RuleDraft>(emptyDraft);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [reloadVersion, setReloadVersion] = useState(0);

  useEffect(() => subscribeToAuth(() => {
    setUser(getCurrentAuthUser());
    setWorkspaces([]);
    setWorkspaceId("");
    setMembership(null);
    setServices([]);
    setRules([]);
    setInstances([]);
  }), []);

  useEffect(() => {
    if (sourceMode !== "api" || !user) {
      setWorkspaces([]);
      setWorkspaceId("");
      setMembership(null);
      setServices([]);
      setRules([]);
      setInstances([]);
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
        if (current) setError(t(language, "alertLoadFailed"));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [apiBaseUrl, language, sourceMode, user?.sub]);

  useEffect(() => {
    if (!workspaceId || sourceMode !== "api" || !user) return;
    const controller = new AbortController();
    let current = true;
    setLoading(true);
    setError("");
    Promise.all([
      getWorkspaceMembership(workspaceId, controller.signal, apiBaseUrl),
      listWorkspaceServices(workspaceId, controller.signal, apiBaseUrl),
      listWorkspaceAlertRules(workspaceId, controller.signal, apiBaseUrl),
      listWorkspaceAlerts(workspaceId, undefined, controller.signal, apiBaseUrl),
    ])
      .then(([role, serviceItems, ruleItems, alertItems]) => {
        if (!current) return;
        setMembership(role);
        setServices(serviceItems);
        setRules(ruleItems);
        setInstances(alertItems);
      })
      .catch(() => {
        if (current) {
          setMembership(null);
          setServices([]);
          setRules([]);
          setInstances([]);
          setError(t(language, "alertLoadFailed"));
        }
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [apiBaseUrl, language, sourceMode, user?.sub, workspaceId, reloadVersion]);

  useEffect(() => {
    if (sourceMode !== "api" || !user || !workspaceId) return;
    const timer = window.setInterval(() => setReloadVersion((version) => version + 1), 30_000);
    return () => window.clearInterval(timer);
  }, [sourceMode, user?.sub, workspaceId]);

  const canManage = Boolean(
    isOidcConfigured() &&
    user &&
    membership &&
    ["owner", "administrator", "operator"].includes(membership.role),
  );

  const visibleInstances = useMemo(() => {
    if (filter === "active") {
      return instances.filter((item) => item.status === "pending" || item.status === "firing");
    }
    if (filter === "history") {
      return instances.filter((item) => item.status === "resolved" || item.status === "cancelled");
    }
    return instances;
  }, [filter, instances]);

  const firingCount = instances.filter((item) => item.status === "firing").length;
  const pendingCount = instances.filter((item) => item.status === "pending").length;
  const enabledCount = rules.filter((rule) => rule.enabled).length;

  const startCreate = () => {
    setEditingRule(null);
    setDraft(emptyDraft());
    setShowForm(true);
    setError("");
    setNotice("");
  };

  const startEdit = (rule: WorkspaceAlertRule) => {
    setEditingRule(rule);
    setDraft({
      name: rule.name,
      description: rule.description ?? "",
      service_name: rule.service_name ?? "",
      operator: rule.operator,
      threshold: String(rule.threshold),
      window_seconds: rule.window_seconds,
      pending_seconds: String(rule.pending_seconds),
      severity: rule.severity,
    });
    setShowForm(true);
    setError("");
    setNotice("");
  };

  const closeForm = () => {
    setShowForm(false);
    setEditingRule(null);
    setDraft(emptyDraft());
  };

  const submitRule = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!workspaceId || !canManage || !draft.name.trim()) return;
    const threshold = Number(draft.threshold);
    const pendingSeconds = Number(draft.pending_seconds);
    if (!Number.isFinite(threshold) || threshold <= 0 || threshold > 1_000_000_000) {
      setError(t(language, "alertInvalidThreshold"));
      return;
    }
    if (!Number.isInteger(pendingSeconds) || pendingSeconds < 0 || pendingSeconds > 3600) {
      setError(t(language, "alertInvalidPending"));
      return;
    }

    const input: WorkspaceAlertRuleInput = {
      name: draft.name.trim(),
      description: draft.description.trim(),
      service_name: draft.service_name,
      metric: "request_rate",
      operator: draft.operator,
      threshold,
      window_seconds: draft.window_seconds,
      pending_seconds: pendingSeconds,
      severity: draft.severity,
    };

    setSaving(true);
    setError("");
    setNotice("");
    try {
      if (editingRule) {
        await updateWorkspaceAlertRule(workspaceId, editingRule.id, input, apiBaseUrl);
        setNotice(t(language, "alertRuleUpdated"));
      } else {
        await createWorkspaceAlertRule(workspaceId, input, apiBaseUrl);
        setNotice(t(language, "alertRuleCreated"));
      }
      closeForm();
      setReloadVersion((version) => version + 1);
    } catch {
      setError(t(language, "alertMutationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const toggleRule = async (rule: WorkspaceAlertRule) => {
    if (!workspaceId || !canManage) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      await setWorkspaceAlertRuleEnabled(workspaceId, rule.id, !rule.enabled, apiBaseUrl);
      setNotice(rule.enabled ? t(language, "alertRuleDisabledNotice") : t(language, "alertRuleEnabledNotice"));
      setReloadVersion((version) => version + 1);
    } catch {
      setError(t(language, "alertMutationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const removeRule = async (rule: WorkspaceAlertRule) => {
    if (!workspaceId || !canManage || !window.confirm(t(language, "alertDeleteConfirm"))) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      await deleteWorkspaceAlertRule(workspaceId, rule.id, apiBaseUrl);
      setNotice(t(language, "alertRuleDeleted"));
      setReloadVersion((version) => version + 1);
    } catch {
      setError(t(language, "alertMutationFailed"));
    } finally {
      setSaving(false);
    }
  };

  if (sourceMode === "demo") {
    return (
      <section className="alert-workspace">
        <div className="panel alert-notice">
          <span className="alert-heading-icon"><ShieldCheck size={18} /></span>
          <div><h2>{t(language, "alertWorkspaceTitle")}</h2><p>{t(language, "alertDemoReadOnly")}</p></div>
        </div>
      </section>
    );
  }

  if (!user) {
    return (
      <section className="alert-workspace">
        <div className="panel alert-notice">
          <span className="alert-heading-icon"><ShieldCheck size={18} /></span>
          <div><h2>{t(language, "alertWorkspaceTitle")}</h2><p>{isOidcConfigured() ? t(language, "alertSignInRequired") : t(language, "alertAuthNotConfigured")}</p></div>
        </div>
      </section>
    );
  }

  return (
    <section className="alert-workspace">
      <div className="alert-toolbar">
        <div>
          <div className="incident-kicker"><Bell size={12} /> {t(language, "alertOperations")}</div>
          <h2>{t(language, "alertWorkspaceTitle")}</h2>
          <p>{t(language, "alertWorkspaceSubtitle")}</p>
        </div>
        <div className="alert-toolbar-actions">
          <label className="alert-workspace-select">
            <span>{t(language, "incidentSelectWorkspace")}</span>
            <select value={workspaceId} onChange={(event) => setWorkspaceId(event.target.value)}>
              {workspaces.map((workspace) => <option key={workspace.id} value={workspace.id}>{workspace.name}</option>)}
            </select>
          </label>
          <button type="button" className="button-secondary alert-refresh" onClick={() => setReloadVersion((version) => version + 1)} disabled={loading}>
            <RefreshCw size={14} /> {t(language, "retry")}
          </button>
          {canManage && <button type="button" className="button-primary" onClick={startCreate} disabled={saving}><Plus size={14} /> {t(language, "alertRuleCreate")}</button>}
        </div>
      </div>

      {error && <div className="incident-feedback error" role="alert">{error}</div>}
      {notice && <div className="incident-feedback success" role="status"><Check size={13} /> {notice}</div>}
      {membership && !canManage && <div className="incident-inline-notice"><ShieldCheck size={14} /> {t(language, "alertRoleReadOnly")}</div>}

      {!workspaces.length && !loading ? (
        <div className="panel alert-empty">{t(language, "incidentNoWorkspaces")}</div>
      ) : (
        <>
          <div className="alert-summary">
            <div className="alert-summary-card"><span>{t(language, "alertRulesTitle")}</span><strong>{number(rules.length, language, 0)}</strong></div>
            <div className="alert-summary-card"><span>{t(language, "alertEnabledRules")}</span><strong>{number(enabledCount, language, 0)}</strong></div>
            <div className="alert-summary-card"><span>{t(language, "alertFiring")}</span><strong className="alert-count-firing">{number(firingCount, language, 0)}</strong></div>
            <div className="alert-summary-card"><span>{t(language, "alertPending")}</span><strong>{number(pendingCount, language, 0)}</strong></div>
          </div>

          {showForm && (
            <section className="panel alert-form-panel">
              <div className="alert-panel-heading">
                <div><h3>{editingRule ? t(language, "alertRuleEdit") : t(language, "alertRuleCreate")}</h3><p>{t(language, "alertOnlyRequestRate")}</p></div>
                <button type="button" className="icon-button" onClick={closeForm} aria-label={t(language, "cancel")}><X size={15} /></button>
              </div>
              <form className="alert-form-grid" onSubmit={(event) => void submitRule(event)}>
                <label className="alert-form-wide"><span>{t(language, "alertRuleName")}</span><input required maxLength={120} value={draft.name} onChange={(event) => setDraft((value) => ({ ...value, name: event.target.value }))} /></label>
                <label className="alert-form-wide"><span>{t(language, "alertRuleDescription")}</span><textarea rows={2} maxLength={2000} value={draft.description} onChange={(event) => setDraft((value) => ({ ...value, description: event.target.value }))} /></label>
                <label><span>{t(language, "alertRuleService")}</span>
                  <select value={draft.service_name} onChange={(event) => setDraft((value) => ({ ...value, service_name: event.target.value }))}>
                    <option value="">{t(language, "alertAllServices")}</option>
                    {services.map((service) => <option key={service.id} value={service.name}>{service.name}</option>)}
                  </select>
                </label>
                <label><span>{t(language, "alertRuleOperator")}</span>
                  <select value={draft.operator} onChange={(event) => setDraft((value) => ({ ...value, operator: event.target.value as AlertRuleOperator }))}>
                    {operators.map((operator) => <option key={operator.value} value={operator.value}>{operator.label}</option>)}
                  </select>
                </label>
                <label><span>{t(language, "alertRuleThreshold")}</span><input type="number" required min="0.000001" max="1000000000" step="any" value={draft.threshold} onChange={(event) => setDraft((value) => ({ ...value, threshold: event.target.value }))} /></label>
                <label><span>{t(language, "alertRuleWindow")}</span>
                  <select value={draft.window_seconds} onChange={(event) => setDraft((value) => ({ ...value, window_seconds: Number(event.target.value) }))}>
                    <option value={60}>60s</option><option value={300}>300s</option><option value={900}>900s</option>
                  </select>
                </label>
                <label><span>{t(language, "alertRulePending")}</span><input type="number" required min="0" max="3600" step="1" value={draft.pending_seconds} onChange={(event) => setDraft((value) => ({ ...value, pending_seconds: event.target.value }))} /></label>
                <label><span>{t(language, "incidentSeverityLabel")}</span>
                  <select value={draft.severity} onChange={(event) => setDraft((value) => ({ ...value, severity: event.target.value as WorkspaceAlertRule["severity"] }))}>
                    {(["critical", "high", "medium", "low"] as const).map((severity) => <option key={severity} value={severity}>{t(language, severity)}</option>)}
                  </select>
                </label>
                <div className="alert-form-actions">
                  <button type="button" className="button-secondary" onClick={closeForm}>{t(language, "cancel")}</button>
                  <button type="submit" className="button-primary" disabled={saving}>{saving ? t(language, "saving") : editingRule ? t(language, "alertRuleUpdate") : t(language, "alertRuleSave")}</button>
                </div>
              </form>
            </section>
          )}

          <div className="alert-grid">
            <section className="panel alert-panel">
              <div className="alert-panel-heading">
                <div><h3>{t(language, "alertRulesTitle")}</h3><p>{t(language, "alertRulesSubtitle")}</p></div>
                {loading && <span className="alert-loading"><span className="telemetry-spinner" /></span>}
              </div>
              {rules.length === 0 && !loading ? <div className="alert-empty">{t(language, "alertNoRules")}</div> :
                <div className="alert-rule-list">
                  {rules.map((rule) => (
                    <article className="alert-rule-card" key={rule.id}>
                      <div className="alert-rule-copy">
                        <div className="alert-rule-title-row">
                          <strong>{rule.name}</strong>
                          <span className={`alert-enabled-pill ${rule.enabled ? "is-enabled" : "is-disabled"}`}>{t(language, rule.enabled ? "alertEnabled" : "alertDisabled")}</span>
                        </div>
                        <p>{rule.description || t(language, "alertNoDescription")}</p>
                        <small>{rule.service_name || t(language, "alertAllServices")} · {rule.metric} · {rule.operator} {number(rule.threshold, language, 3)} req/s · {number(rule.window_seconds, language, 0)}s · {number(rule.pending_seconds, language, 0)}s</small>
                      </div>
                      {canManage && (
                        <div className="alert-rule-actions">
                          <button type="button" className="alert-icon-action" title={t(language, "alertRuleEdit")} aria-label={t(language, "alertRuleEdit")} onClick={() => startEdit(rule)} disabled={saving}><Pencil size={13} /></button>
                          <button type="button" className="alert-icon-action" title={t(language, rule.enabled ? "alertDisable" : "alertEnable")} aria-label={t(language, rule.enabled ? "alertDisable" : "alertEnable")} onClick={() => void toggleRule(rule)} disabled={saving}>{rule.enabled ? <Pause size={13} /> : <Play size={13} />}</button>
                          <button type="button" className="alert-icon-action danger" title={t(language, "alertDelete")} aria-label={t(language, "alertDelete")} onClick={() => void removeRule(rule)} disabled={saving}><Trash2 size={13} /></button>
                        </div>
                      )}
                    </article>
                  ))}
                </div>
              }
            </section>

            <section className="panel alert-panel">
              <div className="alert-panel-heading">
                <div><h3>{t(language, "alertInstancesTitle")}</h3><p>{t(language, "alertInstancesSubtitle")}</p></div>
                <div className="incident-filter alert-filter">
                  <button type="button" className={filter === "active" ? "selected" : ""} onClick={() => setFilter("active")}>{t(language, "alertActiveFilter")}</button>
                  <button type="button" className={filter === "all" ? "selected" : ""} onClick={() => setFilter("all")}>{t(language, "all")}</button>
                  <button type="button" className={filter === "history" ? "selected" : ""} onClick={() => setFilter("history")}>{t(language, "alertHistoryFilter")}</button>
                </div>
              </div>
              {visibleInstances.length === 0 && !loading ? <div className="alert-empty">{t(language, "alertNoInstances")}</div> :
                <div className="alert-instance-list">
                  {visibleInstances.map((item) => (
                      <article className="alert-instance-card" key={item.id}>
                        <div className="alert-instance-top">
                          <span className={`alert-status-pill status-${item.status}`}>{t(language, statusKey(item.status))}</span>
                          <span className={`alert-severity severity-${item.severity}`}>{t(language, item.severity)}</span>
                        </div>
                        <strong className="alert-instance-title">{item.rule_name}</strong>
                        <p>{item.service_name} · {item.metric}</p>
                        <div className="alert-instance-values">
                          <span>{t(language, "alertCurrentValue")} <strong>{number(item.value, language, 3)} req/s</strong></span>
                          <span>{t(language, "alertThreshold")} <strong>{item.operator} {number(item.threshold, language, 3)} req/s</strong></span>
                        </div>
                        <small><Clock3 size={11} /> {t(language, "alertLastEvaluated")} {dateTime(item.last_evaluated_at, language)}</small>
                        {item.fired_at && <small><Activity size={11} /> {t(language, "alertFiredAt")} {dateTime(item.fired_at, language)}</small>}
                        {item.resolved_at && <small><Check size={11} /> {t(language, "alertResolvedAt")} {dateTime(item.resolved_at, language)}</small>}
                      </article>
                  ))}
                </div>
              }
            </section>
          </div>
          <p className="alert-footnote"><Activity size={12} /> {t(language, "alertOnlyRequestRate")} · {t(language, "alertAutoRefresh")}</p>
        </>
      )}
    </section>
  );
}

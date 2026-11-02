"use client";

import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import {
  Activity,
  Check,
  Edit3,
  Gauge,
  Pause,
  Play,
  Plus,
  RefreshCw,
  ShieldCheck,
  Target,
  Trash2,
  X,
} from "lucide-react";
import {
  createWorkspaceSLO,
  deleteWorkspaceSLO,
  getWorkspaceMembership,
  getWorkspaceSLOBudget,
  listWorkspaceServices,
  listWorkspaceSLOs,
  listWorkspaces,
  setWorkspaceSLOEnabled,
  updateWorkspaceSLO,
} from "../../lib/orbit/client";
import type {
  ServiceSummary,
  SLOAvailabilityBudget,
  WorkspaceMembershipSummary,
  WorkspaceSLO,
  WorkspaceSLOBudget,
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

type Draft = {
  name: string;
  route: string;
  targetPercent: string;
  rollingWindowDays: 1 | 3 | 7;
};

const emptyDraft = (): Draft => ({
  name: "",
  route: "/emit",
  targetPercent: "99.9",
  rollingWindowDays: 7,
});

function dateTime(value: string | null | undefined, language: Language): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat(formatLocale(language), {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function number(value: number, language: Language, maximumFractionDigits = 2): string {
  return new Intl.NumberFormat(formatLocale(language), { maximumFractionDigits }).format(value);
}

function percent(value: number, language: Language): string {
  return number(value * 100, language, 3) + "%";
}

function budgetStatusKey(status: SLOAvailabilityBudget["status"]): TranslationKey {
  switch (status) {
    case "insufficient_data": return "sloStatusInsufficient";
    case "within_budget": return "sloStatusWithinBudget";
    case "exhausted": return "sloStatusExhausted";
  }
}

export default function SLOWorkspace({ apiBaseUrl, language, sourceMode }: Props) {
  const [user, setUser] = useState<OrbitAuthUser | null>(() => getCurrentAuthUser());
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([]);
  const [workspaceId, setWorkspaceId] = useState("");
  const [membership, setMembership] = useState<WorkspaceMembershipSummary | null>(null);
  const [services, setServices] = useState<ServiceSummary[]>([]);
  const [serviceId, setServiceId] = useState("");
  const [slos, setSlos] = useState<WorkspaceSLO[]>([]);
  const [selectedSloId, setSelectedSloId] = useState("");
  const [budget, setBudget] = useState<WorkspaceSLOBudget | null>(null);
  const [budgetLoading, setBudgetLoading] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [editingSlo, setEditingSlo] = useState<WorkspaceSLO | null>(null);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [reloadVersion, setReloadVersion] = useState(0);
  const [budgetReload, setBudgetReload] = useState(0);

  useEffect(() => subscribeToAuth(() => {
    setUser(getCurrentAuthUser());
    setWorkspaces([]);
    setWorkspaceId("");
    setMembership(null);
    setServices([]);
    setServiceId("");
    setSlos([]);
    setSelectedSloId("");
    setBudget(null);
  }), []);

  useEffect(() => {
    if (sourceMode !== "api" || !user) {
      setWorkspaces([]);
      setWorkspaceId("");
      setMembership(null);
      setServices([]);
      setServiceId("");
      setSlos([]);
      setSelectedSloId("");
      setBudget(null);
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
        if (current) setError(t(language, "sloLoadFailed"));
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
      listWorkspaceSLOs(workspaceId, undefined, controller.signal, apiBaseUrl),
    ])
      .then(([role, serviceItems, sloItems]) => {
        if (!current) return;
        setMembership(role);
        setServices(serviceItems);
        setSlos(sloItems);
        setServiceId((previous) =>
          previous && serviceItems.some((service) => service.id === previous)
            ? previous
            : serviceItems[0]?.id ?? "",
        );
        setSelectedSloId((previous) =>
          previous && sloItems.some((item) => item.id === previous)
            ? previous
            : sloItems[0]?.id ?? "",
        );
      })
      .catch(() => {
        if (current) {
          setMembership(null);
          setServices([]);
          setSlos([]);
          setServiceId("");
          setSelectedSloId("");
          setBudget(null);
          setError(t(language, "sloLoadFailed"));
        }
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [apiBaseUrl, language, reloadVersion, sourceMode, user?.sub, workspaceId]);

  const serviceSLOs = useMemo(
    () => slos.filter((item) => item.service_id === serviceId),
    [serviceId, slos],
  );
  const selectedSlo = useMemo(
    () => serviceSLOs.find((item) => item.id === selectedSloId) ?? null,
    [selectedSloId, serviceSLOs],
  );
  const selectedService = useMemo(
    () => services.find((item) => item.id === serviceId) ?? null,
    [serviceId, services],
  );

  useEffect(() => {
    if (!selectedSlo || !selectedSlo.enabled || sourceMode !== "api" || !user) {
      setBudget(null);
      return;
    }
    const controller = new AbortController();
    let current = true;
    setBudgetLoading(true);
    setBudget(null);
    getWorkspaceSLOBudget(workspaceId, selectedSlo.id, controller.signal, apiBaseUrl)
      .then((result) => {
        if (current) setBudget(result);
      })
      .catch(() => {
        if (current) setError(t(language, "sloLoadFailed"));
      })
      .finally(() => {
        if (current) setBudgetLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [apiBaseUrl, budgetReload, language, selectedSlo?.id, selectedSlo?.enabled, sourceMode, user?.sub, workspaceId]);

  useEffect(() => {
    if (sourceMode !== "api" || !user || !workspaceId) return;
    const timer = window.setInterval(() => setReloadVersion((value) => value + 1), 30_000);
    return () => window.clearInterval(timer);
  }, [sourceMode, user?.sub, workspaceId]);

  const canManage = Boolean(
    isOidcConfigured() &&
    user &&
    membership &&
    ["owner", "administrator", "operator"].includes(membership.role),
  );

  const startCreate = () => {
    setEditingSlo(null);
    setDraft(emptyDraft());
    setShowForm(true);
    setError("");
    setNotice("");
  };

  const startEdit = (item: WorkspaceSLO) => {
    setEditingSlo(item);
    setDraft({
      name: item.name,
      route: item.route,
      targetPercent: String(item.target * 100),
      rollingWindowDays: item.rolling_window_days,
    });
    setSelectedSloId(item.id);
    setBudget(null);
    setShowForm(true);
    setError("");
    setNotice("");
  };

  const closeForm = () => {
    setShowForm(false);
    setEditingSlo(null);
    setDraft(emptyDraft());
  };

  const submitSLO = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!workspaceId || !serviceId || !canManage || !draft.name.trim()) return;
    const targetPercent = Number(draft.targetPercent);
    const target = targetPercent / 100;
    if (!Number.isFinite(targetPercent) || target < 0.9 || target > 0.9999) {
      setError(t(language, "sloMutationFailed"));
      return;
    }
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const input = {
        name: draft.name.trim(),
        route: draft.route.trim(),
        target,
        rolling_window_days: draft.rollingWindowDays,
      };
      if (editingSlo) {
        const updated = await updateWorkspaceSLO(workspaceId, editingSlo.id, input, apiBaseUrl);
        setSelectedSloId(updated.id);
        setNotice(t(language, "sloUpdated"));
      } else {
        const created = await createWorkspaceSLO(workspaceId, {
          service_id: serviceId,
          ...input,
        }, apiBaseUrl);
        setSelectedSloId(created.id);
        setNotice(t(language, "sloCreated"));
      }
      closeForm();
      setReloadVersion((value) => value + 1);
      setBudgetReload((value) => value + 1);
    } catch {
      setError(t(language, "sloMutationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const toggleSLO = async (item: WorkspaceSLO) => {
    if (!workspaceId || !canManage) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      await setWorkspaceSLOEnabled(workspaceId, item.id, !item.enabled, apiBaseUrl);
      setNotice(t(language, item.enabled ? "sloDisabled" : "sloEnabled"));
      if (selectedSloId === item.id) setBudget(null);
      setReloadVersion((value) => value + 1);
      setBudgetReload((value) => value + 1);
    } catch {
      setError(t(language, "sloMutationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const removeSLO = async (item: WorkspaceSLO) => {
    if (!workspaceId || !canManage || !window.confirm(t(language, "sloDeleteConfirm"))) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      await deleteWorkspaceSLO(workspaceId, item.id, apiBaseUrl);
      if (selectedSloId === item.id) {
        setSelectedSloId("");
        setBudget(null);
      }
      setNotice(t(language, "sloDeleted"));
      setReloadVersion((value) => value + 1);
    } catch {
      setError(t(language, "sloMutationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const refreshBudget = () => setBudgetReload((value) => value + 1);

  if (sourceMode === "demo") {
    return (
      <section className="slo-workspace">
        <div className="panel slo-notice">
          <span className="slo-heading-icon"><ShieldCheck size={18} /></span>
          <div><h2>{t(language, "sloWorkspaceTitle")}</h2><p>{t(language, "sloDemoReadOnly")}</p></div>
        </div>
      </section>
    );
  }

  if (!user) {
    return (
      <section className="slo-workspace">
        <div className="panel slo-notice">
          <span className="slo-heading-icon"><ShieldCheck size={18} /></span>
          <div>
            <h2>{t(language, "sloWorkspaceTitle")}</h2>
            <p>{isOidcConfigured() ? t(language, "sloSignInRequired") : t(language, "sloAuthNotConfigured")}</p>
          </div>
        </div>
      </section>
    );
  }

  return (
    <section className="slo-workspace">
      <div className="slo-toolbar">
        <div>
          <div className="incident-kicker"><Target size={12} /> {t(language, "sloOperations")}</div>
          <h2>{t(language, "sloWorkspaceTitle")}</h2>
          <p>{t(language, "sloWorkspaceSubtitle")}</p>
        </div>
        <div className="slo-toolbar-actions">
          <label className="slo-workspace-select">
            <span>{t(language, "incidentSelectWorkspace")}</span>
            <select value={workspaceId} onChange={(event) => setWorkspaceId(event.target.value)}>
              {workspaces.map((workspace) => <option key={workspace.id} value={workspace.id}>{workspace.name}</option>)}
            </select>
          </label>
          <button type="button" className="button-secondary" onClick={() => setReloadVersion((value) => value + 1)} disabled={loading}>
            <RefreshCw size={14} /> {t(language, "retry")}
          </button>
          {canManage && (
            <button type="button" className="button-primary" onClick={startCreate} disabled={!serviceId || saving}>
              <Plus size={14} /> {t(language, "sloCreate")}
            </button>
          )}
        </div>
      </div>

      {error && <div className="incident-feedback error" role="alert">{error}</div>}
      {notice && <div className="incident-feedback success" role="status"><Check size={13} /> {notice}</div>}
      {membership && !canManage && <div className="incident-inline-notice"><ShieldCheck size={14} /> {t(language, "sloRoleReadOnly")}</div>}

      {!workspaces.length && !loading ? (
        <div className="panel slo-empty">{t(language, "incidentNoWorkspaces")}</div>
      ) : services.length === 0 && !loading ? (
        <div className="panel slo-empty">{t(language, "sloNoServices")}</div>
      ) : (
        <>
          <div className="slo-service-toolbar">
            <label className="slo-service-select">
              <span>{t(language, "sloService")}</span>
              <select value={serviceId} onChange={(event) => {
                setServiceId(event.target.value);
                setSelectedSloId("");
                setBudget(null);
              }}>
                {services.map((service) => <option key={service.id} value={service.id}>{service.name} · {service.environment}</option>)}
              </select>
            </label>
            {selectedService && <span className="slo-environment-pill">{selectedService.environment}</span>}
          </div>

          {showForm && (
            <section className="panel slo-form-panel">
              <div className="slo-panel-heading">
                <div><h3>{t(language, editingSlo ? "sloEditTitle" : "sloCreateTitle")}</h3><p>{t(language, "sloValidEventPolicy")}</p></div>
                <button type="button" className="icon-button" onClick={closeForm} aria-label={t(language, "cancel")}><X size={15} /></button>
              </div>
              <form className="slo-form-grid" onSubmit={(event) => void submitSLO(event)}>
                <label>
                  <span>{t(language, "sloService")}</span>
                  <select value={serviceId} onChange={(event) => setServiceId(event.target.value)} disabled={Boolean(editingSlo)}>
                    {services.map((service) => <option key={service.id} value={service.id}>{service.name} · {service.environment}</option>)}
                  </select>
                </label>
                <label>
                  <span>{t(language, "sloName")}</span>
                  <input required maxLength={120} value={draft.name} onChange={(event) => setDraft((value) => ({ ...value, name: event.target.value }))} />
                </label>
                <label>
                  <span>{t(language, "sloRoute")}</span>
                  <input required maxLength={128} pattern="/[A-Za-z0-9][A-Za-z0-9/_-]{0,127}" value={draft.route} onChange={(event) => setDraft((value) => ({ ...value, route: event.target.value }))} />
                  <small>{t(language, "sloRouteHelp")}</small>
                </label>
                <label>
                  <span>{t(language, "sloTarget")}</span>
                  <input type="number" required min="90" max="99.99" step="0.01" value={draft.targetPercent} onChange={(event) => setDraft((value) => ({ ...value, targetPercent: event.target.value }))} />
                  <small>{t(language, "sloTargetHelp")}</small>
                </label>
                <label>
                  <span>{t(language, "sloWindow")}</span>
                  <select value={draft.rollingWindowDays} onChange={(event) => setDraft((value) => ({ ...value, rollingWindowDays: Number(event.target.value) as 1 | 3 | 7 }))}>
                    <option value={1}>1 {t(language, "sloDay")}</option>
                    <option value={3}>3 {t(language, "sloDays")}</option>
                    <option value={7}>7 {t(language, "sloDays")}</option>
                  </select>
                </label>
                <div className="slo-form-actions">
                  <button type="button" className="button-secondary" onClick={closeForm}>{t(language, "cancel")}</button>
                  <button type="submit" className="button-primary" disabled={saving}>{saving ? t(language, "saving") : t(language, editingSlo ? "sloUpdate" : "sloSave")}</button>
                </div>
              </form>
            </section>
          )}

          <section className="panel slo-panel">
            <div className="slo-panel-heading">
              <div><h3>{t(language, "sloObjectives")}</h3><p>{t(language, "sloDefinitionsSubtitle")}</p></div>
              {loading && <span className="slo-loading"><span className="telemetry-spinner" /></span>}
            </div>
            {serviceSLOs.length === 0 && !loading ? (
              <div className="slo-empty">{t(language, "sloNoObjectives")}</div>
            ) : (
              <div className="slo-objective-list">
                {serviceSLOs.map((item) => (
                  <article className={"slo-objective-card " + (selectedSloId === item.id ? "is-selected" : "")} key={item.id}>
                    <button type="button" className="slo-objective-select" onClick={() => {
                      setSelectedSloId(item.id);
                      setBudget(null);
                    }}>
                      <span className="slo-objective-title">
                        <strong>{item.name}</strong>
                        <span className={"slo-state-pill " + (item.enabled ? "is-enabled" : "is-disabled")}>
                          {t(language, item.enabled ? "sloEnabled" : "sloDisabled")}
                        </span>
                      </span>
                      <span className="slo-objective-meta">
                        <code>{item.route}</code>
                        <span>{percent(item.target, language)}</span>
                        <span>{number(item.rolling_window_days, language, 0)} {t(language, "sloDays")}</span>
                      </span>
                    </button>
                    {canManage && (
                      <div className="slo-objective-actions">
                        <button type="button" className="slo-icon-action" onClick={() => startEdit(item)} title={t(language, "edit")} aria-label={t(language, "edit")} disabled={saving}><Edit3 size={13} /></button>
                        <button type="button" className="slo-icon-action" onClick={() => void toggleSLO(item)} title={t(language, item.enabled ? "sloDisable" : "sloEnable")} aria-label={t(language, item.enabled ? "sloDisable" : "sloEnable")} disabled={saving}>
                          {item.enabled ? <Pause size={13} /> : <Play size={13} />}
                        </button>
                        <button type="button" className="slo-icon-action danger" onClick={() => void removeSLO(item)} title={t(language, "sloDelete")} aria-label={t(language, "sloDelete")} disabled={saving}><Trash2 size={13} /></button>
                      </div>
                    )}
                  </article>
                ))}
              </div>
            )}
          </section>

          <section className="panel slo-budget-panel">
            <div className="slo-panel-heading">
              <div><h3><Gauge size={14} /> {t(language, "sloBudgetTitle")}</h3><p>{t(language, "sloBudgetHelp")}</p></div>
              {selectedSlo?.enabled && (
                <button type="button" className="button-secondary" onClick={refreshBudget} disabled={budgetLoading}>
                  <RefreshCw size={13} /> {t(language, "sloRefreshBudget")}
                </button>
              )}
            </div>
            {!selectedSlo ? (
              <div className="slo-empty">{t(language, "sloNoBudget")}</div>
            ) : !selectedSlo.enabled ? (
              <div className="slo-empty">{t(language, "sloDisabled")}</div>
            ) : budgetLoading ? (
              <div className="slo-empty"><span className="telemetry-spinner" /> {t(language, "loading")}</div>
            ) : !budget ? (
              <div className="slo-empty">{t(language, "sloLoadFailed")}</div>
            ) : budget.budget.status === "insufficient_data" ? (
              <div className="slo-insufficient">
                <span className="slo-status-pill status-insufficient">{t(language, budgetStatusKey(budget.budget.status))}</span>
                <p>{t(language, "sloInsufficientData")}</p>
                <small>{t(language, "sloEvaluatedWindow")}: {dateTime(budget.window_start, language)} — {dateTime(budget.window_end, language)}</small>
              </div>
            ) : (
              <>
                <div className="slo-budget-status-row">
                  <span className={"slo-status-pill status-" + budget.budget.status}>{t(language, budgetStatusKey(budget.budget.status))}</span>
                  <small>{t(language, "sloEvaluatedWindow")}: {dateTime(budget.window_start, language)} — {dateTime(budget.window_end, language)}</small>
                </div>
                <div className="slo-budget-summary">
                  <div><span>{t(language, "sloAvailability")}</span><strong>{percent(budget.budget.availability ?? 0, language)}</strong></div>
                  <div><span>{t(language, "sloBudgetRemaining")}</span><strong>{number(budget.budget.budget_remaining_percent ?? 0, language, 2)}%</strong></div>
                  <div><span>{t(language, "sloBurnRate")}</span><strong>{number(budget.budget.burn_rate ?? 0, language, 3)}×</strong></div>
                </div>
                <div className="slo-budget-details">
                  <div><span>{t(language, "sloGoodEvents")}</span><strong>{number(budget.budget.good_events, language)}</strong></div>
                  <div><span>{t(language, "sloBadEvents")}</span><strong>{number(budget.budget.bad_events, language)}</strong></div>
                  <div><span>{t(language, "sloTotalEvents")}</span><strong>{number(budget.budget.total_events, language)}</strong></div>
                  <div><span>{t(language, "sloAllowedErrors")}</span><strong>{number(budget.budget.allowed_bad_events ?? 0, language)}</strong></div>
                  <div><span>{t(language, "sloRemainingErrors")}</span><strong>{number(budget.budget.remaining_bad_events ?? 0, language)}</strong></div>
                  <div><span>{t(language, "sloBudgetConsumed")}</span><strong>{number(budget.budget.budget_consumed_percent ?? 0, language, 2)}%</strong></div>
                </div>
              </>
            )}
            <p className="slo-valid-policy"><ShieldCheck size={12} /> {t(language, "sloValidEventPolicy")}</p>
          </section>
        </>
      )}
    </section>
  );
}

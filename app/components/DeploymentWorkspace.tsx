"use client";

import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import { ArrowLeftRight, Check, Clock3, GitBranch, Plus, RefreshCw, ShieldCheck, X } from "lucide-react";
import {
  compareWorkspaceDeploymentAnnotations,
  createWorkspaceDeploymentAnnotation,
  getWorkspaceMembership,
  listWorkspaceDeploymentAnnotations,
  listWorkspaceServices,
  listWorkspaces,
} from "../../lib/orbit/client";
import type {
  DeploymentAnnotationComparison,
  ServiceSummary,
  WorkspaceDeploymentAnnotation,
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

function dateTime(value: string | null | undefined, language: Language): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat(formatLocale(language), {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function number(value: number, language: Language): string {
  return new Intl.NumberFormat(formatLocale(language), { maximumFractionDigits: 0 }).format(value);
}

function elapsedLabel(seconds: number, language: Language): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return number(hours, language) + " " + t(language, "deploymentHours") + " " + number(minutes, language) + " " + t(language, "deploymentMinutes");
  return number(minutes, language) + " " + t(language, "deploymentMinutes");
}

export default function DeploymentWorkspace({ apiBaseUrl, language, sourceMode }: Props) {
  const [user, setUser] = useState<OrbitAuthUser | null>(() => getCurrentAuthUser());
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([]);
  const [workspaceId, setWorkspaceId] = useState("");
  const [membership, setMembership] = useState<WorkspaceMembershipSummary | null>(null);
  const [services, setServices] = useState<ServiceSummary[]>([]);
  const [serviceId, setServiceId] = useState("");
  const [annotations, setAnnotations] = useState<WorkspaceDeploymentAnnotation[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [version, setVersion] = useState("");
  const [commitSHA, setCommitSHA] = useState("");
  const [changeReference, setChangeReference] = useState("");
  const [releaseNotes, setReleaseNotes] = useState("");
  const [deployedAt, setDeployedAt] = useState("");
  const [fromId, setFromId] = useState("");
  const [toId, setToId] = useState("");
  const [comparison, setComparison] = useState<DeploymentAnnotationComparison | null>(null);
  const [comparing, setComparing] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [reloadVersion, setReloadVersion] = useState(0);

  useEffect(() => subscribeToAuth(() => {
    setUser(getCurrentAuthUser());
    setWorkspaces([]);
    setWorkspaceId("");
    setMembership(null);
    setServices([]);
    setServiceId("");
    setAnnotations([]);
    setComparison(null);
  }), []);

  useEffect(() => {
    if (sourceMode !== "api" || !user) {
      setWorkspaces([]);
      setWorkspaceId("");
      setMembership(null);
      setServices([]);
      setServiceId("");
      setAnnotations([]);
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
      .catch(() => { if (current) setError(t(language, "deploymentLoadFailed")); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; controller.abort(); };
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
    ])
      .then(([role, serviceItems]) => {
        if (!current) return;
        setMembership(role);
        setServices(serviceItems);
        setServiceId((previous) =>
          previous && serviceItems.some((service) => service.id === previous)
            ? previous
            : serviceItems[0]?.id ?? "",
        );
      })
      .catch(() => {
        if (current) {
          setMembership(null);
          setServices([]);
          setServiceId("");
          setError(t(language, "deploymentLoadFailed"));
        }
      })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; controller.abort(); };
  }, [apiBaseUrl, language, sourceMode, user?.sub, workspaceId]);

  useEffect(() => {
    if (!workspaceId || !serviceId || sourceMode !== "api" || !user) {
      setAnnotations([]);
      setComparison(null);
      setFromId("");
      setToId("");
      return;
    }
    const controller = new AbortController();
    let current = true;
    setLoading(true);
    setError("");
    listWorkspaceDeploymentAnnotations(workspaceId, serviceId, controller.signal, apiBaseUrl)
      .then((items) => {
        if (!current) return;
        setAnnotations(items);
        setComparison(null);
        setFromId(items.length > 1 ? items[items.length - 1].id : items[0]?.id ?? "");
        setToId(items[0]?.id ?? "");
      })
      .catch(() => { if (current) setError(t(language, "deploymentLoadFailed")); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; controller.abort(); };
  }, [apiBaseUrl, language, reloadVersion, serviceId, sourceMode, user?.sub, workspaceId]);

  useEffect(() => {
    if (sourceMode !== "api" || !user || !workspaceId) return;
    const timer = window.setInterval(() => setReloadVersion((value) => value + 1), 30_000);
    return () => window.clearInterval(timer);
  }, [sourceMode, user?.sub, workspaceId]);

  const canManage = Boolean(
    isOidcConfigured() && user && membership &&
    ["owner", "administrator", "operator"].includes(membership.role),
  );
  const selectedService = useMemo(
    () => services.find((service) => service.id === serviceId) ?? null,
    [services, serviceId],
  );
  const canCompare = Boolean(fromId && toId && fromId !== toId);

  const closeForm = () => {
    setShowForm(false);
    setVersion("");
    setCommitSHA("");
    setChangeReference("");
    setReleaseNotes("");
    setDeployedAt("");
  };

  const submitDeployment = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!workspaceId || !serviceId || !canManage || !version.trim()) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      await createWorkspaceDeploymentAnnotation(workspaceId, {
        service_id: serviceId,
        version: version.trim(),
        commit_sha: commitSHA.trim(),
        change_reference: changeReference.trim(),
        release_notes: releaseNotes.trim(),
        ...(deployedAt ? { deployed_at: new Date(deployedAt).toISOString() } : {}),
      }, apiBaseUrl);
      closeForm();
      setNotice(t(language, "deploymentCreated"));
      setReloadVersion((value) => value + 1);
    } catch {
      setError(t(language, "deploymentMutationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const runComparison = async () => {
    if (!workspaceId || !canCompare) return;
    setComparing(true);
    setError("");
    setComparison(null);
    try {
      const result = await compareWorkspaceDeploymentAnnotations(
        workspaceId, fromId, toId, undefined, apiBaseUrl,
      );
      setComparison(result);
    } catch {
      setError(t(language, "deploymentCompareInvalid"));
    } finally {
      setComparing(false);
    }
  };

  if (sourceMode === "demo") {
    return (
      <section className="deployment-workspace">
        <div className="panel deployment-notice">
          <span className="deployment-heading-icon"><ShieldCheck size={18} /></span>
          <div><h2>{t(language, "deploymentWorkspaceTitle")}</h2><p>{t(language, "deploymentDemoReadOnly")}</p></div>
        </div>
      </section>
    );
  }

  if (!user) {
    return (
      <section className="deployment-workspace">
        <div className="panel deployment-notice">
          <span className="deployment-heading-icon"><ShieldCheck size={18} /></span>
          <div>
            <h2>{t(language, "deploymentWorkspaceTitle")}</h2>
            <p>{isOidcConfigured() ? t(language, "deploymentSignInRequired") : t(language, "deploymentAuthNotConfigured")}</p>
          </div>
        </div>
      </section>
    );
  }

  return (
    <section className="deployment-workspace">
      <div className="deployment-toolbar">
        <div>
          <div className="incident-kicker"><GitBranch size={12} /> {t(language, "deploymentOperations")}</div>
          <h2>{t(language, "deploymentWorkspaceTitle")}</h2>
          <p>{t(language, "deploymentWorkspaceSubtitle")}</p>
        </div>
        <div className="deployment-toolbar-actions">
          <label className="deployment-workspace-select">
            <span>{t(language, "incidentSelectWorkspace")}</span>
            <select value={workspaceId} onChange={(event) => setWorkspaceId(event.target.value)}>
              {workspaces.map((workspace) => <option key={workspace.id} value={workspace.id}>{workspace.name}</option>)}
            </select>
          </label>
          <button type="button" className="button-secondary" onClick={() => setReloadVersion((value) => value + 1)} disabled={loading}>
            <RefreshCw size={14} /> {t(language, "retry")}
          </button>
          {canManage && (
            <button type="button" className="button-primary" onClick={() => setShowForm(true)} disabled={!serviceId || saving}>
              <Plus size={14} /> {t(language, "deploymentCreate")}
            </button>
          )}
        </div>
      </div>

      {error && <div className="incident-feedback error" role="alert">{error}</div>}
      {notice && <div className="incident-feedback success" role="status"><Check size={13} /> {notice}</div>}
      {membership && !canManage && (
        <div className="incident-inline-notice"><ShieldCheck size={14} /> {t(language, "deploymentRoleReadOnly")}</div>
      )}

      {!workspaces.length && !loading ? (
        <div className="panel deployment-empty">{t(language, "incidentNoWorkspaces")}</div>
      ) : services.length === 0 && !loading ? (
        <div className="panel deployment-empty">{t(language, "deploymentNoServices")}</div>
      ) : (
        <>
          <div className="deployment-toolbar-secondary">
            <label className="deployment-service-select">
              <span>{t(language, "deploymentService")}</span>
              <select value={serviceId} onChange={(event) => setServiceId(event.target.value)}>
                {services.map((service) => (
                  <option key={service.id} value={service.id}>
                    {service.name} · {service.environment}
                  </option>
                ))}
              </select>
            </label>
            {selectedService && (
              <span className="deployment-environment-pill">{selectedService.environment}</span>
            )}
          </div>

          {showForm && (
            <section className="panel deployment-form-panel">
              <div className="deployment-panel-heading">
                <div><h3>{t(language, "deploymentCreateTitle")}</h3><p>{t(language, "deploymentEnvironment")}: {selectedService?.environment ?? "—"}</p></div>
                <button type="button" className="icon-button" onClick={closeForm} aria-label={t(language, "cancel")}><X size={15} /></button>
              </div>
              <form className="deployment-form-grid" onSubmit={(event) => void submitDeployment(event)}>
                <label>
                  <span>{t(language, "deploymentService")}</span>
                  <select value={serviceId} onChange={(event) => setServiceId(event.target.value)}>
                    {services.map((service) => <option key={service.id} value={service.id}>{service.name} · {service.environment}</option>)}
                  </select>
                </label>
                <label>
                  <span>{t(language, "deploymentVersion")}</span>
                  <input required maxLength={128} value={version} onChange={(event) => setVersion(event.target.value)} placeholder="v1.4.0" />
                </label>
                <label>
                  <span>{t(language, "deploymentCommitSHA")}</span>
                  <input maxLength={64} pattern="[A-Fa-f0-9]{7,64}" value={commitSHA} onChange={(event) => setCommitSHA(event.target.value)} placeholder="abcdef1234567" spellCheck={false} />
                </label>
                <label>
                  <span>{t(language, "deploymentChangeReference")}</span>
                  <input maxLength={256} value={changeReference} onChange={(event) => setChangeReference(event.target.value)} placeholder="ORBIT-123" />
                </label>
                <label>
                  <span>{t(language, "deploymentTime")}</span>
                  <input type="datetime-local" value={deployedAt} onChange={(event) => setDeployedAt(event.target.value)} />
                </label>
                <label className="deployment-form-wide">
                  <span>{t(language, "deploymentReleaseNotes")}</span>
                  <textarea rows={3} maxLength={4000} value={releaseNotes} onChange={(event) => setReleaseNotes(event.target.value)} />
                </label>
                <div className="deployment-form-actions">
                  <button type="button" className="button-secondary" onClick={closeForm}>{t(language, "cancel")}</button>
                  <button type="submit" className="button-primary" disabled={saving}>
                    {saving ? t(language, "saving") : t(language, "deploymentSave")}
                  </button>
                </div>
              </form>
            </section>
          )}

          <section className="panel deployment-panel">
            <div className="deployment-panel-heading">
              <div><h3>{t(language, "deploymentHistory")}</h3><p>{t(language, "deploymentHistorySubtitle")}</p></div>
              {loading && <span className="deployment-loading"><span className="telemetry-spinner" /></span>}
            </div>
            {annotations.length === 0 && !loading ? (
              <div className="deployment-empty">{t(language, "deploymentNoHistory")}</div>
            ) : (
              <div className="deployment-history-list">
                {annotations.map((item) => (
                  <article className="deployment-history-card" key={item.id}>
                    <div className="deployment-history-top">
                      <strong>{item.version}</strong>
                      <span>{dateTime(item.deployed_at, language)}</span>
                    </div>
                    <div className="deployment-history-meta">
                      <span>{item.service_name}</span>
                      <span>{item.environment}</span>
                      {item.commit_sha && <code>{item.commit_sha}</code>}
                      {item.change_reference && <span>{item.change_reference}</span>}
                    </div>
                    {item.release_notes && <p>{item.release_notes}</p>}
                  </article>
                ))}
              </div>
            )}
          </section>

          <section className="panel deployment-compare-panel">
            <div className="deployment-panel-heading">
              <div><h3><ArrowLeftRight size={14} /> {t(language, "deploymentCompare")}</h3><p>{t(language, "deploymentCompareInvalid")}</p></div>
            </div>
            <div className="deployment-compare-controls">
              <label>
                <span>{t(language, "deploymentCompareFrom")}</span>
                <select value={fromId} onChange={(event) => { setFromId(event.target.value); setComparison(null); }}>
                  <option value="">{t(language, "deploymentSelectService")}</option>
                  {annotations.map((item) => <option key={item.id} value={item.id}>{item.version} · {dateTime(item.deployed_at, language)}</option>)}
                </select>
              </label>
              <label>
                <span>{t(language, "deploymentCompareTo")}</span>
                <select value={toId} onChange={(event) => { setToId(event.target.value); setComparison(null); }}>
                  <option value="">{t(language, "deploymentSelectService")}</option>
                  {annotations.map((item) => <option key={item.id} value={item.id}>{item.version} · {dateTime(item.deployed_at, language)}</option>)}
                </select>
              </label>
              <button type="button" className="button-primary" onClick={() => void runComparison()} disabled={!canCompare || comparing}>
                <ArrowLeftRight size={14} /> {comparing ? t(language, "loading") : t(language, "deploymentCompareRun")}
              </button>
            </div>
            {comparison && (
              <div className="deployment-compare-result">
                <div className="deployment-compare-releases">
                  <div><span>{comparison.from.version}</span><small>{dateTime(comparison.from.deployed_at, language)}</small></div>
                  <ArrowLeftRight size={16} />
                  <div><span>{comparison.to.version}</span><small>{dateTime(comparison.to.deployed_at, language)}</small></div>
                </div>
                <div className="deployment-compare-stats">
                  <div><span>{t(language, "deploymentCompareVersion")}</span><strong>{t(language, comparison.version_changed ? "yes" : "no")}</strong></div>
                  <div><span>{t(language, "deploymentCompareCommit")}</span><strong>{t(language, comparison.commit_changed ? "yes" : "no")}</strong></div>
                  <div><span>{t(language, "deploymentCompareElapsed")}</span><strong>{elapsedLabel(comparison.elapsed_seconds, language)}</strong></div>
                </div>
                <p>{t(language, "deploymentEnvironment")}: {comparison.from.environment} · {comparison.from.service_name}</p>
              </div>
            )}
          </section>
        </>
      )}
    </section>
  );
}

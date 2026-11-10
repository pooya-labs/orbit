"use client";

import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { LogIn, LogOut } from "lucide-react";
import { listWorkspaces } from "../../lib/orbit/client";
import {
  activateLocalWorkSession,
  beginOidcLogin,
  clearAuthSession,
  getCurrentAuthUser,
  isLocalWorkEnabled,
  isOidcConfigured,
  stageLocalWorkAccessToken,
  subscribeToAuth,
  type OrbitAuthUser
} from "../../lib/orbit/oidc";
import { isAppsScriptRuntime } from "../../lib/orbit/apps-script-bridge";
import { t, type Language } from "../../lib/orbit/i18n";

function displayName(user: OrbitAuthUser): string {
  return user.name || user.preferred_username || user.email || user.sub;
}

export default function AuthControls({ language }: { language: Language }) {
  const [user, setUser] = useState<OrbitAuthUser | null>(null);
  const [busy, setBusy] = useState(false);
  const [localKey, setLocalKey] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    setUser(getCurrentAuthUser());
    return subscribeToAuth(() => setUser(getCurrentAuthUser()));
  }, []);

  if (isAppsScriptRuntime()) {
    if (!user) return null;
    return (
      <div className="auth-controls" aria-live="polite">
        <span className="auth-user" title="Synthetic session; no production identity is used">
          {displayName(user)}
        </span>
      </div>
    );
  }

  if (isLocalWorkEnabled()) {
    const signInLocal = async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      setBusy(true);
      setError("");
      try {
        stageLocalWorkAccessToken(localKey);
        await listWorkspaces(undefined, process.env.NEXT_PUBLIC_ORBIT_API_URL || "http://localhost:8080");
        activateLocalWorkSession();
        setLocalKey("");
      } catch (cause) {
        clearAuthSession();
        setError(cause instanceof Error ? cause.message : "The local access key could not be verified.");
      } finally {
        setBusy(false);
      }
    };

    return (
      <div className="auth-controls" aria-live="polite">
        {user ? (
          <>
            <span className="auth-user" title={user.sub}>{displayName(user)}</span>
            <button className="auth-button" type="button" onClick={clearAuthSession} title={t(language, "signOut")}>
              <LogOut size={13} aria-hidden="true" /> {t(language, "signOut")}
            </button>
          </>
        ) : (
          <form className="auth-local-form" onSubmit={(event) => void signInLocal(event)}>
            <label className="sr-only" htmlFor="orbit-local-work-key">
              Local Work access key
            </label>
            <input
              id="orbit-local-work-key"
              className="auth-local-key"
              type="password"
              autoComplete="current-password"
              value={localKey}
              onChange={(event) => setLocalKey(event.target.value)}
              placeholder="Local access key"
              aria-describedby={error ? "orbit-local-work-error" : undefined}
              required
            />
            <button className="auth-button" type="submit" disabled={busy || localKey.trim().length < 32}>
              <LogIn size={13} aria-hidden="true" /> {busy ? "Verifying…" : "Unlock Local Work"}
            </button>
          </form>
        )}
        {error && <span id="orbit-local-work-error" className="auth-error" role="alert">{error}</span>}
      </div>
    );
  }

  if (!isOidcConfigured()) return null;

  const signIn = async () => {
    setBusy(true);
    setError("");
    try {
      await beginOidcLogin();
    } catch (cause) {
      setBusy(false);
      setError(cause instanceof Error ? cause.message : "Sign-in could not be started.");
    }
  };

  return (
    <div className="auth-controls" aria-live="polite">
      {user ? (
        <>
          <span className="auth-user" title={user.sub}>{displayName(user)}</span>
          <button className="auth-button" type="button" onClick={clearAuthSession} title={t(language, "signOut")}>
            <LogOut size={13} aria-hidden="true" /> {t(language, "signOut")}
          </button>
        </>
      ) : (
        <button className="auth-button" type="button" onClick={() => void signIn()} disabled={busy}>
          <LogIn size={13} aria-hidden="true" /> {busy ? t(language, "signingIn") : t(language, "signIn")}
        </button>
      )}
      {error && <span className="auth-error" role="alert">{error}</span>}
    </div>
  );
}

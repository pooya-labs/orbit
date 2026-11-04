"use client";

import { useEffect, useState } from "react";
import { LogIn, LogOut } from "lucide-react";
import {
  beginOidcLogin,
  clearAuthSession,
  getCurrentAuthUser,
  isOidcConfigured,
  isAppsScriptRuntime,
  subscribeToAuth,
  type OrbitAuthUser
} from "../../lib/orbit/oidc";
import { t, type Language } from "../../lib/orbit/i18n";

function displayName(user: OrbitAuthUser): string {
  return user.name || user.preferred_username || user.email || user.sub;
}

export default function AuthControls({ language }: { language: Language }) {
  const [user, setUser] = useState<OrbitAuthUser | null>(() => getCurrentAuthUser());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => subscribeToAuth(() => setUser(getCurrentAuthUser())), []);
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

"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { completeOidcCallback, OrbitAuthError } from "../../../lib/orbit/oidc";
import { t, type Language } from "../../../lib/orbit/i18n";

export default function OidcCallbackPage() {
  const router = useRouter();
  const started = useRef(false);
  const [language, setLanguage] = useState<Language>("en");
  const [error, setError] = useState("");

  useEffect(() => {
    if (window.navigator.language.toLowerCase().startsWith("fa")) setLanguage("fa");
  }, []);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const url = new URL(window.location.href);
    const params = {
      code: url.searchParams.get("code"),
      state: url.searchParams.get("state"),
      error: url.searchParams.get("error"),
      errorDescription: url.searchParams.get("error_description"),
      issuer: url.searchParams.get("iss")
    };
    // Remove authorization codes and provider error parameters from the visible URL/history.
    window.history.replaceState(window.history.state, "", url.pathname);
    void completeOidcCallback(params)
      .then(() => router.replace("/"))
      .catch((cause: unknown) => {
        setError(cause instanceof OrbitAuthError ? cause.message : "Sign-in could not be completed.");
      });
  }, [router]);

  return (
    <main className="auth-callback" lang={language} dir={language === "fa" ? "rtl" : "ltr"}>
      <section className="auth-callback-card" aria-live="polite">
        {!error && <div className="auth-callback-spinner" aria-hidden="true" />}
        <h1>{t(language, "authCallbackTitle")}</h1>
        <p>{error ? t(language, "authCallbackFailed") : t(language, "authCallbackMessage")}</p>
        {error && <p role="alert">{error}</p>}
      </section>
    </main>
  );
}

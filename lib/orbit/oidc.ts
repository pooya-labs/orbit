import { createRemoteJWKSet, jwtVerify } from "jose";
import { isAppsScriptRuntime } from "./apps-script-bridge";

export interface OrbitAuthUser {
  sub: string;
  name?: string;
  email?: string;
  preferred_username?: string;
  picture?: string;
}

interface OidcConfiguration {
  issuer: string;
  clientId: string;
  audience?: string;
  scope: string;
  redirectUri?: string;
}

interface DiscoveryDocument {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  jwks_uri: string;
  code_challenge_methods_supported?: string[];
  response_types_supported?: string[];
  token_endpoint_auth_methods_supported?: string[];
}

interface AuthTransaction {
  state: string;
  nonce: string;
  codeVerifier: string;
  redirectUri: string;
  createdAt: number;
}

interface TokenResponse {
  access_token?: unknown;
  id_token?: unknown;
  refresh_token?: unknown;
  token_type?: unknown;
  expires_in?: unknown;
  error?: unknown;
  error_description?: unknown;
}

interface ActiveSession {
  user: OrbitAuthUser;
  accessToken: string;
  refreshToken?: string;
  expiresAt: number;
}

export class OrbitAuthError extends Error {
  readonly code: string;
  constructor(message: string, code = "oidc_error") {
    super(message);
    this.name = "OrbitAuthError";
    this.code = code;
  }
}

const TRANSACTION_KEY = "orbit.oidc.transaction.v1";
const TRANSACTION_MAX_AGE_MS = 10 * 60 * 1000;
const ACCESS_TOKEN_SKEW_MS = 45 * 1000;
const ALLOWED_ID_TOKEN_ALGORITHMS = ["RS256", "ES256"] as const;

let activeSession: ActiveSession | null = null;
let sessionGeneration = 0;
let discoveryPromise: Promise<DiscoveryDocument> | null = null;
let refreshPromise: Promise<string | null> | null = null;
const listeners = new Set<() => void>();

export const orbitBasePath = process.env.NEXT_PUBLIC_ORBIT_BASE_PATH ?? "";

export function isOidcConfigured(): boolean {
  if (isAppsScriptRuntime()) return true;
  return Boolean(
    process.env.NEXT_PUBLIC_ORBIT_OIDC_ISSUER?.trim() &&
    process.env.NEXT_PUBLIC_ORBIT_OIDC_CLIENT_ID?.trim()
  );
}

function getConfiguration(): OidcConfiguration {
  const issuer = process.env.NEXT_PUBLIC_ORBIT_OIDC_ISSUER?.trim();
  const clientId = process.env.NEXT_PUBLIC_ORBIT_OIDC_CLIENT_ID?.trim();
  if (!issuer || !clientId) {
    throw new OrbitAuthError("Browser sign-in is not configured for this deployment.", "oidc_not_configured");
  }
  assertTrustedEndpoint(issuer, "OIDC issuer");
  const scopes = new Set(
    (process.env.NEXT_PUBLIC_ORBIT_OIDC_SCOPE?.trim() || "openid profile email").split(/\s+/).filter(Boolean)
  );
  scopes.add("openid");
  return {
    issuer,
    clientId,
    audience: process.env.NEXT_PUBLIC_ORBIT_OIDC_AUDIENCE?.trim() || undefined,
    scope: Array.from(scopes).join(" "),
    redirectUri: process.env.NEXT_PUBLIC_ORBIT_OIDC_REDIRECT_URI?.trim() || undefined
  };
}

function assertTrustedEndpoint(value: string, label: string): URL {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new OrbitAuthError(label + " must be an absolute URL.", "invalid_oidc_url");
  }
  const localHttpAllowed =
    process.env.NODE_ENV !== "production" &&
    url.protocol === "http:" &&
    ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname);
  if (url.protocol !== "https:" && !localHttpAllowed) {
    throw new OrbitAuthError(label + " must use HTTPS.", "insecure_oidc_endpoint");
  }
  if (url.username || url.password || url.search || url.hash) {
    throw new OrbitAuthError(label + " must not contain credentials, a query, or a fragment.", "invalid_oidc_url");
  }
  return url;
}

function getRedirectUri(config: OidcConfiguration): string {
  if (config.redirectUri) {
    assertTrustedEndpoint(config.redirectUri, "OIDC redirect URI");
    return config.redirectUri;
  }
  if (typeof window === "undefined") {
    throw new OrbitAuthError("The OIDC redirect URI is only available in a browser.", "browser_required");
  }
  return window.location.origin + orbitBasePath + "/auth/callback/";
}

async function loadDiscoveryDocument(): Promise<DiscoveryDocument> {
  const config = getConfiguration();
  const discoveryUrl = config.issuer.replace(/\/+$/, "") + "/.well-known/openid-configuration";
  let response: Response;
  try {
    response = await fetch(discoveryUrl, {
      method: "GET",
      headers: { Accept: "application/json" },
      cache: "no-store",
      credentials: "omit"
    });
  } catch {
    throw new OrbitAuthError("The identity provider discovery endpoint could not be reached.", "discovery_network_error");
  }
  const document = await response.json().catch(() => null) as DiscoveryDocument | null;
  if (!response.ok || !document) {
    throw new OrbitAuthError("The identity provider returned an invalid discovery response.", "invalid_discovery_response");
  }
  if (document.issuer !== config.issuer) {
    throw new OrbitAuthError("The discovery issuer does not exactly match NEXT_PUBLIC_ORBIT_OIDC_ISSUER.", "issuer_mismatch");
  }
  assertTrustedEndpoint(document.authorization_endpoint, "Authorization endpoint");
  assertTrustedEndpoint(document.token_endpoint, "Token endpoint");
  assertTrustedEndpoint(document.jwks_uri, "JWKS endpoint");
  if (!document.response_types_supported?.includes("code")) {
    throw new OrbitAuthError("The identity provider does not advertise the authorization-code flow.", "authorization_code_unsupported");
  }
  if (!document.code_challenge_methods_supported?.includes("S256")) {
    throw new OrbitAuthError("The identity provider must advertise PKCE S256 support.", "pkce_s256_unsupported");
  }
  if (
    document.token_endpoint_auth_methods_supported &&
    !document.token_endpoint_auth_methods_supported.includes("none")
  ) {
    throw new OrbitAuthError("The identity provider token endpoint does not advertise public-client authentication.", "public_client_unsupported");
  }
  return document;
}

async function getDiscoveryDocument(): Promise<DiscoveryDocument> {
  if (!discoveryPromise) {
    discoveryPromise = loadDiscoveryDocument().catch((error: unknown) => {
      discoveryPromise = null;
      throw error;
    });
  }
  return discoveryPromise;
}

function base64Url(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function randomValue(byteLength = 32): string {
  const bytes = new Uint8Array(byteLength);
  crypto.getRandomValues(bytes);
  return base64Url(bytes);
}

async function createCodeChallenge(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
  return base64Url(new Uint8Array(digest));
}

function storeTransaction(transaction: AuthTransaction): void {
  try {
    window.sessionStorage.setItem(TRANSACTION_KEY, JSON.stringify(transaction));
  } catch {
    throw new OrbitAuthError("This browser blocks session storage, so sign-in cannot be started safely.", "transaction_storage_unavailable");
  }
}

function takeTransaction(): AuthTransaction {
  let raw: string | null;
  try {
    raw = window.sessionStorage.getItem(TRANSACTION_KEY);
    window.sessionStorage.removeItem(TRANSACTION_KEY);
  } catch {
    throw new OrbitAuthError("The sign-in transaction could not be read from this browser session.", "transaction_storage_unavailable");
  }
  if (!raw) {
    throw new OrbitAuthError("The sign-in transaction is missing or has already been used. Start sign-in again.", "transaction_missing");
  }
  let transaction: AuthTransaction;
  try {
    transaction = JSON.parse(raw) as AuthTransaction;
  } catch {
    throw new OrbitAuthError("The sign-in transaction is invalid. Start sign-in again.", "transaction_invalid");
  }
  if (
    !transaction ||
    typeof transaction.state !== "string" ||
    typeof transaction.nonce !== "string" ||
    typeof transaction.codeVerifier !== "string" ||
    typeof transaction.redirectUri !== "string" ||
    typeof transaction.createdAt !== "number"
  ) {
    throw new OrbitAuthError("The sign-in transaction is incomplete. Start sign-in again.", "transaction_invalid");
  }
  const age = Date.now() - transaction.createdAt;
  if (age < -30_000 || age > TRANSACTION_MAX_AGE_MS) {
    throw new OrbitAuthError("The sign-in request expired. Start sign-in again.", "transaction_expired");
  }
  return transaction;
}

function notifyAuthChanged(): void {
  for (const listener of listeners) listener();
}

export function subscribeToAuth(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function getCurrentAuthUser(): OrbitAuthUser | null {
  if (isAppsScriptRuntime()) {
    return { sub: "demo-operator", name: "ORBIT Demo Operator", preferred_username: "demo-operator" };
  }
  return activeSession?.user ?? null;
}

export async function beginOidcLogin(): Promise<void> {
  const config = getConfiguration();
  if (typeof window === "undefined" || !window.crypto?.subtle) {
    throw new OrbitAuthError("A browser with Web Crypto support is required for secure sign-in.", "webcrypto_unavailable");
  }
  const discovery = await getDiscoveryDocument();
  const state = randomValue();
  const nonce = randomValue();
  const codeVerifier = randomValue(48);
  const redirectUri = getRedirectUri(config);
  const codeChallenge = await createCodeChallenge(codeVerifier);
  storeTransaction({ state, nonce, codeVerifier, redirectUri, createdAt: Date.now() });

  const authorizationUrl = new URL(discovery.authorization_endpoint);
  authorizationUrl.searchParams.set("client_id", config.clientId);
  authorizationUrl.searchParams.set("response_type", "code");
  authorizationUrl.searchParams.set("redirect_uri", redirectUri);
  authorizationUrl.searchParams.set("scope", config.scope);
  authorizationUrl.searchParams.set("state", state);
  authorizationUrl.searchParams.set("nonce", nonce);
  authorizationUrl.searchParams.set("code_challenge", codeChallenge);
  authorizationUrl.searchParams.set("code_challenge_method", "S256");
  if (config.audience) authorizationUrl.searchParams.set("audience", config.audience);
  window.location.assign(authorizationUrl.toString());
}

async function readTokenResponse(response: Response, context: string): Promise<TokenResponse> {
  const payload = await response.json().catch(() => null) as TokenResponse | null;
  if (!response.ok) {
    const description = typeof payload?.error_description === "string"
      ? payload.error_description.slice(0, 400)
      : context + " failed with HTTP " + response.status + ".";
    throw new OrbitAuthError(description, typeof payload?.error === "string" ? payload.error : "token_request_failed");
  }
  if (!payload || typeof payload !== "object") {
    throw new OrbitAuthError(context + " returned an invalid response.", "invalid_token_response");
  }
  return payload;
}

function validateTokenResponse(payload: TokenResponse): { accessToken: string; expiresIn: number; refreshToken?: string } {
  if (typeof payload.access_token !== "string" || !payload.access_token) {
    throw new OrbitAuthError("The identity provider did not return an access token.", "access_token_missing");
  }
  if (typeof payload.token_type !== "string" || payload.token_type.toLowerCase() !== "bearer") {
    throw new OrbitAuthError("The identity provider must return a Bearer token.", "invalid_token_type");
  }
  const expiresIn = Number(payload.expires_in);
  if (!Number.isFinite(expiresIn) || expiresIn <= 0 || expiresIn > 86_400) {
    throw new OrbitAuthError("The access-token lifetime is missing or invalid.", "invalid_token_lifetime");
  }
  return {
    accessToken: payload.access_token,
    expiresIn,
    refreshToken: typeof payload.refresh_token === "string" && payload.refresh_token ? payload.refresh_token : undefined
  };
}

async function verifyIdentityToken(
  idToken: string,
  nonce: string,
  discovery: DiscoveryDocument,
  config: OidcConfiguration
): Promise<OrbitAuthUser> {
  const jwks = createRemoteJWKSet(new URL(discovery.jwks_uri), {
    timeoutDuration: 5_000,
    cooldownDuration: 30_000,
    cacheMaxAge: 10 * 60 * 1000
  });
  let payload;
  try {
    const verified = await jwtVerify(idToken, jwks, {
      issuer: config.issuer,
      audience: config.clientId,
      algorithms: [...ALLOWED_ID_TOKEN_ALGORITHMS],
      maxTokenAge: "10m",
      clockTolerance: 60
    });
    payload = verified.payload;
  } catch {
    throw new OrbitAuthError("The ID token signature or required claims could not be verified.", "id_token_verification_failed");
  }
  if (payload.nonce !== nonce) {
    throw new OrbitAuthError("The ID token nonce does not match this sign-in request.", "nonce_mismatch");
  }
  if (typeof payload.azp === "string" && payload.azp !== config.clientId) {
    throw new OrbitAuthError("The ID token authorized party does not match this client.", "authorized_party_mismatch");
  }
  if (Array.isArray(payload.aud) && payload.aud.length > 1 && payload.azp !== config.clientId) {
    throw new OrbitAuthError("An ID token with multiple audiences must identify this client as its authorized party.", "authorized_party_missing");
  }
  if (typeof payload.sub !== "string" || !payload.sub) {
    throw new OrbitAuthError("The ID token does not contain a valid subject.", "subject_missing");
  }
  return {
    sub: payload.sub,
    name: typeof payload.name === "string" ? payload.name : undefined,
    email: typeof payload.email === "string" ? payload.email : undefined,
    preferred_username: typeof payload.preferred_username === "string" ? payload.preferred_username : undefined,
    picture: typeof payload.picture === "string" ? payload.picture : undefined
  };
}

export async function completeOidcCallback(params: {
  code?: string | null;
  state?: string | null;
  error?: string | null;
  errorDescription?: string | null;
  issuer?: string | null;
}): Promise<OrbitAuthUser> {
  const transaction = takeTransaction();
  if (!params.state || params.state !== transaction.state) {
    throw new OrbitAuthError("The sign-in state does not match this browser session.", "state_mismatch");
  }
  if (params.error) {
    const description = params.errorDescription?.trim().slice(0, 400);
    throw new OrbitAuthError(description || "The identity provider declined the sign-in request.", params.error);
  }
  if (!params.code) {
    throw new OrbitAuthError("The identity provider did not return an authorization code.", "authorization_code_missing");
  }

  const config = getConfiguration();
  if (params.issuer && params.issuer !== config.issuer) {
    throw new OrbitAuthError("The authorization response issuer does not match the configured identity provider.", "authorization_issuer_mismatch");
  }
  const discovery = await getDiscoveryDocument();
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    client_id: config.clientId,
    code: params.code,
    redirect_uri: transaction.redirectUri,
    code_verifier: transaction.codeVerifier
  });
  let response: Response;
  try {
    response = await fetch(discovery.token_endpoint, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/x-www-form-urlencoded"
      },
      body,
      cache: "no-store",
      credentials: "omit"
    });
  } catch {
    throw new OrbitAuthError("The token endpoint could not be reached. Check the identity provider CORS policy.", "token_network_error");
  }

  const tokenPayload = await readTokenResponse(response, "Authorization-code exchange");
  const tokens = validateTokenResponse(tokenPayload);
  if (typeof tokenPayload.id_token !== "string" || !tokenPayload.id_token) {
    throw new OrbitAuthError("The identity provider did not return an ID token.", "id_token_missing");
  }
  const user = await verifyIdentityToken(tokenPayload.id_token, transaction.nonce, discovery, config);
  sessionGeneration += 1;
  refreshPromise = null;
  activeSession = {
    user,
    accessToken: tokens.accessToken,
    refreshToken: tokens.refreshToken,
    expiresAt: Date.now() + tokens.expiresIn * 1000
  };
  notifyAuthChanged();
  return user;
}

async function refreshAccessToken(): Promise<string | null> {
  const previous = activeSession;
  const generation = sessionGeneration;
  if (!previous?.refreshToken) {
    if (sessionGeneration === generation && activeSession === previous) {
      activeSession = null;
      notifyAuthChanged();
    }
    throw new OrbitAuthError("Your access token expired. Sign in again.", "reauthentication_required");
  }

  try {
    const config = getConfiguration();
    const discovery = await getDiscoveryDocument();
    const body = new URLSearchParams({
      grant_type: "refresh_token",
      client_id: config.clientId,
      refresh_token: previous.refreshToken
    });
    const response = await fetch(discovery.token_endpoint, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/x-www-form-urlencoded"
      },
      body,
      cache: "no-store",
      credentials: "omit"
    });
    const payload = await readTokenResponse(response, "Access-token refresh");
    const tokens = validateTokenResponse(payload);

    // A late refresh response must never resurrect a session after sign-out or a newer login.
    if (sessionGeneration !== generation || activeSession !== previous) return null;

    activeSession = {
      ...previous,
      accessToken: tokens.accessToken,
      refreshToken: tokens.refreshToken ?? previous.refreshToken,
      expiresAt: Date.now() + tokens.expiresIn * 1000
    };
    notifyAuthChanged();
    return activeSession.accessToken;
  } catch (error) {
    if (sessionGeneration === generation && activeSession === previous) {
      activeSession = null;
      notifyAuthChanged();
    }
    if (error instanceof OrbitAuthError) throw error;
    throw new OrbitAuthError("The session could not be refreshed. Sign in again.", "refresh_failed");
  }
}

export async function getAccessToken(): Promise<string | null> {
  if (isAppsScriptRuntime() || !activeSession) return null;
  if (Date.now() < activeSession.expiresAt - ACCESS_TOKEN_SKEW_MS) {
    return activeSession.accessToken;
  }
  if (!refreshPromise) {
    const pending = refreshAccessToken();
    refreshPromise = pending;
    try {
      return await pending;
    } finally {
      if (refreshPromise === pending) refreshPromise = null;
    }
  }
  return refreshPromise;
}

export function clearAuthSession(): void {
  sessionGeneration += 1;
  activeSession = null;
  refreshPromise = null;
  notifyAuthChanged();
}

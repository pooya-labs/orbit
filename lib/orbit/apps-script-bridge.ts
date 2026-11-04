import { setAppsScriptDemoUser } from "./apps-script-session";

export type AppsScriptApiResponse = { status: number; body: unknown };
type BridgeBootstrap = {
  sessionToken: string;
  expiresInSeconds: number;
  user: { sub: string; name?: string; preferred_username?: string };
  workspace: { id: string; name: string };
  mode: "public-demo";
};
type PendingRequest = {
  resolve: (response: AppsScriptApiResponse) => void;
  reject: (error: Error) => void;
  timeout: number;
  signal?: AbortSignal;
  abort?: () => void;
};
const pendingRequests = new Map<string, PendingRequest>();
let sessionToken = "";
let trustedParentOrigin = "";
let readyPromise: Promise<BridgeBootstrap> | null = null;

export function isAppsScriptRuntime(): boolean {
  if (typeof window === "undefined") return false;
  return new URLSearchParams(window.location.search).get("orbitRuntime") === "apps-script";
}
function makeId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
  return hex.slice(0, 8) + "-" + hex.slice(8, 12) + "-" + hex.slice(12, 16) + "-" +
    hex.slice(16, 20) + "-" + hex.slice(20);
}
function getBrowserKey(): string {
  const key = "orbit.apps-script.browser.v1";
  try {
    const existing = window.localStorage.getItem(key);
    if (existing && /^[a-f0-9-]{36}$/i.test(existing)) return existing;
    const generated = makeId();
    window.localStorage.setItem(key, generated);
    return generated;
  } catch { return makeId(); }
}
function isTrustedAppsScriptOrigin(origin: string): boolean {
  try {
    const url = new URL(origin);
    return url.protocol === "https:" &&
      (url.hostname === "script.google.com" ||
       url.hostname === "script.googleusercontent.com" ||
       url.hostname.endsWith(".googleusercontent.com"));
  } catch { return false; }
}
function receiveBridgeMessage(event: MessageEvent<unknown>): void {
  if (event.source !== window.parent || !isTrustedAppsScriptOrigin(event.origin)) return;
  if (!event.data || typeof event.data !== "object") return;
  const message = event.data as Record<string, unknown>;
  if (message.type === "ORBIT_BRIDGE_INIT_ERROR") {
    sessionToken = "";
    return;
  }
  if (message.type === "ORBIT_BRIDGE_INIT") {
    const payload = message.payload as BridgeBootstrap | undefined;
    if (!payload || typeof payload.sessionToken !== "string" ||
        typeof payload.user?.sub !== "string" || typeof payload.workspace?.id !== "string") return;
    sessionToken = payload.sessionToken;
    trustedParentOrigin = event.origin;
    setAppsScriptDemoUser(payload.user);
    return;
  }
  if (message.type === "ORBIT_BRIDGE_RESPONSE" && typeof message.id === "string") {
    const pending = pendingRequests.get(message.id);
    if (!pending) return;
    pendingRequests.delete(message.id);
    window.clearTimeout(pending.timeout);
    if (pending.signal && pending.abort) pending.signal.removeEventListener("abort", pending.abort);
    const response = message.response as AppsScriptApiResponse | undefined;
    if (!response || typeof response.status !== "number") {
      pending.reject(new Error("The Apps Script bridge returned an invalid response."));
      return;
    }
    pending.resolve(response);
  }
}
if (typeof window !== "undefined") window.addEventListener("message", receiveBridgeMessage);

function ensureReady(): Promise<void> {
  if (!isAppsScriptRuntime()) return Promise.reject(new Error("The Apps Script bridge is not active."));
  if (sessionToken && trustedParentOrigin) return Promise.resolve();
  if (readyPromise) return readyPromise.then(() => undefined);
  readyPromise = new Promise<BridgeBootstrap>((resolve, reject) => {
    const timeout = window.setTimeout(() => {
      readyPromise = null;
      reject(new Error("Apps Script bridge timed out. Open the official ORBIT demo Web App URL."));
    }, 20000);
    const onMessage = (event: MessageEvent<unknown>) => {
      if (event.source !== window.parent || !isTrustedAppsScriptOrigin(event.origin)) return;
      if (!event.data || typeof event.data !== "object") return;
      const message = event.data as Record<string, unknown>;
      if (message.type === "ORBIT_BRIDGE_INIT_ERROR") {
        window.clearTimeout(timeout);
        window.removeEventListener("message", onMessage);
        readyPromise = null;
        reject(new Error(typeof message.message === "string" ? message.message : "The demo session could not start."));
        return;
      }
      if (message.type !== "ORBIT_BRIDGE_INIT") return;
      const payload = message.payload as BridgeBootstrap | undefined;
      if (!payload || typeof payload.sessionToken !== "string" ||
          typeof payload.user?.sub !== "string" || typeof payload.workspace?.id !== "string") {
        window.clearTimeout(timeout);
        window.removeEventListener("message", onMessage);
        readyPromise = null;
        reject(new Error("Apps Script returned an incomplete demo session."));
        return;
      }
      sessionToken = payload.sessionToken;
      trustedParentOrigin = event.origin;
      window.clearTimeout(timeout);
      window.removeEventListener("message", onMessage);
      resolve(payload);
    };
    window.addEventListener("message", onMessage);
    window.parent.postMessage({ type: "ORBIT_BRIDGE_HELLO", clientId: getBrowserKey() }, "*");
  });
  return readyPromise.then(() => undefined);
}
export async function requestAppsScriptApi(
  method: "GET" | "POST" | "PATCH" | "PUT" | "DELETE",
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<AppsScriptApiResponse> {
  await ensureReady();
  if (signal?.aborted) throw new DOMException("The request was aborted.", "AbortError");
  if (!trustedParentOrigin || !sessionToken) throw new Error("The Apps Script demo session is not ready.");
  const id = makeId();
  return new Promise<AppsScriptApiResponse>((resolve, reject) => {
    const timeout = window.setTimeout(() => {
      pendingRequests.delete(id);
      if (signal && abort) signal.removeEventListener("abort", abort);
      reject(new Error("The Apps Script demo request timed out."));
    }, 25000);
    const abort = () => {
      pendingRequests.delete(id);
      window.clearTimeout(timeout);
      reject(new DOMException("The request was aborted.", "AbortError"));
    };
    if (signal) signal.addEventListener("abort", abort, { once: true });
    pendingRequests.set(id, { resolve, reject, timeout, signal, abort });
    window.parent.postMessage({
      type: "ORBIT_BRIDGE_REQUEST", id, sessionToken,
      request: { method, path, ...(body !== undefined ? { body } : {}) },
    }, trustedParentOrigin);
  });
}

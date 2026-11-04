export type AppsScriptDemoUser = {
  sub: string;
  name?: string;
  preferred_username?: string;
  email?: string;
  picture?: string;
};

let currentDemoUser: AppsScriptDemoUser | null = null;
const listeners = new Set<() => void>();

export function getAppsScriptDemoUser(): AppsScriptDemoUser | null {
  return currentDemoUser;
}

export function setAppsScriptDemoUser(user: AppsScriptDemoUser): void {
  if (!user || typeof user.sub !== "string" || !user.sub.startsWith("demo:")) return;
  currentDemoUser = {
    sub: user.sub,
    ...(user.name ? { name: user.name } : {}),
    ...(user.preferred_username ? { preferred_username: user.preferred_username } : {}),
  };
  for (const listener of listeners) listener();
}

export function subscribeToAppsScriptDemoSession(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

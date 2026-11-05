const { test, expect } = require("@playwright/test");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");

const wrapperTemplate = readFileSync(join(__dirname, "../../apps-script/Index.html"), "utf8");
const pagesOrigin = "https://pooya-labs.github.io";
const appUrl = pagesOrigin + "/orbit/?orbitRuntime=apps-script";
const scriptUrl = "https://script.google.com/macros/s/orbit-test/exec";

test("Apps Script HtmlService wrapper completes the cross-origin bridge handshake and API round-trip", async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  const wrapperHtml = wrapperTemplate.replace("<?= appUrl ?>", appUrl);

  await context.addInitScript(() => {
    const diagnostics = { origin: window.location.origin, messages: [], errors: [] };
    Object.defineProperty(window, "__orbitBridgeDiagnostics", { value: diagnostics });
    window.addEventListener("message", (event) => {
      diagnostics.messages.push({
        origin: event.origin,
        fromParent: event.source === window.parent,
        type: event.data && typeof event.data === "object" ? event.data.type : typeof event.data,
      });
    });
    window.addEventListener("error", (event) => diagnostics.errors.push(String(event.message)));

    if (window.location.hostname !== "script.google.com") return;

    const state = { bootstrapCalls: [], apiCalls: [] };
    Object.defineProperty(window, "__orbitScriptMock", { value: state });

    const bootstrapPayload = {
      sessionToken: "browser-smoke-session-token",
      expiresInSeconds: 21600,
      user: { sub: "demo:browser-smoke", name: "ORBIT Browser Smoke", preferred_username: "browser-smoke" },
      workspace: { id: "workspace-browser-smoke", name: "ORBIT Browser Smoke Workspace" },
      mode: "public-demo",
    };

    function createRunner() {
      const handlers = { success: null, failure: null };
      const runner = {
        withSuccessHandler(handler) { handlers.success = handler; return runner; },
        withFailureHandler(handler) { handlers.failure = handler; return runner; },
        orbitBootstrap(clientId) {
          state.bootstrapCalls.push(clientId);
          Promise.resolve().then(() => handlers.success?.(bootstrapPayload));
          return runner;
        },
        orbitApi(request) {
          const captured = JSON.parse(JSON.stringify(request));
          state.apiCalls.push(captured);
          const response = captured.path === "/api/v1/meta"
            ? { status: 200, body: { name: "orbit-api", version: "0.10.0", environment: "apps-script-browser-smoke" } }
            : captured.path === "/api/v1/workspaces"
              ? { status: 200, body: { data: [] } }
              : {
                  status: 200,
                  body: {
                    ok: true,
                    method: captured.method,
                    path: captured.path,
                    sessionToken: captured.sessionToken,
                    body: captured.body ?? null,
                  },
                };
          Promise.resolve().then(() => handlers.success?.(response));
          return runner;
        },
      };
      return runner;
    }

    window.google = { script: {} };
    Object.defineProperty(window.google.script, "run", { get: createRunner });
  });

  await context.route("https://script.google.com/macros/s/orbit-test/exec", (route) =>
    route.fulfill({ status: 200, contentType: "text/html", body: wrapperHtml })
  );
  await context.route(pagesOrigin + "/orbit/**", async (route) => {
    const requested = new URL(route.request().url());
    const localUrl = "http://127.0.0.1:4173" + requested.pathname + requested.search;
    const response = await route.fetch({ url: localUrl });
    await route.fulfill({ response });
  });

  try {
    await page.goto(scriptUrl);
    const wrapperStatus = page.locator("#status");
    await expect(wrapperStatus).toHaveText("Connected · isolated demo workspace", { timeout: 20_000 });

    const frame = page.frameLocator("#orbit-frame");
    try {
      await expect.poll(
        () => page.evaluate(() => window.__orbitScriptMock.apiCalls.filter((call) => call.path === "/api/v1/meta").length),
        { timeout: 8_000 },
      ).toBe(1);
      await expect(frame.locator(".last-updated")).toContainText("apps-script-browser-smoke", { timeout: 8_000 });
    } catch (error) {
      const parentDiagnostics = await page.evaluate(() => ({
        url: window.location.href,
        state: window.__orbitScriptMock,
        messages: window.__orbitBridgeDiagnostics,
      }));
      const frameDiagnostics = await frame.locator("body").evaluate(() => window.__orbitBridgeDiagnostics);
      throw new Error("Apps Script bridge diagnostics: " + JSON.stringify({ parentDiagnostics, frameDiagnostics }) + "\\n" + String(error));
    }

    const bootstrapCalls = await page.evaluate(() => window.__orbitScriptMock.bootstrapCalls);
    expect(bootstrapCalls).toHaveLength(1);
    expect(bootstrapCalls[0]).toMatch(/^[a-f0-9-]{36}$/i);

    const metadataCalls = await page.evaluate(() =>
      window.__orbitScriptMock.apiCalls.filter((call) => call.path === "/api/v1/meta")
    );
    expect(metadataCalls).toHaveLength(1);
    expect(metadataCalls[0].sessionToken).toBe("browser-smoke-session-token");

    const roundTrip = await frame.locator("body").evaluate(() => new Promise((resolve, reject) => {
      const id = "apps-script-browser-smoke-write";
      const timeout = window.setTimeout(() => {
        window.removeEventListener("message", onMessage);
        reject(new Error("Timed out waiting for the Apps Script bridge response."));
      }, 8_000);
      function onMessage(event) {
        if (event.source !== window.parent || !event.data || event.data.type !== "ORBIT_BRIDGE_RESPONSE" || event.data.id !== id) return;
        window.clearTimeout(timeout);
        window.removeEventListener("message", onMessage);
        resolve(event.data.response);
      }
      window.addEventListener("message", onMessage);
      window.parent.postMessage({
        type: "ORBIT_BRIDGE_REQUEST",
        id,
        sessionToken: "browser-smoke-session-token",
        request: {
          method: "POST",
          path: "/api/v1/workspaces/workspace-browser-smoke/incidents",
          body: { title: "Bridge round-trip smoke", severity: "low" },
        },
      }, "https://script.google.com");
    }));

    expect(roundTrip).toEqual({
      status: 200,
      body: {
        ok: true,
        method: "POST",
        path: "/api/v1/workspaces/workspace-browser-smoke/incidents",
        sessionToken: "browser-smoke-session-token",
        body: { title: "Bridge round-trip smoke", severity: "low" },
      },
    });

    const writeCalls = await page.evaluate(() =>
      window.__orbitScriptMock.apiCalls.filter((call) => call.method === "POST")
    );
    expect(writeCalls).toHaveLength(1);
    expect(writeCalls[0].sessionToken).toBe("browser-smoke-session-token");
    expect(writeCalls[0].body).toEqual({ title: "Bridge round-trip smoke", severity: "low" });
  } finally {
    await context.close();
  }
});

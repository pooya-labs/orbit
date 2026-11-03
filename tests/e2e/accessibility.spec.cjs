const { test, expect } = require("@playwright/test");
const { AxeBuilder } = require("@axe-core/playwright");
const packageVersion = require("../../package.json").version;

test.beforeEach(async ({ page }) => {
  await page.goto("./");
});

test("footer version stays in sync with the application package version", async ({ page }) => {
  await expect(page.locator(".footer-version")).toHaveText(`v${packageVersion}`);
});

// The sidebar animates color/background on theme and route changes. Axe must
// inspect the settled UI, not an intermediate animation frame whose blended
// colors can sit just below the 4.5:1 text threshold.
async function waitForNavigationTransitions(navigation) {
  await navigation.evaluate(async (element) => {
    // Flush style changes before querying the Web Animations API.
    void getComputedStyle(element).color;
    const nodes = [element, ...element.querySelectorAll("*")];
    const transitions = nodes
      .flatMap((node) => node.getAnimations())
      .filter((animation) => (
        animation.playState === "running"
        && animation.effect?.getTiming().iterations !== Infinity
      ));
    await Promise.all(transitions.map((animation) => animation.finished.catch(() => undefined)));
  });
}

async function expectNoAxeViolations(page, label) {
  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();
  const violations = results.violations.map((violation) => ({
    id: violation.id,
    impact: violation.impact,
    help: violation.help,
    nodes: violation.nodes.map((node) => ({
      target: node.target,
      summary: node.failureSummary,
    })),
  }));
  expect(violations, `${label}: ${JSON.stringify(violations, null, 2)}`).toEqual([]);
}

test("keyboard users can reveal the skip link and reach the main landmark", async ({ page }) => {
  const skipLink = page.getByRole("link", { name: "Skip to main content" });
  const before = await skipLink.evaluate((element) => element.getBoundingClientRect().top);

  await page.keyboard.press("Tab");
  await expect(skipLink).toBeFocused();
  const after = await skipLink.evaluate((element) => element.getBoundingClientRect().top);
  expect(after).toBeGreaterThan(before);

  await page.keyboard.press("Enter");
  await expect(page.locator("main#main-content")).toBeFocused();
  expect(page.url()).toContain("#main-content");
});

test("primary navigation updates the active page and its announced heading", async ({ page }) => {
  const navigation = page.getByRole("navigation", { name: "Primary navigation" });
  const metrics = navigation.getByRole("button", { name: "Metrics", exact: true });

  await metrics.click();

  await expect(page.getByRole("heading", { level: 1, name: "Metrics" })).toBeVisible();
  await expect(metrics).toHaveAttribute("aria-current", "page");
});

test("Overview chart table follows the selected time range", async ({ page }) => {
  const range = page.getByRole("group", { name: "Time range" });
  const sixHours = range.getByRole("button", { name: "6 hours" });

  await sixHours.click();
  await expect(sixHours).toHaveAttribute("aria-pressed", "true");

  const disclosure = page.locator("details.overview-chart-data-table");
  await disclosure.locator("summary").click();
  const table = disclosure.getByRole("table");
  await expect(table).toBeVisible();
  await expect(table.locator("caption")).toContainText("selected 6 hours");
  await expect(table.locator("thead th")).toHaveCount(2);
  await expect(table.locator("tbody tr").first()).toBeVisible();
  await expect(page.locator(".chart-x-labels span")).toHaveCount(7);
});

test("mobile navigation hides closed controls and traps/restores keyboard focus", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  // Load at the mobile breakpoint instead of testing while the desktop
  // sidebar is still completing its responsive exit transition.
  await page.reload();

  const skipLink = page.getByRole("link", { name: "Skip to main content" });
  const menu = page.getByRole("button", { name: "Open navigation" });
  await expect(menu).toBeVisible();

  await page.keyboard.press("Tab");
  await expect(skipLink).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(menu).toBeFocused();

  await menu.click();
  await expect(menu).toHaveAttribute("aria-expanded", "true");

  const close = page.getByRole("button", { name: "Close navigation" });
  const profileLink = page.locator("aside.sidebar").getByRole("link", { name: "@pooya16pg-spec" });
  await expect(close).toBeFocused();

  await page.keyboard.press("Shift+Tab");
  await expect(profileLink).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(close).toBeFocused();

  await page.keyboard.press("Escape");
  await expect(menu).toHaveAttribute("aria-expanded", "false");
  await expect(menu).toBeFocused();
});

test("mobile navigation closes after route selection and returns focus to its trigger", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const menu = page.getByRole("button", { name: "Open navigation" });
  await menu.click();

  const metrics = page.getByRole("navigation", { name: "Primary navigation" })
    .getByRole("button", { name: "Metrics", exact: true });
  await metrics.click();

  await expect(page.getByRole("heading", { level: 1, name: "Metrics" })).toBeVisible();
  await expect(menu).toHaveAttribute("aria-expanded", "false");
  await expect(menu).toBeFocused();
});

test("theme and language controls update visible theme and RTL direction", async ({ page }) => {
  await page.getByRole("button", { name: "Switch to light theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

  await page.getByRole("button", { name: "فارسی" }).click();
  await expect(page.locator(".app-shell")).toHaveAttribute("dir", "rtl");
  await expect(page.locator(".app-shell")).toHaveAttribute("lang", "fa");
  await expect(page.getByRole("link", { name: "رفتن به محتوای اصلی" })).toBeAttached();
});

test("Metrics chart table renders Prometheus samples and updates with the API time window", async ({ page }) => {
  const now = Math.floor(Date.now() / 1000);
  const requestedMetrics = [];
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const json = (body) => route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body),
    });

    if (url.pathname === "/api/v1/meta") {
      return json({ name: "orbit-api", version: "0.10.0", environment: "development" });
    }
    if (url.pathname === "/api/v1/workspaces") {
      return json({ data: [] });
    }
    if (url.pathname === "/api/v1/telemetry/services") {
      return json({
        source: "telemetry_backends",
        window: url.searchParams.get("window") || "1h",
        queried_at: new Date().toISOString(),
        sources_queried: ["prometheus"],
        has_more: false,
        data: [{ name: "orbit-telemetry-demo", sources: ["prometheus"] }],
      });
    }
    if (url.pathname === "/api/v1/telemetry/services/orbit-telemetry-demo/attributes") {
      return json({
        service: "orbit-telemetry-demo",
        window: url.searchParams.get("window") || "1h",
        queried_at: new Date().toISOString(),
        sources_queried: ["prometheus"],
        attributes: {},
        attribute_sources: {},
      });
    }
    if (url.pathname === "/api/v1/telemetry/metrics") {
      const window = url.searchParams.get("window") || "1h";
      const metric = url.searchParams.get("metric") || "request_rate";
      requestedMetrics.push(metric);
      const values = (offset) => [
        [now - 120, String(12.5 + offset)],
        [now - 60, String(14.2 + offset)],
        [now, String(13.9 + offset)],
      ];
      return json({
        source: "prometheus",
        window,
        queried_at: new Date().toISOString(),
        query: metric === "failed_request_rate"
          ? 'sum(rate(orbit_demo_requests_total{slo_outcome="bad"}[5m]))'
          : metric === "successful_request_rate"
            ? 'sum(rate(orbit_demo_requests_total{slo_outcome="good"}[5m]))'
            : "sum(rate(orbit_demo_requests_total[5m]))",
        metric,
        step: "60s",
        service: "orbit-telemetry-demo",
        group_by: ["instance"],
        data: [
          {
            metric: { instance: "demo-1", service_version: "1.0.0", deployment_environment: "ci" },
            values: values(0),
          },
          {
            metric: { instance: "demo-2", service_version: "1.0.0", deployment_environment: "ci" },
            values: values(2),
          },
        ],
      });
    }
    return json({ error: { code: "not_found", message: "Unhandled browser-test endpoint", request_id: "playwright" } });
  });

  await page.getByRole("button", { name: "Connect a source" }).click();
  await page.getByLabel("API base URL").fill("http://127.0.0.1:4173");
  await page.getByRole("button", { name: "Test and connect" }).click();

  const navigation = page.getByRole("navigation", { name: "Primary navigation" });
  await navigation.getByRole("button", { name: "Metrics", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Metrics" })).toBeVisible();

  const metricSelector = page.getByLabel("Metric");
  await expect(metricSelector).toHaveValue("request_rate");
  await expect(metricSelector.locator("option")).toHaveCount(3);
  const failedMetricRequest = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return url.pathname === "/api/v1/telemetry/metrics" && url.searchParams.get("metric") === "failed_request_rate";
  });
  await metricSelector.selectOption("failed_request_rate");
  await failedMetricRequest;
  await expect.poll(() => requestedMetrics.at(-1)).toBe("failed_request_rate");
  await expect(page.locator(".telemetry-stat-row .telemetry-kicker").first()).toHaveText("FAILED REQUEST RATE");

  const disclosure = page.locator("details.telemetry-data-table");
  await expect(disclosure).toBeVisible();
  await disclosure.locator("summary").click();

  const table = disclosure.getByRole("table");
  await expect(table.locator("caption")).toContainText("FAILED REQUEST RATE samples");
  await expect(table.locator("caption")).toContainText("orbit-telemetry-demo");
  await expect(table.locator("caption")).toContainText("1h");
  await expect(table.locator("thead th")).toHaveCount(3);
  await expect(table.locator("tbody tr")).toHaveCount(6);

  const sixHours = page.getByRole("group", { name: "Window" }).getByRole("button", { name: "6 hours" });
  await sixHours.click();
  await expect(sixHours).toHaveAttribute("aria-pressed", "true");
  await expect(disclosure).toBeVisible();
  await disclosure.locator("summary").click();
  await expect(disclosure).toHaveAttribute("open", "");
  await expect(table.locator("caption")).toContainText("6h");
  await expect(table.locator("tbody tr")).toHaveCount(6);

  const metricsAxe = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();
  const metricsViolations = metricsAxe.violations.map((violation) => ({
    id: violation.id,
    impact: violation.impact,
    help: violation.help,
    nodes: violation.nodes.map((node) => ({
      target: node.target,
      summary: node.failureSummary,
    })),
  }));
  expect(metricsViolations, JSON.stringify(metricsViolations, null, 2)).toEqual([]);
});

test("staging telemetry requires workspace authentication and never calls legacy global endpoints", async ({ page }) => {
  const legacyRequests = [];
  const workspaceRequests = [];
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const json = (body, status = 200) => route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(body),
    });

    if (url.pathname === "/api/v1/meta") {
      return json({ name: "orbit-api", version: "0.10.0", environment: "staging" });
    }
    if (url.pathname === "/api/v1/workspaces") {
      workspaceRequests.push(url.pathname);
      return json({ data: [] });
    }
    if (url.pathname.startsWith("/api/v1/telemetry/")) {
      legacyRequests.push(url.pathname);
      return json({ error: { code: "legacy_route_disabled", message: "Unscoped telemetry reads are disabled." } }, 404);
    }
    return json({ error: { code: "not_found", message: "Unhandled browser-test endpoint" } }, 404);
  });

  await page.getByRole("button", { name: "Connect a source" }).click();
  await page.getByLabel("API base URL").fill("https://api.example.test");
  await page.getByRole("button", { name: "Test and connect" }).click();

  const navigation = page.getByRole("navigation", { name: "Primary navigation" });
  for (const view of ["Metrics", "Logs", "Traces"]) {
    await navigation.getByRole("button", { name: view, exact: true }).click();
    await expect(page.getByRole("heading", { level: 1, name: view })).toBeVisible();
    await expect(page.locator(".incident-inline-notice")).toContainText(
      "Browser sign-in is not configured for this deployment. Workspace telemetry is unavailable.",
    );
  }

  await expect.poll(() => legacyRequests.length).toBe(0);
  await expect.poll(() => workspaceRequests.length).toBe(0);
});

test("axe finds no WCAG 2.1 A/AA or WCAG 2.2 AA violations on Overview", async ({ page }) => {
  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();

  const violations = results.violations.map((violation) => ({
    id: violation.id,
    impact: violation.impact,
    help: violation.help,
    nodes: violation.nodes.map((node) => ({
      target: node.target,
      summary: node.failureSummary,
    })),
  }));

  expect(violations, JSON.stringify(violations, null, 2)).toEqual([]);
});

test("axe finds no WCAG 2.1 A/AA or WCAG 2.2 AA violations in the open mobile drawer", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Open navigation" }).click();
  await expect(page.getByRole("button", { name: "Close navigation" })).toBeFocused();

  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();

  const violations = results.violations.map((violation) => ({
    id: violation.id,
    impact: violation.impact,
    help: violation.help,
    nodes: violation.nodes.map((node) => ({
      target: node.target,
      summary: node.failureSummary,
    })),
  }));

  expect(violations, JSON.stringify(violations, null, 2)).toEqual([]);
});


test("axe finds no WCAG 2.1 A/AA or WCAG 2.2 AA violations in the light theme", async ({ page }) => {
  await page.getByRole("button", { name: "Switch to light theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();

  const violations = results.violations.map((violation) => ({
    id: violation.id,
    impact: violation.impact,
    help: violation.help,
    nodes: violation.nodes.map((node) => ({
      target: node.target,
      summary: node.failureSummary,
    })),
  }));

  expect(violations, JSON.stringify(violations, null, 2)).toEqual([]);
});

test("all primary dashboard views expose an announced heading and pass axe in dark and light themes", async ({ page }) => {
  test.setTimeout(120_000);
  const labels = [
    "Service map",
    "Services",
    "Infrastructure",
    "Metrics",
    "Traces",
    "Logs",
    "Incidents",
    "Deployments",
    "Alert rules",
    "Notifications",
    "SLOs",
    "Audit log",
  ];
  const navigation = page.getByRole("navigation", { name: "Primary navigation" });

  for (const theme of ["dark", "light"]) {
    if (theme === "light") {
      await page.getByRole("button", { name: "Switch to light theme" }).click();
      await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
      await waitForNavigationTransitions(navigation);
    }

    for (const label of labels) {
      const item = navigation.getByRole("button", { name: label, exact: true });
      await item.click();
      await expect(item).toHaveAttribute("aria-current", "page");
      await expect(page.getByRole("heading", { level: 1, name: label, exact: true })).toBeVisible();
      await waitForNavigationTransitions(navigation);

      const results = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
        .analyze();

      const violations = results.violations.map((violation) => ({
        id: violation.id,
        impact: violation.impact,
        help: violation.help,
        nodes: violation.nodes.map((node) => ({
          target: node.target,
          summary: node.failureSummary,
        })),
      }));

      expect(
        violations,
        `Accessibility violations on ${label} in ${theme} theme: ${JSON.stringify(violations, null, 2)}`,
      ).toEqual([]);
    }
  }
});

test("invalid API base paths are announced as an accessible error", async ({ page }) => {
  await page.getByRole("button", { name: "Connect a source" }).click();
  const apiUrl = page.getByRole("textbox", { name: "API base URL" });
  await apiUrl.fill("https://example.com/orbit/api");
  await page.getByRole("button", { name: "Test and connect" }).click();

  const alert = page.locator(".source-error[role=\"alert\"]");
  await expect(alert).toBeVisible();
  await expect(alert).toContainText("Enter an API base URL");
  await expect(apiUrl).toHaveValue("https://example.com/orbit/api");

  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(
    results.violations.map((violation) => ({ id: violation.id, targets: violation.nodes.map((node) => node.target) })),
  ).toEqual([]);
});

test("reduced-motion preference disables smooth scrolling and long transitions", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });

  const motion = await page.evaluate(() => ({
    preference: window.matchMedia("(prefers-reduced-motion: reduce)").matches,
    scrollBehavior: getComputedStyle(document.documentElement).scrollBehavior,
    transitionDuration: getComputedStyle(document.querySelector(".theme-toggle")).transitionDuration,
    animationDuration: getComputedStyle(document.querySelector(".nav-item")).animationDuration,
  }));

  expect(motion.preference).toBe(true);
  expect(motion.scrollBehavior).toBe("auto");
  expect(parseFloat(motion.transitionDuration)).toBeLessThanOrEqual(0.00002);
  expect(parseFloat(motion.animationDuration)).toBeLessThanOrEqual(0.00002);
});

test("Overview reflows at a 320 CSS-pixel viewport without page-level horizontal overflow", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 900 });
  await page.reload();

  const dimensions = await page.evaluate(() => {
    const viewport = document.documentElement.clientWidth;
    const table = document.querySelector(".overview-chart-data-table table");
    const tableChain = [];
    for (let element = table; element && element !== document.documentElement; element = element.parentElement) {
      const style = getComputedStyle(element);
      const rect = element.getBoundingClientRect();
      tableChain.push({
        tag: element.tagName.toLowerCase(),
        className: typeof element.className === "string" ? element.className : "",
        left: Math.round(rect.left),
        right: Math.round(rect.right),
        width: Math.round(rect.width),
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
        minWidth: style.minWidth,
        maxWidth: style.maxWidth,
        widthStyle: style.width,
        overflowX: style.overflowX,
        display: style.display,
        boxSizing: style.boxSizing,
      });
    }
    const overflow = Array.from(document.querySelectorAll("body *"))
      .filter((element) => !element.closest(".telemetry-data-table-scroll"))
      .map((element) => {
        const rect = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        return {
          tag: element.tagName.toLowerCase(),
          id: element.id,
          className: typeof element.className === "string" ? element.className : "",
          text: (element.textContent || "").trim().slice(0, 80),
          left: Math.round(rect.left),
          right: Math.round(rect.right),
          width: Math.round(rect.width),
          position: style.position,
          display: style.display,
        };
      })
      .filter((item) => item.display !== "none" && item.width > 0 && item.right > viewport + 1)
      .sort((a, b) => b.right - a.right)
      .slice(0, 30);
    return {
      viewport,
      document: document.documentElement.scrollWidth,
      body: document.body.scrollWidth,
      tableChain,
      overflow,
    };
  });
  expect(dimensions.document, JSON.stringify(dimensions, null, 2)).toBeLessThanOrEqual(dimensions.viewport + 1);
  expect(dimensions.body, JSON.stringify(dimensions, null, 2)).toBeLessThanOrEqual(dimensions.viewport + 1);

  const menu = page.getByRole("button", { name: "Open navigation" });
  await expect(menu).toBeVisible();
  await menu.click();
  await expect(page.getByRole("button", { name: "Close navigation" })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(menu).toBeFocused();

  const disclosure = page.locator("details.overview-chart-data-table");
  await disclosure.locator("summary").click();
  const table = disclosure.getByRole("table");
  await expect(table).toBeVisible();
  const tableDimensions = await disclosure.locator(".telemetry-data-table-scroll").evaluate((element) => ({
    viewport: element.clientWidth,
    content: element.scrollWidth,
  }));
  expect(tableDimensions.viewport).toBeLessThanOrEqual(dimensions.viewport);
  expect(tableDimensions.content).toBeGreaterThanOrEqual(tableDimensions.viewport);

  const scrollRegion = disclosure.getByRole("region", {
    name: /Illustrative request-rate samples across the selected/,
  });
  await expect(scrollRegion).toHaveAttribute("tabindex", "0");
  await scrollRegion.evaluate((element) => { element.scrollLeft = 0; });
  await scrollRegion.focus();
  await expect(scrollRegion).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect.poll(() => scrollRegion.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);

  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(
    results.violations.map((violation) => ({ id: violation.id, targets: violation.nodes.map((node) => node.target) })),
  ).toEqual([]);
});



test("axe covers keyboard-focus and hover navigation states in both themes", async ({ page }) => {
  for (const theme of ["dark", "light"]) {
    // Set the saved preference before a fresh document load. This ensures
    // each theme starts with the browser's natural body focus, not the
    // previously clicked theme toggle or a programmatically focused link.
    await page.evaluate((value) => window.localStorage.setItem("orbit.theme", value), theme);
    await page.reload();
    await expect(page.locator(".app-shell")).toHaveAttribute("data-theme", theme);

    const navigation = page.getByRole("navigation", { name: "Primary navigation" });
    const skipLink = page.getByRole("link", { name: "Skip to main content" });
    const overview = navigation.getByRole("button", { name: "Overview", exact: true });
    const services = navigation.getByRole("button", { name: "Services", exact: true });

    // Use real sequential Tab presses from the fresh document: the first
    // tabbable element is the skip link, followed by the active Overview
    // navigation item. This tests actual keyboard modality and focus order.
    await page.keyboard.press("Tab");
    await expect(skipLink).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(overview).toBeFocused();
    expect(await overview.evaluate((element) => element.matches(":focus-visible"))).toBe(true);

    await services.hover();
    await waitForNavigationTransitions(navigation);
    await expectNoAxeViolations(page, `Primary navigation focus + hover in ${theme} theme`);
  }
});

test("axe covers the API source connecting, disabled-submit, and request-error states", async ({ page }) => {
  let releaseMetadata;
  const metadataGate = new Promise((resolve) => {
    releaseMetadata = resolve;
  });

  await page.route("**/api/v1/**", async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname === "/api/v1/meta") {
      await metadataGate;
      return route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "temporarily_unavailable",
            message: "The API is temporarily unavailable.",
            request_id: "playwright-a11y-loading",
          },
        }),
      });
    }
    if (pathname === "/api/v1/workspaces") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: [] }),
      });
    }
    return route.continue();
  });

  await page.getByRole("button", { name: "Connect a source" }).click();
  await page.getByLabel("API base URL").fill("http://127.0.0.1:4173");

  const metadataRequest = page.waitForRequest((request) => (
    new URL(request.url()).pathname === "/api/v1/meta"
  ));
  const submit = page.locator(".source-setup-actions button[type=\"submit\"]");
  await submit.click();
  await metadataRequest;

  await expect(submit).toBeDisabled();
  await expect(submit).toContainText("Connecting");
  await expect(page.getByRole("heading", { name: "Connect an ORBIT API" })).toBeVisible();
  await expectNoAxeViolations(page, "API source connecting with submit disabled");

  // Release the held request only after the in-flight UI has been inspected.
  releaseMetadata();
  const error = page.locator(".source-error[role=\"alert\"]");
  await expect(error).toBeVisible();
  await expect(error).toContainText("temporarily unavailable");
  await expect(submit).toBeEnabled();
  await expectNoAxeViolations(page, "API source request-error state");
});

test("keyboard users can reach and activate every primary view in natural tab order", async ({ page }) => {
  const labels = [
    "Overview",
    "Service map",
    "Services",
    "Infrastructure",
    "Metrics",
    "Traces",
    "Logs",
    "Incidents",
    "Deployments",
    "Alert rules",
    "Notifications",
    "SLOs",
    "Audit log",
  ];
  const navigation = page.getByRole("navigation", { name: "Primary navigation" });
  const skipLink = page.getByRole("link", { name: "Skip to main content" });

  // The skip link is first. Continue from the document's natural Tab order;
  // never programmatically focus a navigation item to simulate keyboard use.
  await page.keyboard.press("Tab");
  await expect(skipLink).toBeFocused();

  for (const label of labels) {
    await page.keyboard.press("Tab");
    const item = navigation.getByRole("button", { name: label, exact: true });

    await expect(item).toBeFocused();
    expect(await item.evaluate((element) => element.matches(":focus-visible"))).toBe(true);

    await page.keyboard.press("Enter");
    await expect(item).toHaveAttribute("aria-current", "page");
    const headingLabel = label === "Overview" ? "Good morning, team." : label;
    await expect(page.getByRole("heading", { level: 1, name: headingLabel, exact: true })).toBeVisible();
  }
});


test("Chromium accessibility tree exposes NVDA-style heading and landmark navigation", async ({ page }) => {
  // NVDA's H/D navigation is driven by the browser's accessibility tree.
  // This diagnostic inspects the same Chromium AX tree directly, rather
  // than inferring screen-reader support from source markup or axe alone.
  await page.waitForLoadState("domcontentloaded");
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Accessibility.enable");
  const { nodes } = await cdp.send("Accessibility.getFullAXTree");

  const propertyValue = (node, propertyName) => (
    node.properties?.find((property) => property.name === propertyName)?.value?.value
  );
  const exposed = nodes
    .filter((node) => !node.ignored)
    .filter((node) => ["heading", "navigation", "main"].includes(node.role?.value))
    .map((node) => ({
      role: node.role.value,
      name: node.name?.value ?? "",
      level: propertyValue(node, "level") ?? null,
    }));

  console.log("ORBIT Chromium accessibility-tree diagnostic:", JSON.stringify(exposed, null, 2));

  const headings = exposed.filter((node) => node.role === "heading");
  const navigation = exposed.find((node) => node.role === "navigation" && node.name === "Primary navigation");
  const main = exposed.find((node) => node.role === "main");
  const pageTitle = headings.find((node) => node.level === 1 && node.name === "Good morning, team.");
  const sidebarGroups = headings.filter((node) => node.level === 2).map((node) => node.name);

  expect(navigation, "Named primary navigation must exist in Chromium's accessibility tree").toBeTruthy();
  expect(main, "Main content landmark must exist in Chromium's accessibility tree").toBeTruthy();
  expect(pageTitle, "The visible page H1 must be exposed as a level-one heading").toBeTruthy();
  expect(sidebarGroups).toEqual(expect.arrayContaining(["OBSERVE", "INVESTIGATE", "DELIVER"]));

  // Report the exact keyboard-reader targets discovered by Chromium.
  console.log("ORBIT heading navigation targets:", JSON.stringify(headings, null, 2));
});


test("every primary view exposes its current H1 in Chromium's accessibility tree", async ({ page }) => {
  const views = [
    { label: "Overview", heading: "Good morning, team." },
    { label: "Service map", heading: "Service map" },
    { label: "Services", heading: "Services" },
    { label: "Infrastructure", heading: "Infrastructure" },
    { label: "Metrics", heading: "Metrics" },
    { label: "Traces", heading: "Traces" },
    { label: "Logs", heading: "Logs" },
    { label: "Incidents", heading: "Incidents" },
    { label: "Deployments", heading: "Deployments" },
    { label: "Alert rules", heading: "Alert rules" },
    { label: "Notifications", heading: "Notifications" },
    { label: "SLOs", heading: "SLOs" },
    { label: "Audit log", heading: "Audit log" },
  ];

  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Accessibility.enable");
  const navigation = page.getByRole("navigation", { name: "Primary navigation" });
  const report = [];

  for (const view of views) {
    await navigation.getByRole("button", { name: view.label, exact: true }).click();
    await expect(page.getByRole("heading", { level: 1, name: view.heading, exact: true })).toBeVisible();

    // Read Chromium's accessibility tree after the client-side view transition.
    // This checks the semantic tree exposed to assistive technology, not only
    // React/DOM text or a successful visual render.
    const { nodes } = await cdp.send("Accessibility.getFullAXTree");
    const propertyValue = (node, propertyName) => (
      node.properties?.find((property) => property.name === propertyName)?.value?.value
    );
    const exposed = nodes.filter((node) => !node.ignored);
    const headings = exposed
      .filter((node) => node.role?.value === "heading")
      .map((node) => ({
        name: node.name?.value ?? "",
        level: propertyValue(node, "level") ?? null,
      }));
    const currentH1 = headings.filter((node) => node.level === 1 && node.name === view.heading);
    const main = exposed.find((node) => node.role?.value === "main");
    const primaryNavigation = exposed.find((node) => (
      node.role?.value === "navigation" && node.name?.value === "Primary navigation"
    ));

    expect(currentH1, `${view.label}: current H1 must be exposed as a level-one AX heading; found ${JSON.stringify(headings)}`).toHaveLength(1);
    expect(main, `${view.label}: main landmark must remain exposed`).toBeTruthy();
    expect(primaryNavigation, `${view.label}: named primary navigation must remain exposed`).toBeTruthy();

    report.push({ view: view.label, currentH1: currentH1[0], headings });
  }

  console.log("ORBIT all-view Chromium accessibility-tree report:", JSON.stringify(report, null, 2));
});

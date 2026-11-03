const { test, expect } = require("@playwright/test");
const { AxeBuilder } = require("@axe-core/playwright");

test.beforeEach(async ({ page }) => {
  await page.goto("./");
});

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

  const skipLink = page.getByRole("link", { name: "Skip to main content" });
  const menu = page.getByRole("button", { name: "Open navigation" });

  await page.keyboard.press("Tab");
  await expect(skipLink).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(menu).toBeFocused();

  await menu.click();
  await expect(menu).toHaveAttribute("aria-expanded", "true");

  const close = page.getByRole("button", { name: "Close navigation" });
  const profileLink = page.getByRole("link", { name: "@pooya16pg-spec" });
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
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const json = (body) => route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body),
    });

    if (url.pathname === "/api/v1/meta") {
      return json({ name: "orbit-api", version: "0.10.0", environment: "playwright" });
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
      const values = (offset) => [
        [now - 120, String(12.5 + offset)],
        [now - 60, String(14.2 + offset)],
        [now, String(13.9 + offset)],
      ];
      return json({
        source: "prometheus",
        window,
        queried_at: new Date().toISOString(),
        query: "sum(rate(orbit_demo_requests_total[5m]))",
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

  await page.getByRole("button", { name: "Connect source" }).click();
  await page.getByLabel("API base URL").fill("http://127.0.0.1:4173");
  await page.getByRole("button", { name: "Check & connect" }).click();

  const navigation = page.getByRole("navigation", { name: "Primary navigation" });
  await navigation.getByRole("button", { name: "Metrics", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Metrics" })).toBeVisible();

  const disclosure = page.locator("details.telemetry-data-table");
  await expect(disclosure).toBeVisible();
  await disclosure.locator("summary").click();

  const table = disclosure.getByRole("table");
  await expect(table.locator("caption")).toContainText("orbit-telemetry-demo");
  await expect(table.locator("caption")).toContainText("1h");
  await expect(table.locator("thead th")).toHaveCount(3);
  await expect(table.locator("tbody tr")).toHaveCount(6);

  const sixHours = page.getByRole("group", { name: "Window" }).getByRole("button", { name: "6 hours" });
  await sixHours.click();
  await expect(sixHours).toHaveAttribute("aria-pressed", "true");
  await expect(table.locator("caption")).toContainText("6h");
  await expect(table.locator("tbody tr")).toHaveCount(6);
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

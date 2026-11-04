import assert from "node:assert/strict";
import { createHash, randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const serverCode = readFileSync(new URL("../apps-script/Code.js", import.meta.url), "utf8");

class FakeRange {
  constructor(sheet, row, column, rowCount, columnCount) {
    this.sheet = sheet;
    this.row = row - 1;
    this.column = column - 1;
    this.rowCount = rowCount;
    this.columnCount = columnCount;
  }
  getValues() {
    return Array.from({ length: this.rowCount }, (_, rowOffset) => {
      const source = this.sheet.rows[this.row + rowOffset] || [];
      return Array.from({ length: this.columnCount }, (_, columnOffset) =>
        source[this.column + columnOffset] === undefined ? "" : source[this.column + columnOffset]);
    });
  }
  setValues(values) {
    values.forEach((row, rowOffset) => {
      const targetIndex = this.row + rowOffset;
      while (this.sheet.rows.length <= targetIndex) this.sheet.rows.push([]);
      const target = this.sheet.rows[targetIndex];
      row.forEach((value, columnOffset) => {
        target[this.column + columnOffset] = value;
      });
    });
    return this;
  }
  setFontWeight() { return this; }
}
class FakeSheet {
  constructor(name) { this.name = name; this.rows = []; }
  getName() { return this.name; }
  setName(name) { this.name = name; return this; }
  getLastRow() { return this.rows.length; }
  getRange(row, column, rowCount = 1, columnCount = 1) {
    return new FakeRange(this, row, column, rowCount, columnCount);
  }
  setFrozenRows() { return this; }
  setColumnWidths() { return this; }
  setColumnWidth() { return this; }
  appendRow(values) { this.rows.push(values.slice()); return this; }
  deleteRow(row) { this.rows.splice(row - 1, 1); return this; }
}
class FakeSpreadsheet {
  constructor(id) {
    this.id = id;
    this.sheets = [new FakeSheet("Sheet1")];
  }
  getId() { return this.id; }
  getUrl() { return "https://docs.google.com/spreadsheets/d/" + this.id + "/edit"; }
  getSheetByName(name) { return this.sheets.find((sheet) => sheet.getName() === name) || null; }
  getSheets() { return this.sheets; }
  insertSheet(name) {
    const sheet = new FakeSheet(name || "Sheet" + (this.sheets.length + 1));
    this.sheets.push(sheet);
    return sheet;
  }
}
const properties = new Map();
const spreadsheets = new Map();
const cache = new Map();
let nextSpreadsheet = 0;
let lockHeld = false;
const scriptProperties = {
  getProperty(key) { return properties.has(key) ? properties.get(key) : null; },
  setProperty(key, value) { properties.set(key, String(value)); return this; },
  deleteProperty(key) { properties.delete(key); return this; },
};
const scriptCache = {
  get(key) { return cache.has(key) ? cache.get(key) : null; },
  put(key, value) { cache.set(key, String(value)); return this; },
  remove(key) { cache.delete(key); return this; },
};
const scriptLock = {
  waitLock() {
    if (lockHeld) throw new Error("Lock is already held in test harness.");
    lockHeld = true;
  },
  releaseLock() { lockHeld = false; },
  hasLock() { return lockHeld; },
};
const sandbox = {
  PropertiesService: { getScriptProperties: () => scriptProperties },
  SpreadsheetApp: {
    create(name) {
      const id = "fake-sheet-" + (++nextSpreadsheet);
      const spreadsheet = new FakeSpreadsheet(id);
      spreadsheet.name = name;
      spreadsheets.set(id, spreadsheet);
      return spreadsheet;
    },
    openById(id) {
      const spreadsheet = spreadsheets.get(id);
      if (!spreadsheet) throw new Error("Unknown spreadsheet " + id);
      return spreadsheet;
    },
  },
  CacheService: { getScriptCache: () => scriptCache },
  LockService: { getScriptLock: () => scriptLock },
  Utilities: {
    DigestAlgorithm: { SHA_256: "sha256" },
    Charset: { UTF_8: "utf8" },
    computeDigest(_algorithm, value) {
      return Array.from(createHash("sha256").update(String(value), "utf8").digest());
    },
    newBlob(value) {
      return { getBytes: () => Buffer.from(String(value), "utf8") };
    },
    getUuid: () => randomUUID(),
  },
  Logger: { log() {} },
  HtmlService: {
    createTemplateFromFile(name) {
      assert.equal(name, "Index");
      return {
        appUrl: "",
        evaluate() {
          return {
            setTitle() { return this; },
            addMetaTag() { return this; },
          };
        },
      };
    },
  },
};
vm.createContext(sandbox);
vm.runInContext(serverCode, sandbox, { filename: "apps-script/Code.js" });

assert.equal(typeof sandbox.setupOrbitDemo, "undefined", "owner setup must not be exposed as a public google.script.run function");
const html = sandbox.doGet();
assert.equal(typeof html.addMetaTag, "function", "doGet returns an HtmlOutput-like object");
const setup = sandbox.setupOrbitDemo_();
assert.match(setup.spreadsheetUrl, /^https:\/\/docs\.google\.com\/spreadsheets\/d\//);
const spreadsheet = spreadsheets.get(setup.spreadsheetId);
assert.ok(spreadsheet, "setup creates a private spreadsheet");
assert.equal(spreadsheet.getSheetByName("records")?.getLastRow(), 1, "default sheet becomes the records sheet");
const records = spreadsheet.getSheetByName("records");

const profileA = "11111111-1111-4111-8111-111111111111";
const profileB = "22222222-2222-4222-8222-222222222222";
const sessionA = sandbox.orbitBootstrap(profileA);
const sessionB = sandbox.orbitBootstrap(profileB);
assert.notEqual(sessionA.workspace.id, sessionB.workspace.id, "browser profiles get different workspaces");
assert.equal(sessionA.mode, "public-demo");
const visitorIdForLimit = "33333333-3333-4333-8333-333333333333";
const usageKey = "ORBIT_DEMO_DAILY_USAGE";
properties.set(usageKey, JSON.stringify({
  day: new Date().toISOString().slice(0, 10),
  bootstraps: 25, requests: 0, mutations: 0
}));
assert.throws(
  () => sandbox.orbitBootstrap(visitorIdForLimit),
  /daily launch limit/,
  "the global daily bootstrap budget must stop repeated fresh profiles"
);
properties.set(usageKey, JSON.stringify({
  day: new Date().toISOString().slice(0, 10),
  bootstraps: 2, requests: 800, mutations: 0
}));
expectStatus(api(sessionA, "GET", "/api/v1/meta"), 429, "global daily request budget");
properties.set(usageKey, JSON.stringify({
  day: new Date().toISOString().slice(0, 10),
  bootstraps: 2, requests: 0, mutations: 200
}));
expectStatus(api(sessionA, "POST", workspacePath + "/incidents", {
  title: "Daily mutation budget test", service_id: service.id, severity: "low",
}), 429, "global daily mutation budget");
properties.set(usageKey, JSON.stringify({
  day: new Date().toISOString().slice(0, 10),
  bootstraps: 2, requests: 0, mutations: 0
}));
assert.equal(sessionA.user.sub.startsWith("demo:"), true);

function api(session, method, path, body) {
  return sandbox.orbitApi({ sessionToken: session.sessionToken, method, path, body });
}
function expectStatus(result, status, label) {
  assert.equal(result.status, status, label + ": " + JSON.stringify(result.body));
  return result.body;
}

expectStatus(api(sessionA, "GET", "/api/v1/meta"), 200, "metadata");
const workspaceList = expectStatus(api(sessionA, "GET", "/api/v1/workspaces"), 200, "workspace list");
assert.equal(workspaceList.data.length, 1);
assert.equal(workspaceList.data[0].id, sessionA.workspace.id);
const membership = expectStatus(api(sessionA, "GET", "/api/v1/workspaces/" + sessionA.workspace.id + "/membership"), 200, "membership");
assert.equal(membership.data.role, "owner");
const services = expectStatus(api(sessionA, "GET", "/api/v1/workspaces/" + sessionA.workspace.id + "/services"), 200, "services");
assert.equal(services.data.length, 5);
const service = services.data.find((item) => item.name === "api-gateway");
assert.ok(service, "seeded API gateway exists");

const workspacePath = "/api/v1/workspaces/" + sessionA.workspace.id;
const incidents = expectStatus(api(sessionA, "GET", workspacePath + "/incidents"), 200, "incidents");
assert.equal(incidents.data.length, 2);
const createdIncident = expectStatus(api(sessionA, "POST", workspacePath + "/incidents", {
  title: "Smoke-test incident", description: "Synthetic test only.",
  service_id: service.id, severity: "high",
}), 200, "create incident");
assert.equal(createdIncident.data.status, "open");
const timeline = expectStatus(api(sessionA, "GET", workspacePath + "/incidents/" + createdIncident.data.id + "/timeline"), 200, "incident timeline");
assert.equal(timeline.data.length, 1);
const transitioned = expectStatus(api(sessionA, "PATCH", workspacePath + "/incidents/" + createdIncident.data.id + "/status", {
  status: "investigating", comment: "Synthetic transition",
}), 200, "incident transition");
assert.equal(transitioned.data.status, "investigating");
expectStatus(api(sessionA, "PATCH", workspacePath + "/incidents/" + createdIncident.data.id + "/status", {
  status: "open",
}), 409, "invalid incident transition");

const rules = expectStatus(api(sessionA, "GET", workspacePath + "/alert-rules"), 200, "alert rules");
assert.ok(rules.data.length >= 1);
const rule = expectStatus(api(sessionA, "POST", workspacePath + "/alert-rules", {
  name: "Smoke rule", description: "Synthetic only", service_name: "api-gateway",
  metric: "request_rate", operator: "gt", threshold: 20, window_seconds: 300,
  pending_seconds: 60, severity: "medium",
}), 200, "create alert rule");
assert.equal(rule.data.enabled, true);
expectStatus(api(sessionA, "PATCH", workspacePath + "/alert-rules/" + rule.data.id + "/enabled", { enabled: false }), 200, "disable alert rule");
expectStatus(api(sessionA, "DELETE", workspacePath + "/alert-rules/" + rule.data.id), 200, "delete alert rule");

const channel = expectStatus(api(sessionA, "POST", workspacePath + "/alert-notification-channels", {
  name: "Smoke demo channel", secret_ref: "DEMO_ONLY",
}), 200, "create notification channel");
assert.equal(channel.data.enabled, false);
expectStatus(api(sessionA, "PATCH", workspacePath + "/alert-notification-channels/" + channel.data.id + "/enabled", {
  enabled: true,
}), 200, "enable simulated notification channel");
const deliveries = expectStatus(api(sessionA, "GET", workspacePath + "/alert-notification-deliveries"), 200, "notification deliveries");
assert.ok(deliveries.data.some((item) => item.channel_id === channel.data.id && item.demo_mode === true));
const delivery = deliveries.data.find((item) => item.channel_id === channel.data.id);
const attempts = expectStatus(api(sessionA, "GET", workspacePath + "/alert-notification-deliveries/" + delivery.id + "/attempts"), 200, "notification attempts");
assert.equal(attempts.data[0].outcome, "delivered");

const deployed = expectStatus(api(sessionA, "POST", workspacePath + "/deployments", {
  service_id: service.id, version: "smoke-1.0.0", commit_sha: "abcdef0123456789",
  change_reference: "DEMO-1", release_notes: "Synthetic deployment",
}), 200, "create deployment");
const deploymentList = expectStatus(api(sessionA, "GET", workspacePath + "/deployments"), 200, "deployment list");
assert.ok(deploymentList.data.some((item) => item.id === deployed.data.id));
const apiDeployments = deploymentList.data.filter((item) => item.service_id === service.id);
assert.ok(apiDeployments.length >= 2);
const comparison = expectStatus(api(sessionA, "GET", workspacePath + "/deployments/compare?from_id=" +
  encodeURIComponent(apiDeployments[1].id) + "&to_id=" + encodeURIComponent(apiDeployments[0].id)), 200, "deployment comparison");
assert.equal(comparison.from.service_id, comparison.to.service_id);

const slo = expectStatus(api(sessionA, "POST", workspacePath + "/slos", {
  service_id: service.id, name: "Smoke availability", route: "GET /health",
  target: 0.999, rolling_window_days: 7,
}), 200, "create SLO");
const budget = expectStatus(api(sessionA, "GET", workspacePath + "/slos/" + slo.data.id + "/budget"), 200, "SLO budget");
assert.equal(budget.objective.id, slo.data.id);
expectStatus(api(sessionA, "PATCH", workspacePath + "/slos/" + slo.data.id + "/enabled", { enabled: false }), 200, "disable SLO");
expectStatus(api(sessionA, "PUT", workspacePath + "/slos/" + slo.data.id, {
  name: "Updated smoke availability", route: "GET /ready", target: 0.995, rolling_window_days: 3,
}), 200, "update SLO");
expectStatus(api(sessionA, "DELETE", workspacePath + "/slos/" + slo.data.id), 200, "delete SLO");

const audit = expectStatus(api(sessionA, "GET", workspacePath + "/audit-events?limit=100"), 200, "audit history");
assert.ok(audit.data.some((item) => item.action === "incident.created"));

const telemetryServices = expectStatus(api(sessionA, "GET", workspacePath + "/telemetry/services?limit=50"), 200, "telemetry service catalog");
assert.equal(telemetryServices.data.length, 5);
const metricCatalog = expectStatus(api(sessionA, "GET", workspacePath + "/telemetry/metrics/catalog"), 200, "metric catalog");
assert.equal(metricCatalog.data.length, 3);
const metrics = expectStatus(api(sessionA, "GET", workspacePath + "/telemetry/metrics?window=1h&service=api-gateway&metric=request_rate&group_by=instance"), 200, "metrics");
assert.equal(metrics.data.length, 3);
const logs = expectStatus(api(sessionA, "GET", workspacePath + "/telemetry/logs?window=1h&service=api-gateway&limit=20"), 200, "logs");
const logTraceId = JSON.parse(logs.data[0].values.find((entry) => JSON.parse(entry[1]).trace_id)?.[1]).trace_id;
const traces = expectStatus(api(sessionA, "GET", workspacePath + "/telemetry/traces?window=1h&service=api-gateway&limit=20"), 200, "traces");
assert.ok(traces.data.some((trace) => trace.traceID === logTraceId), "Logs-to-Trace correlation IDs must match");
const traceDetail = expectStatus(api(sessionA, "GET", workspacePath + "/telemetry/traces/" + logTraceId), 200, "trace detail");
assert.equal(traceDetail.trace_id, logTraceId);
assert.ok(traceDetail.data.batches.length > 0);

const isolated = expectStatus(api(sessionB, "GET", "/api/v1/workspaces"), 200, "second workspace list");
assert.notEqual(isolated.data[0].id, sessionA.workspace.id);
expectStatus(api(sessionB, "GET", workspacePath + "/services"), 404, "cross-workspace read denied");
expectStatus(sandbox.orbitApi({ sessionToken: "invalid", method: "GET", path: "/api/v1/meta" }), 401, "invalid session denied");
expectStatus(api(sessionA, "GET", workspacePath + "/telemetry/metrics?window=30d&service=api-gateway"), 400, "invalid metric window denied");
expectStatus(api(sessionA, "GET", "/api/v1/unlisted-route"), 404, "unlisted API route denied");
expectStatus(api(sessionA, "POST", workspacePath + "/incidents", { title: "x".repeat(17000) }), 413, "oversized request denied");

assert.ok(records.getLastRow() > 20, "records are persisted to the Sheets adapter");
console.log("Apps Script demo runtime smoke: OK");

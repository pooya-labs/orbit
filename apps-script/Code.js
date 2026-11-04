/**
 * ORBIT Google Apps Script demo runtime.
 * Public demo only: synthetic data, browser-isolated workspaces, and no real
 * customer telemetry, credentials, external webhooks, or production systems.
 */
const ORBIT_CONFIG = Object.freeze({
  appUrl: "https://pooya-labs.github.io/orbit/?orbitRuntime=apps-script",
  databaseProperty: "ORBIT_DEMO_SPREADSHEET_ID",
  sheetName: "records",
  schemaVersion: 1,
  sessionTtlSeconds: 21600,
  maxRequestsPerSession: 250,
  maxMutationsPerSession: 60,
  maxBodyBytes: 16384,
  maxRecords: 20000,
  maxVisitors: 500,
  maxDailyInitializers: 5,
  maxDailyBootstraps: 25,
  maxDailyRequests: 800,
  maxDailyMutations: 200
});
const ORBIT_HEADERS = ["collection", "id", "workspace_id", "payload_json", "updated_at"];
const ORBIT_WINDOWS = Object.freeze({ "15m": 900, "1h": 3600, "6h": 21600, "24h": 86400 });

function doGet() {
  getRecordsSheet_();
  const template = HtmlService.createTemplateFromFile("Index");
  template.appUrl = ORBIT_CONFIG.appUrl;
  return template.evaluate().setTitle("ORBIT — Interactive Demo")
    .addMetaTag("viewport", "width=device-width, initial-scale=1");
}

/** Run once from the Apps Script editor to create the private demo datastore. */
function setupOrbitDemo_() {
  const props = PropertiesService.getScriptProperties();
  let id = props.getProperty(ORBIT_CONFIG.databaseProperty);
  let spreadsheet;
  if (id) spreadsheet = SpreadsheetApp.openById(id);
  else {
    spreadsheet = SpreadsheetApp.create("ORBIT Demo Database");
    id = spreadsheet.getId();
    props.setProperty(ORBIT_CONFIG.databaseProperty, id);
  }
  let sheet = spreadsheet.getSheetByName(ORBIT_CONFIG.sheetName);
  if (!sheet) {
    const firstSheet = spreadsheet.getSheets()[0];
    if (firstSheet && firstSheet.getName() === "Sheet1" && firstSheet.getLastRow() === 0) {
      firstSheet.setName(ORBIT_CONFIG.sheetName);
      sheet = firstSheet;
    } else {
      sheet = spreadsheet.insertSheet(ORBIT_CONFIG.sheetName);
    }
  }
  if (sheet.getLastRow() === 0) {
    sheet.getRange(1, 1, 1, ORBIT_HEADERS.length).setValues([ORBIT_HEADERS]);
    sheet.setFrozenRows(1);
    sheet.getRange(1, 1, 1, ORBIT_HEADERS.length).setFontWeight("bold");
    sheet.setColumnWidths(1, ORBIT_HEADERS.length, 190);
    sheet.setColumnWidth(4, 520);
  }
  const result = { spreadsheetId: id, spreadsheetUrl: spreadsheet.getUrl(),
    schemaVersion: ORBIT_CONFIG.schemaVersion,
    message: "ORBIT demo storage is ready. Keep this spreadsheet private." };
  Logger.log("ORBIT demo spreadsheet: " + result.spreadsheetUrl);
  return result;
}

/**
 * Safe, idempotent setup entry point. It is visible in the editor Run menu,
 * but returns no spreadsheet URL or ID to anonymous google.script.run callers.
 */
function initializeOrbitDemo() {
  const lock = LockService.getScriptLock();
  lock.waitLock(10000);
  try {
    const id = PropertiesService.getScriptProperties().getProperty(ORBIT_CONFIG.databaseProperty);
    if (!id) {
      if (!consumeDailyBudget_("initializers", ORBIT_CONFIG.maxDailyInitializers)) {
        throw new Error("The demo setup limit has been reached. Please try again tomorrow.");
      }
      setupOrbitDemo_();
    }
    getRecordsSheet_();
    return { ready: true, schemaVersion: ORBIT_CONFIG.schemaVersion };
  } finally {
    lock.releaseLock();
  }
}

/** Public bridge bootstrap. The browser key is random, pseudonymous and local. */
function orbitBootstrap(clientId) {
  const lock = LockService.getScriptLock();
  lock.waitLock(10000);
  try {
    const sheet = getRecordsSheet_();
    if (typeof clientId !== "string" || !/^[a-f0-9-]{36}$/i.test(clientId)) {
      throw new Error("The browser demo identifier is invalid. Reload the demo.");
    }
    if (!consumeDailyBudget_("bootstraps", ORBIT_CONFIG.maxDailyBootstraps)) {
      throw new Error("The public demo's daily launch limit has been reached. Please try again tomorrow.");
    }
    const browserHash = sha256_(clientId);
    let visitor = readOne_("visitors", "", browserHash, sheet);
    let workspace;
    if (visitor && visitor.workspace_id) {
      workspace = readOne_("workspaces", visitor.workspace_id, visitor.workspace_id, sheet);
    }
    if (!workspace) {
      if (readCollection_("visitors", "", sheet).length >= ORBIT_CONFIG.maxVisitors) {
        throw new Error("The public demo workspace limit has been reached. Please contact the project owner.");
      }
      workspace = createDemoWorkspace_(browserHash, sheet);
      visitor = { id: browserHash, workspace_id: "", browser_key_hash: browserHash, created_at: now_() };
      visitor.workspace_id = workspace.id;
      writeRecord_("visitors", "", visitor, sheet);
    }
    const token = Utilities.getUuid().replace(/-/g, "") + Utilities.getUuid().replace(/-/g, "");
    CacheService.getScriptCache().put("session:" + sha256_(token), workspace.id, ORBIT_CONFIG.sessionTtlSeconds);
    return {
      sessionToken: token, expiresInSeconds: ORBIT_CONFIG.sessionTtlSeconds,
      user: { sub: "demo:" + browserHash.slice(0, 16), name: "ORBIT Demo Operator",
        preferred_username: "demo-operator" },
      workspace: workspace, mode: "public-demo"
    };
  } finally {
    lock.releaseLock();
  }
}

/** API proxy called only through google.script.run in the trusted wrapper. */
function orbitApi(request) {
  let lock;
  try {
    if (!request || typeof request !== "object") return errorResponse_(400, "invalid_request", "The API request is invalid.");
    const method = String(request.method || "").toUpperCase();
    const path = String(request.path || "");
    const token = String(request.sessionToken || "");
    const body = request.body === undefined ? {} : request.body;
    if (!["GET", "POST", "PUT", "PATCH", "DELETE"].includes(method)) {
      return errorResponse_(405, "method_not_allowed", "This request method is not supported.");
    }
    if (!path.startsWith("/api/v1/") || path.length > 1200 || path.indexOf("..") >= 0) {
      return errorResponse_(404, "route_not_found", "The requested demo API route was not found.");
    }
    if (utf8Length_(JSON.stringify(body)) > ORBIT_CONFIG.maxBodyBytes) {
      return errorResponse_(413, "request_too_large", "The demo API request is too large.");
    }
    lock = LockService.getScriptLock();
    lock.waitLock(10000);
    const cache = CacheService.getScriptCache();
    const workspaceId = cache.get("session:" + sha256_(token));
    if (!workspaceId) return errorResponse_(401, "demo_session_expired", "The demo session expired. Reload the page to start a new demo session.");
    if (!consumeRateLimit_(token, method !== "GET").ok) {
      return errorResponse_(429, "demo_rate_limited", "The demo request limit was reached. Wait a little and try again.");
    }
    const result = routeApi_(method, path, body, workspaceId, getRecordsSheet_());
    return { status: 200, body: result };
  } catch (error) {
    return errorResponse_(error && error.status ? Number(error.status) : 500,
      error && error.code ? String(error.code) : "demo_runtime_error",
      error && error.safeMessage ? String(error.safeMessage) : "The demo service could not complete the request.");
  } finally {
    if (lock && lock.hasLock()) lock.releaseLock();
  }
}

function getRecordsSheet_() {
  const id = PropertiesService.getScriptProperties().getProperty(ORBIT_CONFIG.databaseProperty);
  if (!id) throw new Error("Run initializeOrbitDemo once from the Apps Script editor.");
  const spreadsheet = SpreadsheetApp.openById(id);
  let sheet = spreadsheet.getSheetByName(ORBIT_CONFIG.sheetName);
  if (!sheet) {
    const firstSheet = spreadsheet.getSheets()[0];
    if (firstSheet && firstSheet.getName() === "Sheet1" && firstSheet.getLastRow() === 0) {
      firstSheet.setName(ORBIT_CONFIG.sheetName);
      sheet = firstSheet;
    } else {
      sheet = spreadsheet.insertSheet(ORBIT_CONFIG.sheetName);
    }
  }
  if (sheet.getLastRow() === 0) {
    sheet.getRange(1, 1, 1, ORBIT_HEADERS.length).setValues([ORBIT_HEADERS]);
    sheet.setFrozenRows(1);
  }
  return sheet;
}

function createDemoWorkspace_(browserHash, sheet) {
  const stamp = now_();
  const workspace = { id: uuid_(), name: "ORBIT Demo Workspace",
    slug: "orbit-demo-" + browserHash.slice(0, 6), created_at: stamp, updated_at: stamp };
  writeRecord_("workspaces", workspace.id, workspace, sheet);
  const services = [
    ["api-gateway", "Edge request router", "eu-central-1", "operational", 42, 0.0003],
    ["identity-core", "Authentication service", "eu-central-1", "operational", 68, 0.0008],
    ["billing-engine", "Payments and invoicing", "eu-west-1", "degraded", 184, 0.0142],
    ["event-stream", "Asynchronous event processing", "eu-central-1", "operational", 31, 0.0001],
    ["search-indexer", "Search indexing pipeline", "us-east-1", "incident", null, null]
  ].map(function (item) {
    return { id: uuid_(), name: item[0], description: item[1], environment: "production",
      region: item[2], status: item[3], latency_p95_ms: item[4], error_rate: item[5],
      last_seen_at: stamp, source: "demo_seed" };
  });
  services.forEach(function (service) { writeRecord_("services", workspace.id, service, sheet); });
  const incidents = [
    { id: uuid_(), title: "Search indexing stalled",
      description: "The indexing queue stopped advancing after a shard rebalance. Synthetic demo data.",
      service_id: services[4].id, service_name: services[4].name, severity: "critical",
      status: "investigating", started_at: minutesAgo_(14), updated_at: minutesAgo_(3),
      resolved_at: null, source: "demo_seed" },
    { id: uuid_(), title: "Elevated payment latency",
      description: "Payment API latency is above its normal baseline. Synthetic demo data.",
      service_id: services[2].id, service_name: services[2].name, severity: "medium",
      status: "monitoring", started_at: minutesAgo_(38), updated_at: minutesAgo_(7),
      resolved_at: null, source: "demo_seed" }
  ];
  incidents.forEach(function (incident) {
    writeRecord_("incidents", workspace.id, incident, sheet);
    writeRecord_("incident_timeline", workspace.id, {
      id: uuid_(), incident_id: incident.id, workspace_id: workspace.id, event_type: "created",
      from_status: null, to_status: incident.status, comment: "Incident opened from the seeded demo scenario.",
      request_id: "demo-seed", details: { source: "demo_seed" }, created_at: incident.started_at
    }, sheet);
  });
  const rules = [
    { id: uuid_(), workspace_id: workspace.id, name: "API error rate elevated",
      description: "Illustrative threshold for the demo API gateway.", service_name: "api-gateway",
      metric: "request_rate", operator: "gt", threshold: 120, window_seconds: 300,
      pending_seconds: 120, severity: "high", enabled: true, created_at: minutesAgo_(120), updated_at: stamp },
    { id: uuid_(), workspace_id: workspace.id, name: "Billing traffic anomaly",
      description: "Illustrative rule for billing-engine traffic.", service_name: "billing-engine",
      metric: "request_rate", operator: "gt", threshold: 80, window_seconds: 300,
      pending_seconds: 60, severity: "medium", enabled: true, created_at: minutesAgo_(90), updated_at: stamp }
  ];
  rules.forEach(function (rule) { writeRecord_("alert_rules", workspace.id, rule, sheet); });
  const alert = {
    id: uuid_(), workspace_id: workspace.id, alert_rule_id: rules[0].id,
    rule_name: rules[0].name, service_name: "api-gateway", metric: "request_rate",
    operator: "gt", threshold: 120, window_seconds: 300, pending_seconds: 120,
    severity: "high", status: "firing", value: 146.2, pending_since: minutesAgo_(11),
    fired_at: minutesAgo_(9), resolved_at: null, last_evaluated_at: minutesAgo_(1),
    created_at: minutesAgo_(11), updated_at: minutesAgo_(1)
  };
  writeRecord_("alert_instances", workspace.id, alert, sheet);
  const channel = {
    id: uuid_(), workspace_id: workspace.id, name: "Demo on-call channel",
    adapter: "webhook", enabled: true, secret_configured: true, demo_mode: true,
    created_at: minutesAgo_(75), updated_at: stamp
  };
  writeRecord_("notification_channels", workspace.id, channel, sheet);
  const delivery = {
    id: uuid_(), workspace_id: workspace.id, alert_instance_id: alert.id, event_id: uuid_(),
    channel_id: channel.id, channel_name: channel.name, event_type: "alert.firing",
    status: "delivered", attempts: 1, next_attempt_at: stamp, last_attempt_at: minutesAgo_(8),
    delivered_at: minutesAgo_(8), created_at: minutesAgo_(9), updated_at: minutesAgo_(8),
    rule_name: alert.rule_name, service_name: alert.service_name, metric: alert.metric,
    operator: alert.operator, threshold: alert.threshold, value: alert.value,
    severity: alert.severity, instance_status: alert.status, occurred_at: minutesAgo_(9), demo_mode: true
  };
  writeRecord_("notification_deliveries", workspace.id, delivery, sheet);
  writeRecord_("notification_attempts", workspace.id, {
    id: uuid_(), delivery_id: delivery.id, attempt_number: 1, outcome: "delivered",
    http_status: 200, attempted_at: minutesAgo_(8), demo_mode: true
  }, sheet);
  [
    { service: services[0], version: "1.8.2", commit: "a84f62e", ref: "REL-284", notes: "Harden request routing and retry behavior.", ago: 18 },
    { service: services[0], version: "1.8.1", commit: "7d1c09b", ref: "REL-279", notes: "Improve API gateway connection pooling.", ago: 145 },
    { service: services[2], version: "3.4.0", commit: "c52d10a", ref: "REL-271", notes: "Update billing reconciliation workflow.", ago: 420 }
  ].forEach(function (item) {
    writeRecord_("deployments", workspace.id, {
      id: uuid_(), workspace_id: workspace.id, service_id: item.service.id,
      service_name: item.service.name, environment: item.service.environment,
      version: item.version, commit_sha: item.commit, change_reference: item.ref,
      release_notes: item.notes, deployed_at: minutesAgo_(item.ago), created_at: minutesAgo_(item.ago)
    }, sheet);
  });
  [
    { service: services[0], name: "API availability", route: "GET /api/*", target: 0.999, days: 7 },
    { service: services[2], name: "Checkout availability", route: "POST /checkout", target: 0.99, days: 3 }
  ].forEach(function (item) {
    writeRecord_("slos", workspace.id, {
      id: uuid_(), workspace_id: workspace.id, service_id: item.service.id,
      service_name: item.service.name, environment: item.service.environment,
      name: item.name, route: item.route, target: item.target,
      rolling_window_days: item.days, enabled: true, created_at: minutesAgo_(500), updated_at: stamp
    }, sheet);
  });
  [
    ["workspace.created", "workspace", workspace.id],
    ["incident.created", "incident", incidents[0].id],
    ["incident.created", "incident", incidents[1].id],
    ["alert_rule.created", "alert_rule", rules[0].id],
    ["alert_rule.created", "alert_rule", rules[1].id],
    ["notification_channel.created", "notification_channel", channel.id],
    ["deployment.created", "deployment", ""],
    ["slo.created", "slo", ""]
  ].forEach(function (item) {
    writeRecord_("audit_events", workspace.id, {
      id: uuid_(), workspace_id: workspace.id, actor_ref: "demo-operator", actor_role: "owner",
      action: item[0], resource_type: item[1], resource_id: item[2], request_id: "demo-seed",
      details: { source: "demo_seed" }, created_at: stamp
    }, sheet);
  });
  return workspace;
}

function routeApi_(method, path, body, workspaceId, sheet) {
  const parsedPath = parseApiPath_(path);
  const pathname = parsedPath.pathname;
  if (method === "GET" && pathname === "/api/v1/meta") {
    return { name: "orbit-api", version: "1.0.0-demo", environment: "demo" };
  }
  if (method === "GET" && pathname === "/api/v1/workspaces") {
    return { data: readCollection_("workspaces", workspaceId, sheet) };
  }
  const prefix = "/api/v1/workspaces/";
  if (pathname.indexOf(prefix) !== 0) fault_(404, "route_not_found", "The requested demo API route was not found.");
  const rest = pathname.slice(prefix.length);
  const slash = rest.indexOf("/");
  const requestedWorkspaceId = decodeURIComponent(slash < 0 ? rest : rest.slice(0, slash));
  if (requestedWorkspaceId !== workspaceId) fault_(404, "workspace_not_found", "The demo workspace was not found.");
  const route = slash < 0 ? "" : rest.slice(slash + 1);
  const query = parsedPath.query;

  if (method === "GET" && route === "membership") {
    const workspace = readOne_("workspaces", workspaceId, workspaceId, sheet);
    if (!workspace) fault_(404, "workspace_not_found", "The demo workspace was not found.");
    return { data: { workspace_id: workspaceId, role: "owner", created_at: workspace.created_at, updated_at: workspace.updated_at } };
  }
  if (method === "GET" && route === "services") return { data: readCollection_("services", workspaceId, sheet) };

  if (route === "incidents" && method === "GET") {
    let items = readCollection_("incidents", workspaceId, sheet);
    const status = query.get("status");
    if (status) items = items.filter(function (item) { return item.status === status; });
    return { data: sortBy_(items, "started_at", false) };
  }
  if (route === "incidents" && method === "POST") {
    const service = findService_(workspaceId, body.service_id, sheet);
    const stamp = now_();
    const incident = {
      id: uuid_(), title: text_(body.title, 120, "title"),
      description: text_(body.description || "Created in the ORBIT public demo.", 2000, "description"),
      service_id: service.id, service_name: service.name,
      severity: enum_(body.severity, ["critical", "high", "medium", "low"], "severity"),
      status: "open", started_at: stamp, updated_at: stamp, resolved_at: null, source: "demo_seed"
    };
    writeRecord_("incidents", workspaceId, incident, sheet);
    writeRecord_("incident_timeline", workspaceId, {
      id: uuid_(), workspace_id: workspaceId, incident_id: incident.id, event_type: "created",
      from_status: null, to_status: "open", comment: "Created in the isolated ORBIT demo workspace.",
      request_id: "demo-" + uuid_(), details: { source: "apps_script_demo" }, created_at: stamp
    }, sheet);
    audit_(workspaceId, "incident.created", "incident", incident.id, {}, sheet);
    return { data: incident };
  }
  let match = route.match(/^incidents\/([^/]+)\/status$/);
  if (match && method === "PATCH") {
    const incident = requireRecord_("incidents", workspaceId, decodeURIComponent(match[1]), sheet, "incident_not_found");
    const next = enum_(body.status, ["open", "investigating", "monitoring", "resolved"], "status");
    const allowed = { open: ["investigating", "monitoring", "resolved"],
      investigating: ["monitoring", "resolved"], monitoring: ["investigating", "resolved"],
      resolved: ["investigating"] };
    if (incident.status === next || allowed[incident.status].indexOf(next) < 0) {
      fault_(409, "invalid_incident_transition", "The requested incident status transition is not allowed.");
    }
    const previous = incident.status;
    incident.status = next; incident.updated_at = now_();
    incident.resolved_at = next === "resolved" ? incident.updated_at : null;
    writeRecord_("incidents", workspaceId, incident, sheet);
    writeRecord_("incident_timeline", workspaceId, {
      id: uuid_(), workspace_id: workspaceId, incident_id: incident.id, event_type: "status_changed",
      from_status: previous, to_status: next, comment: text_(body.comment || "Status updated in demo.", 1000, "comment"),
      request_id: "demo-" + uuid_(), details: { source: "apps_script_demo" }, created_at: incident.updated_at
    }, sheet);
    audit_(workspaceId, "incident.status_changed", "incident", incident.id, { from: previous, to: next }, sheet);
    return { data: incident };
  }
  match = route.match(/^incidents\/([^/]+)\/timeline$/);
  if (match && method === "GET") {
    const incidentId = decodeURIComponent(match[1]);
    requireRecord_("incidents", workspaceId, incidentId, sheet, "incident_not_found");
    const limit = boundedInteger_(query.get("limit"), 50, 1, 100);
    return { data: sortBy_(readCollection_("incident_timeline", workspaceId, sheet).filter(function (item) {
      return item.incident_id === incidentId;
    }), "created_at", true).slice(-limit) };
  }
  if (route === "audit-events" && method === "GET") {
    return { data: sortBy_(readCollection_("audit_events", workspaceId, sheet), "created_at", false)
      .slice(0, boundedInteger_(query.get("limit"), 100, 1, 100)) };
  }

  if (route === "alert-rules" && method === "GET") return { data: sortBy_(readCollection_("alert_rules", workspaceId, sheet), "created_at", false) };
  if (route === "alert-rules" && method === "POST") {
    const rule = normalizeRule_(body, workspaceId, null, sheet);
    writeRecord_("alert_rules", workspaceId, rule, sheet);
    audit_(workspaceId, "alert_rule.created", "alert_rule", rule.id, {}, sheet);
    return { data: rule };
  }
  match = route.match(/^alert-rules\/([^/]+)$/);
  if (match && method === "PUT") {
    const old = requireRecord_("alert_rules", workspaceId, decodeURIComponent(match[1]), sheet, "alert_rule_not_found");
    const rule = normalizeRule_(body, workspaceId, old, sheet);
    writeRecord_("alert_rules", workspaceId, rule, sheet);
    audit_(workspaceId, "alert_rule.updated", "alert_rule", rule.id, {}, sheet);
    return { data: rule };
  }
  match = route.match(/^alert-rules\/([^/]+)\/enabled$/);
  if (match && method === "PATCH") {
    const rule = requireRecord_("alert_rules", workspaceId, decodeURIComponent(match[1]), sheet, "alert_rule_not_found");
    rule.enabled = Boolean(body.enabled); rule.updated_at = now_();
    writeRecord_("alert_rules", workspaceId, rule, sheet);
    audit_(workspaceId, "alert_rule.enabled_changed", "alert_rule", rule.id, { enabled: rule.enabled }, sheet);
    return { data: rule };
  }
  match = route.match(/^alert-rules\/([^/]+)$/);
  if (match && method === "DELETE") {
    const id = decodeURIComponent(match[1]);
    requireRecord_("alert_rules", workspaceId, id, sheet, "alert_rule_not_found");
    deleteRecord_("alert_rules", workspaceId, id, sheet);
    audit_(workspaceId, "alert_rule.deleted", "alert_rule", id, {}, sheet);
    return null;
  }
  if (route === "alerts" && method === "GET") {
    let alerts = readCollection_("alert_instances", workspaceId, sheet);
    const status = query.get("status");
    if (status) alerts = alerts.filter(function (item) { return item.status === status; });
    return { data: sortBy_(alerts, "created_at", false) };
  }

  if (route === "alert-notification-channels" && method === "GET") {
    return { data: readCollection_("notification_channels", workspaceId, sheet) };
  }
  if (route === "alert-notification-channels" && method === "POST") {
    const name = text_(body.name, 80, "name");
    const secretRef = text_(body.secret_ref, 64, "secret_ref");
    if (!/^[A-Z][A-Z0-9_]{0,63}$/.test(secretRef)) fault_(400, "invalid_secret_reference", "Use an uppercase demo reference such as OPS_PRIMARY.");
    const channel = { id: uuid_(), workspace_id: workspaceId, name: name, adapter: "webhook",
      enabled: false, secret_configured: true, demo_mode: true, secret_ref: secretRef,
      created_at: now_(), updated_at: now_() };
    writeRecord_("notification_channels", workspaceId, channel, sheet);
    audit_(workspaceId, "notification_channel.created", "notification_channel", channel.id, { demo_mode: true }, sheet);
    return { data: channel };
  }
  match = route.match(/^alert-notification-channels\/([^/]+)\/enabled$/);
  if (match && method === "PATCH") {
    const channel = requireRecord_("notification_channels", workspaceId, decodeURIComponent(match[1]), sheet, "notification_channel_not_found");
    channel.enabled = Boolean(body.enabled); channel.updated_at = now_(); channel.demo_mode = true;
    writeRecord_("notification_channels", workspaceId, channel, sheet);
    if (channel.enabled) createSimulatedDelivery_(workspaceId, channel, sheet);
    audit_(workspaceId, "notification_channel.enabled_changed", "notification_channel", channel.id,
      { enabled: channel.enabled, demo_mode: true }, sheet);
    return { data: channel };
  }
  if (route === "alert-notification-deliveries" && method === "GET") {
    let items = readCollection_("notification_deliveries", workspaceId, sheet);
    const status = query.get("status");
    if (status) items = items.filter(function (item) { return item.status === status; });
    return { data: sortBy_(items, "created_at", false) };
  }
  match = route.match(/^alert-notification-deliveries\/([^/]+)\/attempts$/);
  if (match && method === "GET") {
    const deliveryId = decodeURIComponent(match[1]);
    requireRecord_("notification_deliveries", workspaceId, deliveryId, sheet, "delivery_not_found");
    return { data: readCollection_("notification_attempts", workspaceId, sheet)
      .filter(function (item) { return item.delivery_id === deliveryId; })
      .sort(function (a, b) { return a.attempt_number - b.attempt_number; }) };
  }

  if (route === "deployments" && method === "GET") {
    let items = readCollection_("deployments", workspaceId, sheet);
    const serviceId = query.get("service_id");
    if (serviceId) items = items.filter(function (item) { return item.service_id === serviceId; });
    return { data: sortBy_(items, "deployed_at", false) };
  }
  if (route === "deployments" && method === "POST") {
    const service = findService_(workspaceId, body.service_id, sheet);
    const stamp = now_();
    const annotation = {
      id: uuid_(), workspace_id: workspaceId, service_id: service.id, service_name: service.name,
      environment: service.environment, version: text_(body.version, 80, "version"),
      commit_sha: optionalText_(body.commit_sha, 64), change_reference: optionalText_(body.change_reference, 80),
      release_notes: optionalText_(body.release_notes, 2000),
      deployed_at: validDate_(body.deployed_at) || stamp, created_at: stamp
    };
    writeRecord_("deployments", workspaceId, annotation, sheet);
    audit_(workspaceId, "deployment.created", "deployment", annotation.id, {}, sheet);
    return { data: annotation };
  }
  if (route === "deployments/compare" && method === "GET") {
    const from = requireRecord_("deployments", workspaceId, query.get("from_id"), sheet, "deployment_not_found");
    const to = requireRecord_("deployments", workspaceId, query.get("to_id"), sheet, "deployment_not_found");
    if (from.service_id !== to.service_id || from.environment !== to.environment) {
      fault_(400, "deployment_comparison_scope_mismatch", "Choose deployments for the same service and environment.");
    }
    return { from: from, to: to,
      elapsed_seconds: Math.max(0, Math.round((new Date(to.deployed_at).getTime() - new Date(from.deployed_at).getTime()) / 1000)),
      version_changed: from.version !== to.version, commit_changed: (from.commit_sha || "") !== (to.commit_sha || "") };
  }

  if (route === "slos" && method === "GET") {
    let items = readCollection_("slos", workspaceId, sheet);
    const serviceId = query.get("service_id");
    if (serviceId) items = items.filter(function (item) { return item.service_id === serviceId; });
    return { data: sortBy_(items, "created_at", false) };
  }
  if (route === "slos" && method === "POST") {
    const service = findService_(workspaceId, body.service_id, sheet);
    const slo = normalizeSlo_(body, workspaceId, service, null);
    writeRecord_("slos", workspaceId, slo, sheet);
    audit_(workspaceId, "slo.created", "slo", slo.id, {}, sheet);
    return { data: slo };
  }
  match = route.match(/^slos\/([^/]+)\/budget$/);
  if (match && method === "GET") {
    const slo = requireRecord_("slos", workspaceId, decodeURIComponent(match[1]), sheet, "slo_not_found");
    return buildSloBudget_(slo);
  }
  match = route.match(/^slos\/([^/]+)\/enabled$/);
  if (match && method === "PATCH") {
    const slo = requireRecord_("slos", workspaceId, decodeURIComponent(match[1]), sheet, "slo_not_found");
    slo.enabled = Boolean(body.enabled); slo.updated_at = now_();
    writeRecord_("slos", workspaceId, slo, sheet);
    audit_(workspaceId, "slo.enabled_changed", "slo", slo.id, { enabled: slo.enabled }, sheet);
    return { data: slo };
  }
  match = route.match(/^slos\/([^/]+)$/);
  if (match && method === "PUT") {
    const previous = requireRecord_("slos", workspaceId, decodeURIComponent(match[1]), sheet, "slo_not_found");
    const service = findService_(workspaceId, previous.service_id, sheet);
    const slo = normalizeSlo_(body, workspaceId, service, previous);
    writeRecord_("slos", workspaceId, slo, sheet);
    audit_(workspaceId, "slo.updated", "slo", slo.id, {}, sheet);
    return { data: slo };
  }
  match = route.match(/^slos\/([^/]+)$/);
  if (match && method === "DELETE") {
    const id = decodeURIComponent(match[1]);
    requireRecord_("slos", workspaceId, id, sheet, "slo_not_found");
    deleteRecord_("slos", workspaceId, id, sheet);
    audit_(workspaceId, "slo.deleted", "slo", id, {}, sheet);
    return null;
  }

  if (route === "telemetry/services" && method === "GET") {
    return { data: readCollection_("services", workspaceId, sheet).map(function (service) {
      return {
        id: service.id, name: service.name, sources: ["prometheus", "loki", "tempo", "otlp"],
        first_seen_at: minutesAgo_(500), last_seen_at: now_(), last_discovered_at: now_(),
        resource_attributes: { service_name: [service.name], service_version: ["1.8.2"],
          deployment_environment: [service.environment], cloud_region: [service.region] },
        resource_attribute_sources: { service_name: ["otlp"], service_version: ["otlp"],
          deployment_environment: ["otlp"], cloud_region: ["otlp"] }
      };
    }) };
  }
  if (route === "telemetry/metrics/catalog" && method === "GET") {
    return { data: [
      { id: "request_rate", unit: "requests/second", description: "Total demo request rate.", group_by: ["instance", "service_version", "deployment_environment"] },
      { id: "successful_request_rate", unit: "requests/second", description: "Successful demo request rate.", group_by: ["instance", "service_version", "deployment_environment"] },
      { id: "failed_request_rate", unit: "requests/second", description: "Failed demo request rate.", group_by: ["instance", "service_version", "deployment_environment"] }
    ] };
  }
  if (route === "telemetry/metrics" && method === "GET") return demoMetrics_(query, workspaceId, sheet);
  if (route === "telemetry/logs" && method === "GET") return demoLogs_(query, workspaceId, sheet);
  if (route === "telemetry/traces" && method === "GET") return demoTraces_(query, workspaceId, sheet);
  match = route.match(/^telemetry\/traces\/([a-f0-9]{32})$/i);
  if (match && method === "GET") return demoTraceDetail_(match[1].toLowerCase());

  fault_(404, "route_not_found", "The requested demo API route was not found.");
}

function parseApiPath_(path) {
  const question = path.indexOf("?");
  const pathname = question < 0 ? path : path.slice(0, question);
  const rawQuery = question < 0 ? "" : path.slice(question + 1);
  const values = {};
  if (rawQuery) {
    rawQuery.split("&").forEach(function (part) {
      if (!part) return;
      const separator = part.indexOf("=");
      const rawKey = separator < 0 ? part : part.slice(0, separator);
      const rawValue = separator < 0 ? "" : part.slice(separator + 1);
      const key = decodeQueryPart_(rawKey);
      if (!Object.prototype.hasOwnProperty.call(values, key)) values[key] = decodeQueryPart_(rawValue);
    });
  }
  return { pathname: pathname, query: { get: function (key) {
    return Object.prototype.hasOwnProperty.call(values, key) ? values[key] : null;
  } } };
}
function decodeQueryPart_(value) {
  try { return decodeURIComponent(String(value).replace(/\+/g, " ")); }
  catch (ignored) { return ""; }
}

function normalizeRule_(body, workspaceId, previous, sheet) {
  const stamp = now_();
  return {
    id: previous ? previous.id : uuid_(), workspace_id: workspaceId,
    name: text_(body.name, 100, "name"), description: text_(body.description || "", 1000, "description"),
    service_name: safeServiceName_(text_(body.service_name, 100, "service_name"), workspaceId, sheet),
    metric: "request_rate",
    operator: enum_(body.operator, ["gt", "gte", "lt", "lte"], "operator"),
    threshold: boundedNumber_(body.threshold, 0, 1000000, "threshold"),
    window_seconds: boundedInteger_(body.window_seconds, 300, 60, 86400),
    pending_seconds: boundedInteger_(body.pending_seconds, 60, 0, 86400),
    severity: enum_(body.severity, ["critical", "high", "medium", "low"], "severity"),
    enabled: previous ? previous.enabled : true,
    created_at: previous ? previous.created_at : stamp, updated_at: stamp
  };
}
function normalizeSlo_(body, workspaceId, service, previous) {
  const stamp = now_();
  return {
    id: previous ? previous.id : uuid_(), workspace_id: workspaceId,
    service_id: service.id, service_name: service.name, environment: service.environment,
    name: text_(body.name, 100, "name"), route: text_(body.route, 160, "route"),
    target: boundedNumber_(body.target, 0.9, 0.99999, "target"),
    rolling_window_days: enumNumber_(body.rolling_window_days, [1, 3, 7]),
    enabled: previous ? previous.enabled : true,
    created_at: previous ? previous.created_at : stamp, updated_at: stamp
  };
}
function buildSloBudget_(slo) {
  const total = slo.service_name === "billing-engine" ? 248000 : 512000;
  const bad = slo.service_name === "billing-engine" ? 860 : 92;
  const good = total - bad;
  const availability = good / total;
  const allowed = Math.floor(total * (1 - slo.target));
  const remaining = Math.max(0, allowed - bad);
  const consumed = allowed > 0 ? Math.min(100, (bad / allowed) * 100) : 100;
  return {
    objective: slo, sli: "http_request_success_rate",
    valid_event_policy: "Synthetic demo events only; not production SLI evidence.",
    window_start: new Date(Date.now() - slo.rolling_window_days * 86400000).toISOString(),
    window_end: now_(),
    budget: { status: bad > allowed ? "exhausted" : "within_budget", target: slo.target,
      good_events: good, bad_events: bad, total_events: total, availability: availability,
      allowed_bad_events: allowed, remaining_bad_events: remaining,
      budget_consumed_percent: consumed, budget_remaining_percent: Math.max(0, 100 - consumed), burn_rate: 0.82 }
  };
}
function createSimulatedDelivery_(workspaceId, channel, sheet) {
  const alert = readCollection_("alert_instances", workspaceId, sheet)[0];
  if (!alert) return;
  const stamp = now_();
  const delivery = {
    id: uuid_(), workspace_id: workspaceId, alert_instance_id: alert.id, event_id: uuid_(),
    channel_id: channel.id, channel_name: channel.name, event_type: "alert.firing",
    status: "delivered", attempts: 1, next_attempt_at: stamp, last_attempt_at: stamp,
    delivered_at: stamp, created_at: stamp, updated_at: stamp, rule_name: alert.rule_name,
    service_name: alert.service_name, metric: alert.metric, operator: alert.operator,
    threshold: alert.threshold, value: alert.value, severity: alert.severity,
    instance_status: alert.status, occurred_at: stamp, demo_mode: true
  };
  writeRecord_("notification_deliveries", workspaceId, delivery, sheet);
  writeRecord_("notification_attempts", workspaceId, {
    id: uuid_(), delivery_id: delivery.id, attempt_number: 1,
    outcome: "delivered", http_status: 200, attempted_at: stamp, demo_mode: true
  }, sheet);
}

function demoMetrics_(query, workspaceId, sheet) {
  const windowName = allowedWindow_(query.get("window"));
  const seconds = ORBIT_WINDOWS[windowName];
  const service = safeServiceName_(query.get("service"), workspaceId, sheet);
  const metric = enum_(query.get("metric") || "request_rate",
    ["request_rate", "successful_request_rate", "failed_request_rate"], "metric");
  const groupBy = query.get("group_by") || "";
  if (groupBy && ["instance", "service_version", "deployment_environment"].indexOf(groupBy) < 0) {
    fault_(400, "invalid_group_by", "This metric grouping is not available in the demo.");
  }
  const nowSeconds = Math.floor(Date.now() / 1000);
  const step = Math.max(15, Math.floor(seconds / 24));
  const names = groupBy ? (groupBy === "instance" ? ["api-01", "api-02", "api-03"] :
    groupBy === "service_version" ? ["1.8.1", "1.8.2"] : ["production", "staging"]) : [""];
  const base = metric === "failed_request_rate" ? 1.8 : metric === "successful_request_rate" ? 118.4 : 120.2;
  const data = names.map(function (name, seriesIndex) {
    const labels = { __name__: "orbit_demo_requests_total", service_name: service,
      instance: groupBy === "instance" ? name : "api-01",
      service_version: groupBy === "service_version" ? name : "1.8.2",
      deployment_environment: groupBy === "deployment_environment" ? name : "production",
      orbit_workspace_id: workspaceId };
    const values = [];
    for (let index = 0; index < 24; index += 1) {
      const timestamp = nowSeconds - (23 - index) * step;
      const wave = Math.sin((index + seriesIndex * 2) / 3) * (metric === "failed_request_rate" ? 0.45 : 12);
      const spike = index === 17 && metric === "failed_request_rate" ? 2.4 : 0;
      values.push([timestamp, Math.max(0, base + wave + spike + seriesIndex * 3).toFixed(3)]);
    }
    return { metric: labels, values: values };
  });
  return { source: "prometheus", window: windowName, queried_at: now_(),
    query: "Synthetic bounded demo metric; no PromQL is accepted from the browser.",
    step: step + "s", service: service, group_by: groupBy ? [groupBy] : [], metric: metric, data: data };
}
function demoLogs_(query, workspaceId, sheet) {
  const windowName = allowedWindow_(query.get("window"));
  const service = safeServiceName_(query.get("service"), workspaceId, sheet);
  const limit = boundedInteger_(query.get("limit"), 50, 1, 100);
  const nowSeconds = Math.floor(Date.now() / 1000);
  const traceId = "9f4a3c2d1e0b4a6789abcdef01234567";
  const levels = ["info", "info", "warn", "info", "error", "info", "debug"];
  const messages = ["Request accepted by the API gateway.", "Authentication completed in 8ms.",
    "Upstream latency exceeded the rolling baseline.", "Checkout workflow started.",
    "A retryable upstream response was observed.", "Database query completed successfully.",
    "Cache lookup completed."];
  const values = [];
  for (let index = 0; index < Math.min(limit, messages.length); index += 1) {
    const event = { timestamp: new Date((nowSeconds - (messages.length - index) * 27) * 1000).toISOString(),
      level: levels[index], service: service, message: messages[index],
      trace_id: index === 0 || index === 4 ? traceId : undefined, demo: true };
    values.push([String(nowSeconds - (messages.length - index) * 27) + "000000000", JSON.stringify(event)]);
  }
  return { source: "loki", window: windowName, queried_at: now_(), service: service, limit: limit,
    data: [{ stream: { service_name: service, environment: "production", source: "apps-script-demo" }, values: values }] };
}
function demoTraces_(query, workspaceId, sheet) {
  const windowName = allowedWindow_(query.get("window"));
  const service = safeServiceName_(query.get("service"), workspaceId, sheet);
  const limit = boundedInteger_(query.get("limit"), 20, 1, 50);
  const nowSeconds = Math.floor(Date.now() / 1000);
  const data = [];
  for (let index = 0; index < Math.min(limit, 8); index += 1) {
    data.push({ traceID: traceId_(index), rootServiceName: service,
      rootTraceName: index % 2 ? "POST /checkout" : "GET /api/v1/services",
      startTimeUnixNano: String(nowSeconds - index * 43) + "000000000", durationMs: 22 + index * 8 });
  }
  return { source: "tempo", window: windowName, queried_at: now_(), service: service, limit: limit, data: data };
}
function demoTraceDetail_(traceId) {
  const start = String(Math.floor(Date.now() / 1000) - 30) + "000000000";
  const middle = String(Math.floor(Date.now() / 1000) - 29) + "000000000";
  const end = String(Math.floor(Date.now() / 1000) - 28) + "000000000";
  return { source: "tempo", trace_id: traceId, queried_at: now_(), data: { batches: [{
    resource: { attributes: [
      { key: "service.name", value: { stringValue: "api-gateway" } },
      { key: "deployment.environment", value: { stringValue: "production" } }
    ] },
    scopeSpans: [{ scope: { name: "orbit.demo", version: "1.0.0" }, spans: [
      { traceId: traceId, spanId: "1111111111111111", name: "GET /api/v1/checkout",
        startTimeUnixNano: start, endTimeUnixNano: end, kind: 2, status: { code: 1 } },
      { traceId: traceId, spanId: "2222222222222222", parentSpanId: "1111111111111111",
        name: "postgres.query", startTimeUnixNano: middle, endTimeUnixNano: end, kind: 3, status: { code: 1 } }
    ] }]
  }] } };
}
function traceId_(index) {
  if (index === 0) return "9f4a3c2d1e0b4a6789abcdef01234567";
  const suffix = ("000000000000000" + (index + 1).toString(16)).slice(-15);
  return "9f4a3c2d1e0b4a678" + suffix;
}

function findService_(workspaceId, serviceId, sheet) {
  const service = readOne_("services", workspaceId, String(serviceId || ""), sheet);
  if (!service) fault_(404, "service_not_found", "The service was not found in this demo workspace.");
  return service;
}
function safeServiceName_(name, workspaceId, sheet) {
  const value = String(name || "api-gateway");
  if (!readCollection_("services", workspaceId, sheet).some(function (item) { return item.name === value; })) {
    fault_(404, "service_not_found", "The service was not found in this demo workspace.");
  }
  return value;
}
function requireRecord_(collection, workspaceId, id, sheet, code) {
  const item = readOne_(collection, workspaceId, String(id || ""), sheet);
  if (!item) fault_(404, code || "record_not_found", "The requested demo record was not found.");
  return item;
}
function audit_(workspaceId, action, resourceType, resourceId, details, sheet) {
  const stamp = now_();
  writeRecord_("audit_events", workspaceId, {
    id: uuid_(), workspace_id: workspaceId, actor_ref: "demo-operator", actor_role: "owner",
    action: action, resource_type: resourceType, resource_id: resourceId,
    request_id: "demo-" + uuid_(), details: Object.assign({ source: "apps_script_demo" }, details || {}),
    created_at: stamp
  }, sheet);
}
function consumeRateLimit_(token, mutation) {
  const cache = CacheService.getScriptCache();
  const key = "quota:" + sha256_(token) + ":" + (mutation ? "write" : "read");
  const limit = mutation ? ORBIT_CONFIG.maxMutationsPerSession : ORBIT_CONFIG.maxRequestsPerSession;
  const count = Number(cache.get(key) || 0);
  if (count >= limit) return { ok: false };
  const dailyKind = mutation ? "mutations" : "requests";
  const dailyLimit = mutation ? ORBIT_CONFIG.maxDailyMutations : ORBIT_CONFIG.maxDailyRequests;
  if (!consumeDailyBudget_(dailyKind, dailyLimit)) return { ok: false };
  cache.put(key, String(count + 1), ORBIT_CONFIG.sessionTtlSeconds);
  return { ok: true };
}

function consumeDailyBudget_(kind, limit) {
  const props = PropertiesService.getScriptProperties();
  const propertyKey = "ORBIT_DEMO_DAILY_USAGE";
  const today = new Date().toISOString().slice(0, 10);
  let usage = {};
  try {
    usage = JSON.parse(props.getProperty(propertyKey) || "{}");
  } catch (ignored) {
    usage = {};
  }
  if (!usage || usage.day !== today) {
    usage = { day: today, bootstraps: 0, requests: 0, mutations: 0 };
  }
  const count = Number(usage[kind] || 0);
  if (!Number.isFinite(count) || count >= limit) return false;
  usage[kind] = count + 1;
  props.setProperty(propertyKey, JSON.stringify(usage));
  return true;
}
function readCollection_(collection, workspaceId, sheet) {
  const last = sheet.getLastRow();
  if (last < 2) return [];
  const rows = sheet.getRange(2, 1, last - 1, ORBIT_HEADERS.length).getValues();
  const result = [];
  rows.forEach(function (row) {
    if (row[0] === collection && row[2] === workspaceId && row[3]) {
      try { result.push(JSON.parse(String(row[3]))); } catch (ignored) { /* Ignore a corrupt demo row. */ }
    }
  });
  return result;
}
function readOne_(collection, workspaceId, id, sheet) {
  const last = sheet.getLastRow();
  if (last < 2) return null;
  const rows = sheet.getRange(2, 1, last - 1, ORBIT_HEADERS.length).getValues();
  for (let index = rows.length - 1; index >= 0; index -= 1) {
    const row = rows[index];
    if (row[0] === collection && row[1] === id && row[2] === workspaceId && row[3]) {
      try { return JSON.parse(String(row[3])); } catch (ignored) { return null; }
    }
  }
  return null;
}
function writeRecord_(collection, workspaceId, value, sheet) {
  const payload = JSON.stringify(value);
  if (utf8Length_(payload) > 45000) fault_(413, "record_too_large", "The demo record is too large.");
  const last = sheet.getLastRow();
  if (last >= 2) {
    const ids = sheet.getRange(2, 1, last - 1, 3).getValues();
    for (let index = ids.length - 1; index >= 0; index -= 1) {
      if (ids[index][0] === collection && ids[index][1] === value.id && ids[index][2] === workspaceId) {
        sheet.getRange(index + 2, 1, 1, ORBIT_HEADERS.length)
          .setValues([[collection, value.id, workspaceId, payload, now_()]]);
        return;
      }
    }
  }
  if (last >= ORBIT_CONFIG.maxRecords + 1) fault_(507, "demo_storage_limit", "The demo storage limit has been reached.");
  sheet.appendRow([collection, value.id, workspaceId, payload, now_()]);
}
function deleteRecord_(collection, workspaceId, id, sheet) {
  const last = sheet.getLastRow();
  if (last < 2) return;
  const rows = sheet.getRange(2, 1, last - 1, 3).getValues();
  for (let index = rows.length - 1; index >= 0; index -= 1) {
    if (rows[index][0] === collection && rows[index][1] === id && rows[index][2] === workspaceId) {
      sheet.deleteRow(index + 2); return;
    }
  }
}
function sortBy_(items, key, ascending) {
  return items.slice().sort(function (a, b) {
    const left = String(a[key] || ""), right = String(b[key] || "");
    return (left < right ? -1 : left > right ? 1 : 0) * (ascending ? 1 : -1);
  });
}
function allowedWindow_(value) {
  const name = String(value || "1h");
  if (!Object.prototype.hasOwnProperty.call(ORBIT_WINDOWS, name)) fault_(400, "invalid_window", "Choose a supported telemetry window.");
  return name;
}
function boundedInteger_(value, fallback, minimum, maximum) {
  if (value === null || value === undefined || value === "") return fallback;
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < minimum || parsed > maximum) fault_(400, "invalid_number", "A numeric value is outside the supported range.");
  return parsed;
}
function boundedNumber_(value, minimum, maximum, label) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed < minimum || parsed > maximum) fault_(400, "invalid_" + label, "The " + label + " value is outside the supported range.");
  return parsed;
}
function enum_(value, allowed, label) {
  if (allowed.indexOf(value) < 0) fault_(400, "invalid_" + label, "The " + label + " value is not supported.");
  return value;
}
function enumNumber_(value, allowed) {
  const parsed = Number(value);
  if (allowed.indexOf(parsed) < 0) fault_(400, "invalid_window_days", "Choose a supported rolling window.");
  return parsed;
}
function text_(value, maximum, label) {
  const result = String(value === undefined || value === null ? "" : value).trim();
  if (!result || result.length > maximum) fault_(400, "invalid_" + label, "The " + label + " field is missing or too long.");
  return result;
}
function optionalText_(value, maximum) {
  if (value === undefined || value === null || value === "") return undefined;
  const result = String(value).trim();
  if (result.length > maximum) fault_(400, "invalid_text", "A text field is too long.");
  return result || undefined;
}
function validDate_(value) {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date.toISOString();
}
function fault_(status, code, message) {
  const error = new Error(message);
  error.status = status; error.code = code; error.safeMessage = message;
  throw error;
}
function errorResponse_(status, code, message) {
  return { status: status, body: { error: { code: code, message: message, request_id: "demo-" + uuid_() } } };
}
function sha256_(value) {
  const bytes = Utilities.computeDigest(Utilities.DigestAlgorithm.SHA_256, String(value), Utilities.Charset.UTF_8);
  return bytes.map(function (byte) { return ("0" + ((byte + 256) % 256).toString(16)).slice(-2); }).join("");
}
function utf8Length_(value) { return Utilities.newBlob(String(value)).getBytes().length; }
function uuid_() { return Utilities.getUuid(); }
function now_() { return new Date().toISOString(); }
function minutesAgo_(minutes) { return new Date(Date.now() - minutes * 60000).toISOString(); }

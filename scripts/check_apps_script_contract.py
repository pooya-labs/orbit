#!/usr/bin/env python3
"""Static compatibility and safety checks for the public Apps Script demo."""
from __future__ import annotations

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CODE = (ROOT / "apps-script" / "Code.js").read_text(encoding="utf-8")
HTML = (ROOT / "apps-script" / "Index.html").read_text(encoding="utf-8")
BRIDGE = (ROOT / "lib" / "orbit" / "apps-script-bridge.ts").read_text(encoding="utf-8")
MANIFEST = json.loads((ROOT / "apps-script" / "appsscript.json").read_text(encoding="utf-8"))

def require(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit("Apps Script contract check failed: " + message)

require(MANIFEST.get("runtimeVersion") == "V8", "the manifest must select the supported V8 runtime")
webapp = MANIFEST.get("webapp", {})
require(webapp.get("executeAs") == "USER_DEPLOYING", "the Web App must run as its deployer")
require(webapp.get("access") == "ANYONE_ANONYMOUS", "the manifest must declare anonymous public demo access")

for symbol in ("function doGet(", "function setupOrbitDemo_(", "function orbitBootstrap(", "function orbitApi("):
    require(symbol in CODE, "missing Apps Script entry point: " + symbol)
require("function setupOrbitDemo(" not in CODE, "spreadsheet setup function must remain private to google.script.run")
require("if (!id) setupOrbitDemo_()" not in CODE, "anonymous requests must not auto-create the owner spreadsheet")

# Apps Script V8 is not a browser or Node runtime. These APIs are absent there.
unsupported = [
    (r"\bnew\s+URL\s*\(", "URL constructor"),
    (r"\bURLSearchParams\b", "URLSearchParams"),
    (r"\bTextEncoder\b|\bTextDecoder\b", "web text codecs"),
    (r"\bsetTimeout\s*\(|\bsetInterval\s*\(", "browser timers"),
    (r"\bDOMException\b", "DOMException"),
    (r"\b(?:window\s*\.\s*(?:location|crypto|parent|addEventListener|postMessage|localStorage|document|history|navigator)|document\s*\.\s*[A-Za-z_$]|navigator\s*\.\s*[A-Za-z_$])", "browser globals"),
    (r"\bfetch\s*\(", "browser fetch"),
]
for pattern, label in unsupported:
    require(not re.search(pattern, CODE), label + " must not be used in Apps Script server code")

# The public demo is intentionally synthetic and must not emit real outbound traffic.
for forbidden in ("UrlFetchApp.fetch", "MailApp.sendEmail", "GmailApp.sendEmail", "ScriptApp.getOAuthToken"):
    require(forbidden not in CODE, "the public demo must not call " + forbidden)

for required in (
    "ORBIT_CONFIG.maxRequestsPerSession",
    "ORBIT_CONFIG.maxMutationsPerSession",
    "ORBIT_CONFIG.maxBodyBytes",
    "ORBIT_CONFIG.maxRecords",
    "ORBIT_CONFIG.maxDailyBootstraps",
    "ORBIT_CONFIG.maxDailyRequests",
    "ORBIT_CONFIG.maxDailyMutations",
    "consumeDailyBudget_",
    "CacheService.getScriptCache()",
    "LockService.getScriptLock()",
    "parseApiPath_(path)",
    "workspace_not_found",
    "apps_script_demo",
):
    require(required in CODE, "missing runtime guard/contract: " + required)

# All dashboard domains represented by v1 must be served by the Apps Script adapter.
routes = (
    'route === "incidents"',
    'route === "audit-events"',
    'route === "alert-rules"',
    'route === "alerts"',
    'route === "alert-notification-channels"',
    'route === "alert-notification-deliveries"',
    'route === "deployments"',
    'route === "deployments/compare"',
    'route === "slos"',
    'route === "telemetry/services"',
    'route === "telemetry/metrics/catalog"',
    'route === "telemetry/metrics"',
    'route === "telemetry/logs"',
    'route === "telemetry/traces"',
)
for route in routes:
    require(route in CODE, "missing v1 API route: " + route)

require("event.origin !== ORBIT_ORIGIN" in HTML, "wrapper must validate the embedded GitHub Pages origin")
require("event.source !== frame.contentWindow" in HTML, "wrapper must validate the expected iframe source")
require("google.script.run" in HTML, "wrapper must use google.script.run")
require("ORBIT_BRIDGE_REQUEST" in HTML and "ORBIT_BRIDGE_RESPONSE" in BRIDGE,
        "the iframe message bridge must be wired in both directions")
require("https://pooya-labs.github.io" in HTML, "wrapper must use the canonical Pages origin")

print("Apps Script demo contract: OK")

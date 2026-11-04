# ORBIT Apps Script demo runtime

This directory contains the zero-cost, low-volume public portfolio demo runtime.
It is not the production Go API and must never receive customer telemetry,
credentials, confidential data, or real webhook destinations.

## What it provides

- Apps Script Web App wrapper for the existing GitHub Pages dashboard.
- A google.script.run message bridge, so the dashboard does not need CORS,
  a public API key, or a paid proxy.
- One private Google Spreadsheet owned by the deployer, with a JSON-record
  sheet used as a small demo datastore.
- A separate synthetic workspace per browser profile.
- Bounded demo operations for services, incidents/timelines, metrics, logs,
  traces/trace detail, alert rules/instances, simulated notification history,
  deployments/comparison, SLO budgets, and audit events.
- No outbound webhook calls, email sending, arbitrary URL fetching, or real
  OpenTelemetry ingestion.

## First-time setup

1. Open https://script.google.com/ while signed in to the Google account that
   will own the demo.
2. Create a standalone Apps Script project named ORBIT Demo Runtime.
3. In Project Settings, enable Show "appsscript.json" manifest file in editor.
4. Replace the generated Code.js contents with apps-script/Code.js.
   Add an HTML file named Index and paste apps-script/Index.html.
   Replace appsscript.json with the manifest from this folder.
5. Save. In the function selector choose setupOrbitDemo_ and click Run.
   Review the requested Google Sheets permission and authorize it. This creates
   the private ORBIT Demo Database spreadsheet owned by your account.
6. Verify that setupOrbitDemo returns a spreadsheet URL. Keep that spreadsheet
   private; visitors never need direct access to it.
7. Choose Deploy → New deployment → Web app:
   - Execute as: Me
   - Who has access: Anyone (anonymous access)
   - Deploy and authorize the web app.
8. Open the /exec URL in a private/incognito browser. The dashboard should
   show the PUBLIC DEMO · SYNTHETIC DATA wrapper and connect automatically.

Google may show different labels or restrict anonymous deployment for some
account types or regions. Do not select a paid Google Cloud plan or enable
billing to work around such a restriction. If anonymous access is unavailable,
stop and report the exact message; do not switch to a less-safe public endpoint.

## Updating the deployment

After changing the files, save the Apps Script project and edit the existing
deployment to point to a new version. The /exec URL remains the same. Do not
use /dev as the public URL; /dev requires editor access.

## Data and safety model

- The app executes as its owner and writes only synthetic sample records into
  the owner's private spreadsheet.
- Each browser profile gets a random local browser key, hashed before storage,
  and a random six-hour session capability kept only in the iframe's memory.
- Requests are limited to the explicit /api/v1/ allowlist, bounded to 16 KiB,
  and capped per session. Record count is hard-capped.
- Each visitor can only address the workspace associated with that session.
  This is demo isolation, not production-grade identity or multi-tenant
  authorization.
- Notification deliveries are simulation records only. No webhook is called,
  even when a demo channel is enabled.
- Metrics, logs, traces, SLO budgets, and alert evaluations are synthetic.
  Their source and demo status must remain visible in the UI.
- The spreadsheet is owner-private. Do not share it publicly.
- No secrets, OIDC tokens, Google OAuth tokens, or user email addresses are
  sent to the browser or written into the spreadsheet.

## Free-tier boundaries

This design relies on Google Apps Script and Google Sheets quotas, which Google
may change. The current documented consumer-account limit is six minutes per
execution and 30 simultaneous executions per user; these are generous for a
few demo launches per day, but they are not an uptime guarantee. It is intentionally suitable for a few demo sessions per day, not
sustained telemetry ingestion. The project does not activate billing, add a
paid Google Cloud project, or provision any paid service.

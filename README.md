# ORBIT

<p align="center"><a href="https://github.com/pooya-labs/pooya-labs-platform"><img src="https://raw.githubusercontent.com/pooya-labs/pooya-labs-platform/main/apps/platform/public/brand/pooya-monogram-tile.svg" alt="Pooya Labs Platform" width="96" /></a></p>

**Distributed Systems Intelligence & Observability**

[![Stable release](https://img.shields.io/badge/release-v1.0.0-2ea44f)](https://github.com/pooya-labs/orbit/releases/tag/v1.0.0)
[![Local Work release](https://img.shields.io/badge/Local%20Work-v1.1.0-0f766e)](https://github.com/pooya-labs/orbit/releases/tag/local-work-v1.1.0)
[![CI](https://github.com/pooya-labs/orbit/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/pooya-labs/orbit/actions)
[![Security](https://github.com/pooya-labs/orbit/actions/workflows/security.yml/badge.svg?branch=main)](https://github.com/pooya-labs/orbit/actions)

**Project owner & maintainer:** [Pooya Labs Platform](https://github.com/pooya-labs/pooya-labs-platform) · **Source:** [pooya-labs/orbit](https://github.com/pooya-labs/orbit)

ORBIT is an engineering-focused observability workspace for exploring service health, incidents, operational workflows, and distributed telemetry across metrics, logs, and traces.

> **Release status: ORBIT v1.0.0 portfolio-demo scope is complete; ORBIT Local Work v1.1.0 is also published as a separate single-user local runtime.**  
> The patched Keycloak 26.8.2 artifact has been published and strictly validated on AMD64/ARM64, and PR [#138](https://github.com/pooya-labs/orbit/pull/138) is merged. The Cloudflare edge gateway and the separate Workers + D1 foundation are code-complete only to their documented stages; neither is connected to a live Go/Keycloak origin or deployed as a production runtime. Apps Script remains unchanged and synthetic-only.

## Local Work edition

The public v1.0.0 site remains the lightweight portfolio demo. The separate **[ORBIT Local Work v1.1.0 release](https://github.com/pooya-labs/orbit/releases/tag/local-work-v1.1.0)** provides a full single-user local runtime with the Go API, PostgreSQL, authenticated OTLP ingestion, Prometheus, Loki, Tempo, and operational workflows:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-local-work.ps1
```

Open `http://localhost:3000` and sign in with `ORBIT_LOCAL_HUMAN_TOKEN` from the generated, Git-ignored `.env.local-work` file. macOS/Linux users can run `./scripts/start-local-work.sh`. See the [Local Work guide](docs/local-work.md) for security boundaries, webhook configuration, persistence, and the fast test profile.

Local Work is a single-user, loopback-only runtime; it is not a hosted or production deployment. It uses separate local human and workload credentials and sends telemetry through the authenticated workspace gateway.

## Start here

- **[Open the dashboard](https://pooya-labs.github.io/orbit/)**
- **[ORBIT v1.0.0 stable release](https://github.com/pooya-labs/orbit/releases/tag/v1.0.0)**
- **[Release notes](docs/releases/v1.0.0.md)**
- **[Architecture](docs/architecture.md)**
- **[Roadmap and completion record](docs/roadmap.md)**

The GitHub Pages URL serves the static frontend. The persistent interactive portfolio demo runs through the owner-deployed Google Apps Script Web App wrapper, which embeds the dashboard and bridges API requests to the private demo datastore. Its /exec URL is owner-controlled and is intentionally not committed to this public repository.

## What is included in v1.0.0

The workspace includes the following views and workflows:

| Area | Implemented capability |
| --- | --- |
| Overview | Service health, incident summaries, and workspace dashboard |
| Incidents | Workspace-scoped incident lifecycle, detail, and immutable timeline |
| Alerts | Alert-rule definitions, bounded evaluation, and alert instances |
| Notifications | Channel configuration and delivery history; public-demo deliveries are simulations only |
| Deployments | Immutable deployment annotations and same-service release comparison |
| SLOs | HTTP request-success objectives, observed availability, and error budgets |
| Audit | Searchable workspace audit history with pseudonymous actor references |
| Metrics | Curated request, successful-request, and failed-request rate views |
| Logs | Bounded log search with validated trace correlation |
| Traces | Bounded trace search and workspace-authorized span details |
| Localization and accessibility | English/Persian UI, RTL layout, responsive navigation, keyboard semantics, chart table alternatives, and dark/light themes |

The metric catalog is intentionally small and server-owned. The browser cannot submit arbitrary PromQL, metric names, label matchers, or query expressions. Trace and log detail requests are workspace-authorized; high-cardinality trace IDs are not used as Loki index labels.

## Runtime profiles — intentionally separate

ORBIT has three documented execution profiles shown below, plus a separately staged Cloudflare-native Workers + D1 foundation merged in PR #140. The D1 profile is not deployed and is documented separately below. These paths share the dashboard/API contract where applicable, but their deployment state and data guarantees are different.

| Capability | Apps Script portfolio demo | Cloudflare edge profile (post-v1; not live yet) | Local engineering stack |
| --- | --- | --- | --- |
| Frontend | GitHub Pages embedded by the owner-deployed Apps Script Web App | Same static Pages dashboard, configured to send API traffic through orbit-runtime | Same dashboard, served by the local development server |
| Persistence | Private owner-owned Google Spreadsheet; synthetic records only | Canonical Go API + PostgreSQL origin; no application database in the Worker | PostgreSQL with versioned migrations and repository-backed data |
| Identity and isolation | Pseudonymous browser-profile workspaces and bounded demo sessions; not production identity | Existing Keycloak OIDC/PKCE and Go-side workspace authorization, once the origin is deployed | OIDC/role and workspace authorization, workload identities and signal grants |
| Metrics, logs, traces | Bounded synthetic demo data | Existing private Collector → Prometheus/Loki/Tempo pipeline, once the origin is deployed | Real local OTLP/HTTP and OTLP/gRPC ingestion through the Collector |
| Notifications | Simulated delivery history; no outbound requests | Canonical Go outbox/attempt history, once the origin is deployed | Backend delivery/retry implementation for local engineering and test scenarios |
| Hosting | GitHub Pages plus the owner's Google Apps Script deployment | GitHub Pages + Cloudflare Worker + separately hosted Go/Caddy origin; not yet connected end to end | Docker Compose on the developer's machine; not a hosted production service |

### Cloudflare deployment state

The canonical orbit-runtime edge gateway was merged through PR #136. The last inspected Cloudflare state (November 6, 2026) showed Worker version 0.2.3, but its workers.dev route was disabled, no Worker bindings or gateway secret were attached, and the Go/Caddy origin was not provisioned. The Worker therefore remains fail-closed; this repository does not claim that the Cloudflare runtime is live.

A separate Workers + D1 implementation was merged as a staged foundation in [PR #140](https://github.com/pooya-labs/orbit/pull/140). It is a different runtime profile, not an upgrade to the thin edge gateway. It still lacks live telemetry, alert evaluation, SLO budget calculation, notification delivery/attempt history, complete frontend acceptance and a deployed D1 environment. Do not deploy it or point Pages at it until those gates pass.



### Public demo safety and quotas

The Apps Script V8 runtime stores only synthetic demo records in the deployer's private Spreadsheet. It uses a fixed-origin google.script.run bridge, short-lived session capabilities, profile isolation, bounded request bodies, and quotas. Current repository limits include 250 API requests and 60 mutations per session, 25 new sessions, 800 API requests and 200 mutations per UTC day, a 20,000-row storage ceiling, and a 500-profile ceiling.

**Never enter customer data, personal/confidential information, credentials, production telemetry, or real webhook destinations into the public demo.** Notification delivery records are simulations: the demo does not send webhooks, email, or messages. Synthetic metrics, logs, traces, alert evaluations, and SLO results must remain visibly identified as demo data.

The Apps Script deployment is owned and administered separately from GitHub. Repository CI validates the Apps Script runtime contract and exercises the browser bridge with a mocked Google runtime; those automated tests are not a live test of Google's deployed Web App, Spreadsheet, or CacheService. The verification environment could not fetch script.google.com, so live owner-side bootstrap, mutation, and refresh-persistence behavior is **not independently verified by this release record**. This is an evidence limitation, not proof that the deployment is broken. See the [Apps Script setup and runtime guide](apps-script/README.md).

## Local engineering stack

The local stack is for development, integration testing, and demonstrating real local telemetry. It is not deployed behind the public Pages URL.

### Requirements

- Git
- Node.js 20 or newer and npm
- Docker Desktop (for PostgreSQL and the Go API)
- Go 1.27 or newer (for direct Go development)

### Run the dashboard

    git clone https://github.com/pooya-labs/orbit.git
    cd orbit
    npm install
    npm run dev

Open [http://localhost:3000](http://localhost:3000).

### Run the backend

From the repository root:

    docker compose up --build

The default Compose stack starts PostgreSQL, Redis, NATS, and the Go API. The local API is available at [http://localhost:8080](http://localhost:8080).

Useful endpoints:

| Endpoint | Purpose |
| --- | --- |
| /health/live | Process liveness |
| /health/ready | PostgreSQL readiness |
| /api/v1/meta | API metadata |
| /api/v1/services | Service inventory |
| /api/v1/incidents | Incident inventory |

To connect the local dashboard, open **Connect a source**, enter http://localhost:8080, and select **Test and connect**. Keep local development ports bound to loopback; the example stack is not configured as a public deployment.

### Run the local telemetry pipeline

The telemetry backends are optional. Start the observability profile from the repository root:

    docker compose --profile observability up -d postgres api prometheus loki tempo otel-collector telemetry-demo

The instrumented Go workload emits a request metric, a trace, and a structured log through OTLP/HTTP:

    curl.exe http://localhost:8081/emit

The Collector routes metrics to Prometheus, traces to Tempo, and logs to Loki. The Go API exposes bounded query endpoints for the configured telemetry stores. The dashboard presents those results in the Metrics, Logs, and Traces views.

The local path also exercises authenticated workspace ingestion and workspace-isolated read queries. A workload authorized for workspace A must not be able to write to or read data from workspace B. The tests reject direct cross-workspace writes and forged tenant attributes.

**Redis and NATS are provisioned by Compose but their functional application/event integration is deferred.** They are not represented as completed application features in v1.0.0.

## Verification and release evidence

The stable release was built from commit d2197244b1c1316cbe4b362d15c838c8eaa4cb1a, tagged v1.0.0, and published as a non-prerelease GitHub Release.

The release-commit verification completed successfully:

- **CI — 4/4 jobs passed:** web typecheck and production build; Apps Script runtime contract; 22 Chromium/Playwright interaction and axe tests; Go tests, vet and build; PostgreSQL backup/restore; full local OTLP Metrics/Logs/Traces end-to-end checks; authenticated workspace A/B write-to-query isolation; PostgreSQL restart and connection-pool recovery; and immutable workflow-action pin policy.
- **Security — 13/13 jobs passed:** dependency checks, full-history secret scanning, and scans/builds for the API, Collector, Tempo, Caddy, Prometheus, Loki, Redis, NATS, and the telemetry demo images.
- **GitHub Pages deployment:** successful on the stable release commit.
- **Stable release workflow:** exact-main CI and Security gates, version alignment, immutable tag creation, and Release publication all succeeded.
- **Windows NVDA Browse Mode navigation:** the automated heading-navigation check passed on the release-candidate source.

Evidence links: [release CI](https://github.com/pooya-labs/orbit/actions/runs/42032402892) · [release Security](https://github.com/pooya-labs/orbit/actions/runs/42032403220) · [Pages deployment](https://github.com/pooya-labs/orbit/actions/runs/42032403198) · [release workflow](https://github.com/pooya-labs/orbit/actions/runs/42032666313) · [NVDA navigation check](https://github.com/pooya-labs/orbit/actions/runs/42024149075).

These results establish the automated and repository-level checks listed above. They do not imply that every third-party deployment, device, or production environment has been manually accepted.

## Explicitly outside the v1.0.0 completion claim

The following are documented follow-up boundaries, not unfinished tasks hidden inside the released portfolio demo:

1. **Live Google deployment acceptance:** an owner-side clean-browser smoke is still needed to independently verify the deployed Apps Script doGet, bootstrap, metadata, a synthetic mutation, refresh persistence, and separate browser profiles. The automated bridge uses a mocked Google runtime.
2. **Human accessibility acceptance:** the project owner has confirmed the manual device/keyboard review is complete (2026-11-10), in addition to automated Chromium/axe and Windows NVDA checks. No formal WCAG conformance or certification is claimed; the project does not advertise one.
3. **Production identity and hosting:** the Go/PostgreSQL/OTel services, Keycloak, and ingress scaffolding are not a live hosted production service. The patched Keycloak 26.8.2 image is published and passed strict HIGH/CRITICAL scans against its immutable AMD64/ARM64 digest; production Compose intentionally remains on Keycloak 26.8.1 pending a separate deployment decision.
4. **Production operations:** hosted API and managed persistence, DNS/TLS/firewall/private-network policy, encrypted off-host backups and PITR, measured RPO/RTO, approved durable-history retention, sustained target-environment load/capacity, and production recovery exercises require their own deployment evidence.
5. **Deliberately bounded functionality:** arbitrary PromQL/LogQL, unrestricted metric discovery, real public-demo telemetry ingestion, real notification delivery, and Redis/NATS application integration are not v1 demo features.

The public portfolio demo is stable for its stated synthetic, low-volume purpose. It is **not** a production observability service, a customer-ingestion platform, or an uptime-guaranteed multi-tenant SaaS.

## Project documentation

- [Roadmap and v1.0.0 completion record](docs/roadmap.md)
- [Architecture and trust boundaries](docs/architecture.md)
- [Detailed project context and future handoff](docs/project-context.md)
- [v1.0.0 release notes](docs/releases/v1.0.0.md)
- [Apps Script demo setup and runtime](apps-script/README.md)
- [Manual accessibility acceptance guide](docs/manual-acceptance-guide.md)
- [Security threat model](docs/security-threat-model.md)
- [Telemetry retention and cardinality policy](docs/observability-retention.md)
- [PostgreSQL backup and restore runbook](docs/postgres-backup-restore.md)
- [Deployment and operations runbook](docs/operations-runbook.md)
- [OpenAPI 3.1 contract](docs/openapi.yaml)
- [Cloudflare dual-runtime expansion](docs/cloudflare-runtime.md)

## Quality checks

From the repository root:

    npm run typecheck
    npm run build
    cd server
    go test -race ./...
    go vet ./...
    go build ./cmd/orbit-api

GitHub Actions is the authoritative combined check for the web, Go/PostgreSQL, telemetry integration, security, and Pages release paths.

## Repository layout

    app/                         Next.js dashboard and design system
    apps-script/                 Public synthetic demo wrapper and Apps Script API
    lib/orbit/                   Typed API client, bridge, and session contracts
    examples/telemetry-go/       Instrumented OTLP demo workload
    server/cmd/orbit-api/        Go API entry point
    server/internal/             HTTP API, auth, telemetry, workers, and stores
    deploy/                      Local and deferred deployment scaffolding
    docs/                        Architecture, decisions, runbooks, roadmap, release notes
    .github/workflows/            CI, security, Pages, and release automation

## Completion status

**ORBIT v1.0.0 portfolio-demo scope: DONE.** No v1 implementation work remains open. Do not repeat completed features or reopen the immutable release tag for documentation changes. Future work should begin only from a concrete regression, a new issue, or an explicitly approved post-v1 production scope.

## License

No open-source license has been selected. All rights are reserved by default.


---

## Latest project update — 10 November 2026

The stable v1.0.0 release and immutable tag remain unchanged. Post-release maintenance merged [PR #163](https://github.com/pooya-labs/orbit/pull/163) (pgx 5.12.0), [PR #164](https://github.com/pooya-labs/orbit/pull/164) (lucide-react 1.70.0), and [PR #157](https://github.com/pooya-labs/orbit/pull/157) (OpenTelemetry Go modules 1.47.0; merge commit `8759eda737f88958fc24e511b5a8f24357b915c9`). PR #157's exact-head CI [42624823853](https://github.com/pooya-labs/orbit/actions/runs/42624823853) and Security [42624823618](https://github.com/pooya-labs/orbit/actions/runs/42624823618) both passed, including the full telemetry, tenant-isolation, and PostgreSQL recovery checks. The application-code baseline is `8759eda737f88958fc24e511b5a8f24357b915c9` (PR #157). Its post-merge Security [42631275307](https://github.com/pooya-labs/orbit/actions/runs/42631275307) and Pages [42631275337](https://github.com/pooya-labs/orbit/actions/runs/42631275337) succeeded; CI [42631275304](https://github.com/pooya-labs/orbit/actions/runs/42631275304) was cancelled when the documentation-only closeout commit superseded it, not because of a test failure. The final documentation-only main commit `dd5d05baf8291b0ba7de1a3b5f797a6d44fd2bd3` then passed CI [42633782680](https://github.com/pooya-labs/orbit/actions/runs/42633782680), Security [42633782856](https://github.com/pooya-labs/orbit/actions/runs/42633782856), Pages [42633783397](https://github.com/pooya-labs/orbit/actions/runs/42633783397), and stable-release verification [42633998789](https://github.com/pooya-labs/orbit/actions/runs/42633998789). Its CI correctly skipped Go and telemetry jobs because the diff was documentation-only. The immutable `v1.0.0` tag remained on `d2197244b1c1316cbe4b362d15c838c8eaa4cb1a`.

The Keycloak 26.8.2 remediation in [PR #138](https://github.com/pooya-labs/orbit/pull/138) has now been squash-merged as `3c8ebc56d34a4c9ed553338c7d8531bb1c4a0e20`. Its published immutable GHCR digest passed strict scans on AMD64 and ARM64, and the exact-head source rebuild and deferred identity checks passed. Production Compose remains on Keycloak 26.8.1; the patched artifact was not promoted or deployed. The v1.0.0 tag remains unchanged.

The final documentation closeout is complete. [PR #169](https://github.com/pooya-labs/orbit/pull/169) closed the post-merge secret-scan false positive with a narrow exact-path/exact-value Gitleaks allowance. [PR #170](https://github.com/pooya-labs/orbit/pull/170) then recorded the final verification evidence and was squash-merged as `f70e8464359e9b999c8b87490528cd132a80ee80`. The exact PR #170 head passed [CI](https://github.com/pooya-labs/orbit/actions/runs/42726300904) and [Security](https://github.com/pooya-labs/orbit/actions/runs/42726301000). On the resulting `main` commit, [CI](https://github.com/pooya-labs/orbit/actions/runs/42726617351), [Security](https://github.com/pooya-labs/orbit/actions/runs/42726617308), and [GitHub Pages deployment](https://github.com/pooya-labs/orbit/actions/runs/42726617657) all succeeded. No pull requests remained open at closeout. The stable `v1.0.0` tag still points to its original release commit; these documentation-only closeout commits did not move it.

**Final status:** the ORBIT v1.0.0 portfolio-demo scope is complete and closed. Production hosting, live OIDC tenancy, Cloudflare/D1 deployment, off-host backup automation, measured capacity and RPO/RTO exercises remain explicitly deferred until a new owner request.

## Project ownership

<p align="center"><a href="https://pooya-labs.github.io/pooya-labs-platform/"><img src="https://raw.githubusercontent.com/pooya-labs/pooya-labs-platform/main/apps/platform/public/brand/pooya-monogram-tile.svg" alt="Pooya Labs Platform monogram" width="96" /></a></p>

**Owned and maintained by [Pooya Labs Platform](https://pooya-labs.github.io/pooya-labs-platform/).** Project direction and engineering: **Pooya Goodarzi**.

## راهنمای فارسی

### معرفی

**ORBIT** یک محیط مهندسی برای پایش‌پذیری و هوشمندی سامانه‌های توزیع‌شده است. این پروژه گردش‌کارهای سلامت سرویس، رخدادها، هشدارها، استقرارها، اهداف سطح خدمت و داده‌های Metrics، Logs و Traces را در یک داشبورد یکپارچه نمایش می‌دهد.

### امکانات نسخهٔ پایدار ۱.۰.۰

- نمای کلی سلامت سرویس‌ها و خلاصهٔ رخدادها.
- مدیریت چرخهٔ عمر رخدادها و تاریخچهٔ تغییرناپذیر آن‌ها.
- تعریف و ارزیابی محدود قوانین هشدار و مشاهدهٔ نمونه‌های هشدار.
- ثبت نشانه‌های استقرار و مقایسهٔ انتشارهای یک سرویس.
- نمایش SLO، دسترس‌پذیری مشاهده‌شده و بودجهٔ خطا.
- جست‌وجوی محدود و کنترل‌شده در لاگ‌ها و Traceها.
- تاریخچهٔ ممیزی فضای کاری و نماهای منتخب متریک.
- رابط فارسی و انگلیسی، چیدمان راست‌به‌چپ، ناوبری واکنش‌گرا، دسترس‌پذیری صفحه‌کلید و پوستهٔ روشن و تاریک.

### وضعیت محیط‌های اجرا

نسخهٔ پایدار v1.0.0 شامل داشبورد استاتیک GitHub Pages و دموی تعاملی مبتنی بر Google Apps Script است. داده‌های دموی عمومی مصنوعی‌اند و ارسال اعلان‌ها در این محیط شبیه‌سازی می‌شود. پشتهٔ محلی Go/PostgreSQL و خط لولهٔ مشاهده‌پذیری برای توسعه و آزمون در نظر گرفته شده‌اند.

پروفایل Cloudflare در حال توسعهٔ پس از نسخهٔ ۱ است و نباید آن را یک سرویس مستقرشده یا فعال فرض کرد. همچنین سرویس‌های تولیدی میزبانی‌شده، هویت سازمانی و زیرساخت عملیاتی کامل جزو ادعای تکمیل نسخهٔ نمایشی v1.0.0 نیستند.

### اجرای داشبورد و پشتهٔ محلی

برای داشبورد:

```bash
git clone https://github.com/pooya-labs/orbit.git
cd orbit
npm install
npm run dev
```

برای اجرای سرویس‌های محلی، از ریشهٔ مخزن:

```bash
docker compose up --build
```

این پشته برای توسعه و آزمون است؛ پورت‌های توسعه را بدون پیکربندی امنیتی مناسب به اینترنت عمومی متصل نکنید.

### بررسی کیفیت

```bash
npm run typecheck
npm run build
cd server
go test -race ./...
go vet ./...
go build ./cmd/orbit-api
```

### مستندات تکمیلی

نقشهٔ راه، معماری، راهنمای استقرار، مدل تهدید امنیتی، راهنمای دسترس‌پذیری و قرارداد OpenAPI در پوشهٔ `docs/` و پیوندهای بخش انگلیسی README قرار دارند. وضعیت نسخهٔ پایدار و محدودهٔ کارهای پس از v1 نیز در همان مستندات ثبت شده است.

### آخرین تغییرات — ۱۰ نوامبر ۲۰۲۶

بررسی دستی صفحه‌کلید، دستگاه‌های واقعی و فناوری‌های کمکی در ۱۰ نوامبر ۲۰۲۶ توسط مالک پروژه تأیید شد. این تأیید به‌معنای ادعای انطباق رسمی یا گواهی WCAG نیست.

ارتقای OpenTelemetry به نسخهٔ ۱.۴۷.۰ در PR شمارهٔ ۱۵۷ با موفقیت ادغام شد. سپس PR شمارهٔ ۱۳۸ برای اصلاح Keycloak 26.8.2 نیز ادغام شد و ایمیج چندمعماری آن روی AMD64 و ARM64 با digest تغییرناپذیر و اسکن سخت‌گیرانه تأیید شد. PR نهایی بستن محدودهٔ فعلی نسخهٔ پورتفولیو، شمارهٔ ۱۶۹، با موفقیت ادغام شد. CI و Security روی commit نهایی شاخهٔ اصلی به‌ترتیب در اجراهای 42724757890 و 42724757906، انتشار Pages در اجرای 42724758064 و بررسی انتشار پایدار در اجرای 42724838160 همگی موفق شدند. اسکن Gitleaks نیز با استثنای دقیق و محدود به دو مسیر مستنداتی و مقدار تگ عمومی اصلاح شد؛ هیچ قاعدهٔ عمومی اسکن یا آستانهٔ HIGH/CRITICAL تضعیف نشد. تگ تغییرناپذیر v1.0.0 جابه‌جا نشد. Compose محیط Production همچنان روی Keycloak 26.8.1 است و ایمیج اصلاح‌شده مستقر نشده است. راه‌اندازی زیرساخت واقعی، آزمون ظرفیت در محیط هدف، پشتیبان‌گیری Production و سیاست نگهداری تاریخچه‌ها تا درخواست بعدی به تعویق افتاده‌اند. محدودهٔ نسخهٔ عمومی GitHub Pages و دموی مصنوعی Apps Script همان نسخهٔ پایدار ۱.۰.۰ است.

### مالکیت و نگه‌داری

<p align="center"><a href="https://pooya-labs.github.io/pooya-labs-platform/"><img src="https://raw.githubusercontent.com/pooya-labs/pooya-labs-platform/main/apps/platform/public/brand/pooya-monogram-tile.svg" alt="نشان مونوگرام پویا لبز" width="96" /></a></p>

**مالک و نگه‌دارنده: [پلتفرم پویا لبز](https://pooya-labs.github.io/pooya-labs-platform/)** · طراحی و توسعه: **پویا گودرزی**.

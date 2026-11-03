# ORBIT accessibility review

## Status

The keyboard and landmark baseline was merged into `main` via PR #109 (squash commit `e9e7d74387dddcf76226cfab32dd979f56af4dc9`). The remaining M01 accessibility acceptance item stays open for actual human keyboard, assistive-technology, and rendered-state visual review.

## Implemented baseline

- A first-in-order, localized skip link moves keyboard users to the primary content landmark.
- The application shell is a neutral container; the dashboard content is exposed as the page's `main` landmark.
- The sidebar exposes a named primary-navigation landmark, semantic group headings, and an active item announced with `aria-current="page"`.
- The updated page heading is announced politely when client-side navigation changes the selected view. The mobile navigation trigger exposes its expanded state and controlled navigation region. Opening the drawer moves focus to its close control. Escape closes it, Tab and Shift+Tab remain within the open drawer, and closing restores focus to the trigger. The closed off-canvas navigation is hidden from keyboard navigation.
- Links and interactive controls share a high-visibility `:focus-visible` treatment. Existing `prefers-reduced-motion` rules remain in place.
- English and Persian labels are supplied for the skip link and primary navigation.
- The demo chart time-range selector is a named group; its selected option is exposed with `aria-pressed`, with localized group names in English and Persian.
- Incident and alert filters, notification delivery filters, the selected incident, and the selected SLO objective expose their state with `aria-pressed`; Audit filters and telemetry grouping already did so.
- Both charts now have expandable native HTML data tables. The live Metrics table uses the same Prometheus response as its SVG chart and includes localized time, series, and request-rate columns. The Overview table derives clearly labelled illustrative samples from the exact SVG fixture points; its time axis and table timestamps both follow the selected 1-hour, 6-hour, or 24-hour window.
- Preview-only controls without an implemented action are disabled and expose a localized unavailable description. The top-bar Notifications control opens the implemented Notifications view, and the Docs affordance links to the repository documentation.

## Automated verification

The web CI job runs:

```sh
python3 scripts/check_accessibility_contract.py
python3 scripts/check_accessibility_contrast.py
npm run typecheck
npm run build
python3 scripts/check_accessibility_html.py
```

The source-level Python contract is a static regression guard for landmarks, labels, focus handling, and CSS hooks. The contrast check verifies the dark/light base text palette, the explicit light-theme text colors against the project's pale surfaces, and visible keyboard focus indicators. The generated-HTML audit inspects the exported pages for document language/title, main and named navigation landmarks, a working skip-link target, heading presence, duplicate IDs, missing accessible names on interactive controls, unlabeled form fields, missing image alternatives, and broken ARIA ID references. All three checks use only Python's standard library.

These Python checks are targeted source, palette, and generated-HTML regression guards. The palette script does not sample every computed color/background combination at runtime. Browser-level axe checks are run separately by the Playwright suite described below; neither the static scripts nor axe alone provide a full contrast audit, WCAG conformance report, or screen-reader test.

## Browser verification — 2026-11-03

The web CI job now installs Playwright Test 1.64.1, axe-core/playwright 4.14.0, and Chromium. It serves the production static export under the GitHub Pages `/orbit/` base path and runs the browser suite with:

```sh
npx playwright test --config=playwright.config.cjs
```

Pre-merge application/test head: `5e1e37cf789bfed9baf971f914a9df895e328ea3` from PR #109.

The web job in [CI run 41578302352](https://github.com/pooya16pg-spec/orbit/actions/runs/41578302352) passed on this head. It completed the static accessibility checks, theme contrast guard, TypeScript typecheck, production build, generated-HTML audit, and all **14 Playwright tests**. The source accessibility contract passed **68 checks**; the generated-HTML audit passed for **5 pages / 791 elements**.

Coverage:
1. First-Tab skip-link reveal and activation, followed by focus on the main landmark.
2. Primary navigation selection, active state, and announced page heading.
3. Overview chart table, selected time range, semantic headers, and range-aware labels.
4. Mobile drawer keyboard focus, Tab/Shift+Tab containment, Escape close, and trigger-focus restoration.
5. Mobile route selection and focus restoration.
6. Theme switching and Persian RTL direction/labels.
7. Metrics chart table populated from a controlled API mock, selected-window update, and semantic table structure.
8. axe WCAG 2.1 A/AA and WCAG 2.2 AA scan on dark Overview.
9. The same axe scan with the mobile drawer open.
10. The same axe scan on light-theme Overview.
11. All 12 primary dashboard views expose an announced heading and pass axe in **both dark and light themes** (24 view/theme scans).
12. Invalid API base paths are announced as an accessible error.
13. Reduced-motion preference disables smooth scrolling and long transitions.
14. Overview reflows at a 320 CSS-pixel viewport without page-level horizontal overflow; its named chart-table scroll region accepts keyboard focus and scrolls horizontally with ArrowRight.

The same CI run's Go API tests/build and workflow action-pin policy jobs passed. The full local telemetry pipeline end-to-end job subsequently completed successfully in CI run 41578302352 on this exact application/test head, verifying OTLP metrics/logs/traces through ORBIT API, Prometheus Collector scrape, workspace A/B isolation, and cleanup.

The latest [Security run 41578302399](https://github.com/pooya16pg-spec/orbit/actions/runs/41578302399) completed all **13 jobs successfully**, including the full-history secret scan, dependency review, API/Collector/Caddy/Tempo image scans, and all Compose image scans with SBOM generation. Pages-source verification [41578302366](https://github.com/pooya16pg-spec/orbit/actions/runs/41578302366) passed. The temporary branch Pages deployment [41578298625](https://github.com/pooya16pg-spec/orbit/actions/runs/41578298625) succeeded before merge. After merging PR #109, the temporary branch trigger and preview environment were removed; the main-only Pages deployment [41579531993](https://github.com/pooya16pg-spec/orbit/actions/runs/41579531993) then succeeded from the protected `github-pages` environment. The canonical site is https://pooya16pg-spec.github.io/orbit/.

PR #109 was squash-merged into `main` as `e9e7d74387dddcf76226cfab32dd979f56af4dc9`. The merged baseline is on `main`; the separate human acceptance gate below remains open.



The rendered axe scans found and helped correct low-contrast secondary text in dark Metrics/Overview, several light-theme badge/status/avatar colors, and a workspace-switcher visible-label mismatch. The regression script now explicitly guards the affected dark and light selectors. The final current-head run passed the browser interactions and all axe scans with no reported violations in the scanned states.

This suite tests the built Chromium page, not NVDA, VoiceOver, or TalkBack. It does not establish WCAG conformance or replace the manual acceptance below.

## Manual acceptance still required

Before closing the M01 accessibility review, test the deployed and local application with:

1. Keyboard only: skip link, every primary view, forms and validation errors, drawer open/close, Escape, tab order, focus restoration, zoom/reflow, both LTR and RTL layouts.
2. Screen readers: NVDA with Firefox or Edge on Windows; VoiceOver with Safari on macOS/iOS; TalkBack with Chrome on Android. Confirm navigation landmarks, headings, active view, form labels, loading/error/success announcements, table/list structure, and chart alternatives.
3. Visual access: text and control contrast in dark and light themes, focus visibility, status information that may rely on color, and reduced-motion behavior.
4. Control audit: confirm each visible control has a real action or is clearly presented as unavailable; verify that the Notifications control opens the Notifications view, and review the unavailable workspace, settings, incident-options, and help affordances.

Record browser/OS/assistive-technology versions, findings, and remediations here. Do not claim screen-reader support or WCAG conformance solely from passing static checks or the automated Chromium/axe suite.


## M01 baseline merged — 2026-11-03

PR #109 was squash-merged into `main` as `e9e7d74387dddcf76226cfab32dd979f56af4dc9`. The merge contains the keyboard/landmark and chart-table baseline described above.

Verified evidence:
- Web CI: 14/14 Playwright tests passed (14 tests, 35.8s), including 24 axe view/theme scans across all 12 primary views in dark and light themes, reduced-motion handling, keyboard navigation, drawer focus handling, 320 CSS-pixel reflow, and keyboard scrolling of the chart-data table.
- Static accessibility contract: 68 checks passed; contrast regression guard passed; generated HTML audit passed (5 pages / 791 elements).
- TypeScript typecheck and production build passed.
- Go race tests, vet, build, module-lock check, and PostgreSQL backup/restore round-trip passed.
- Local telemetry E2E passed: metrics, logs, and traces reached their backends and were queryable through the ORBIT API; Prometheus scraped the Collector; authenticated workspace A/B telemetry write-to-query isolation passed.
- Security run 41578754427 passed all 13 jobs on the latest documentation/code head, including image scans and the CycloneDX SBOM set. Pages-source check and the temporary branch deployment also succeeded before cleanup.

The automated baseline is merged, but the manual M01 acceptance checkbox remains open. No NVDA, VoiceOver, or TalkBack session was run in the remote CI environment; no WCAG conformance or screen-reader certification is claimed. Rendered visual-state contrast and human keyboard/chart-table review remain open.


## M01 navigation contrast transition regression — merged PR #113 (2026-11-03)

A post-merge axe run (CI `41597808873`) exposed two contrast violations on Service map in light theme immediately after navigation/theme transitions. The foregrounds were intermediate blended values, `#277d72` (4.42:1) and `#648085` (4.09:1), not the stable CSS values. The threshold remains 4.5:1.

PR #113 was squash-merged into `main` as `661ee0b7ea3faaa39197d03579e3775a5cdefd3b`:
- Sidebar text color changes instantly; only its background may transition. This removes the below-threshold color interpolation from the visible UI.
- The all-view axe suite waits for finite navigation transitions to finish after route and theme changes.
- The static contrast regression guard prevents a future color/all transition and checks the stable nav text against both endpoint surfaces.

Stable light-theme contrast: normal text `#5b6c76` is 5.45:1 on white and 4.82:1 on the active pale background; active text `#127868` is 5.37:1 on white and 4.74:1 on the pale background.

Exact PR head `8af075d01fd3178857e6aafe867d5f091a3525e6` passed CI run [41599111532](https://github.com/pooya16pg-spec/orbit/actions/runs/41599111532): all 4 jobs and 14/14 Playwright tests, including the full 12-view dark/light axe matrix. Security run [41599111683](https://github.com/pooya16pg-spec/orbit/actions/runs/41599111683) passed all 13 jobs. The manual keyboard, rendered-state, and real NVDA/VoiceOver/TalkBack acceptance items above remain open.


## M01 interaction-state coverage — merged PR #114 (2026-11-03)

PR #114 was squash-merged into `main` as `91f60ba15f9df37095c92d12e58e056589fec455`. It extends the rendered browser/axe checks without changing axe rules or reducing contrast thresholds.

New browser coverage:
- Keyboard-focus the active Overview navigation link to exercise the real `:focus-visible` indicator, hover Services, and run axe in both dark and light themes after transitions settle.
- Hold the API metadata request at a deterministic test gate; assert that the connecting label is announced and the submit button is disabled, then run axe against the loading form.
- Release the request with a structured HTTP 503 response; assert the alert and restored submit control, then run axe against the visible error state.

The exact PR head `0f8d87245840d638d2a6ca886180683848282ca2` passed CI [41603839829](https://github.com/pooya16pg-spec/orbit/actions/runs/41603839829): all 4 jobs and **16/16 Playwright tests** (42.2 seconds), including the prior 12-view dark/light axe matrix, focus/hover states, loading/error states, Go tests/race/vet/build, PostgreSQL backup/restore, and the full Docker observability E2E. Security [41603839542](https://github.com/pooya16pg-spec/orbit/actions/runs/41603839542) passed all **13/13 jobs** including secret/dependency scans, image scans, and SBOMs.

This closes the added automated interaction-state coverage. It does not replace manual keyboard-only acceptance or real NVDA/VoiceOver/TalkBack sessions. M01 remains open for human review and no WCAG conformance or screen-reader certification is claimed.

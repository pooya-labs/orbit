# ORBIT accessibility review

## Status

The keyboard and landmark baseline is implemented on `feat/m01-accessibility-review`. The complete M01 accessibility acceptance item remains open because actual assistive-technology and visual contrast checks require manual testing.

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

Latest code head: `f9f170abf785a68396a5de2d8081da432b2264cf` on `feat/m01-accessibility-review` (PR #109).

The web job in [CI run 41577030551](https://github.com/pooya16pg-spec/orbit/actions/runs/41577030551) passed on this head. It completed the static accessibility checks, theme contrast guard, TypeScript typecheck, production build, generated-HTML audit, and all **13 Playwright tests**. The source accessibility contract passed **68 checks**; the generated-HTML audit passed for **5 pages / 791 elements**.

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
11. All primary dashboard views expose an announced heading and pass the axe scan.
12. Invalid API base paths are announced as an accessible error.
13. Overview reflows at a 320 CSS-pixel viewport without page-level horizontal overflow.

The same CI run's Go API tests/build and workflow action-pin policy jobs also passed. The local telemetry pipeline end-to-end job was still running when this snapshot was recorded; overall CI must not be described as fully green until that job completes. [Security run 41577030522](https://github.com/pooya16pg-spec/orbit/actions/runs/41577030522) passed all **13 jobs**, including dependency checks, full-history secret scanning, container-image scans, and the Tempo SBOM upload. [Pages-source verification 41577030465](https://github.com/pooya16pg-spec/orbit/actions/runs/41577030465) passed.

The latest automated evidence is tied to the exact application/test-code head above. A subsequent documentation-only update does not change that tested code. The PR remains open and unmerged; `main` is unchanged.


The rendered axe scans found and helped correct low-contrast secondary text in dark Metrics/Overview, several light-theme badge/status/avatar colors, and a workspace-switcher visible-label mismatch. The regression script now explicitly guards the affected dark and light selectors. The final current-head run passed the browser interactions and all axe scans with no reported violations in the scanned states.

This suite tests the built Chromium page, not NVDA, VoiceOver, or TalkBack. It does not establish WCAG conformance or replace the manual acceptance below.

## Manual acceptance still required

Before closing the M01 accessibility review, test the deployed and local application with:

1. Keyboard only: skip link, every primary view, forms and validation errors, drawer open/close, Escape, tab order, focus restoration, zoom/reflow, both LTR and RTL layouts.
2. Screen readers: NVDA with Firefox or Edge on Windows; VoiceOver with Safari on macOS/iOS; TalkBack with Chrome on Android. Confirm navigation landmarks, headings, active view, form labels, loading/error/success announcements, table/list structure, and chart alternatives.
3. Visual access: text and control contrast in dark and light themes, focus visibility, status information that may rely on color, and reduced-motion behavior.
4. Control audit: confirm each visible control has a real action or is clearly presented as unavailable; verify that the Notifications control opens the Notifications view, and review the unavailable workspace, settings, incident-options, and help affordances.

Record browser/OS/assistive-technology versions, findings, and remediations here. Do not claim screen-reader support or WCAG conformance solely from passing static checks or the automated Chromium/axe suite.

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
- The live Metrics time-series chart has an expandable native data table with localized time, series, and request-rate columns; its caption identifies the selected service and time window. The Overview demo chart remains illustrative and is still part of manual chart-alternative acceptance.
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

These checks are targeted regression guards. The palette script does not sample every computed color/background combination at runtime, and none of the scripts is an axe scan, browser accessibility-tree inspection, a full contrast audit, WCAG conformance report, or screen-reader test.

## Manual acceptance still required

Before closing the M01 accessibility review, test the deployed and local application with:

1. Keyboard only: skip link, every primary view, forms and validation errors, drawer open/close, Escape, tab order, focus restoration, zoom/reflow, both LTR and RTL layouts.
2. Screen readers: NVDA with Firefox or Edge on Windows; VoiceOver with Safari on macOS/iOS; TalkBack with Chrome on Android. Confirm navigation landmarks, headings, active view, form labels, loading/error/success announcements, table/list structure, and chart alternatives.
3. Visual access: text and control contrast in dark and light themes, focus visibility, status information that may rely on color, and reduced-motion behavior.
4. Control audit: confirm each visible control has a real action or is clearly presented as unavailable; verify that the Notifications control opens the Notifications view, and review the unavailable workspace, settings, incident-options, and help affordances.

Record browser/OS/assistive-technology versions, findings, and remediations here. Do not claim screen-reader support or WCAG conformance solely from a passing typecheck/build or this static contract check.

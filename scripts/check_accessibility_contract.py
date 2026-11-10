#!/usr/bin/env python3
"""Guard the core keyboard-navigation and landmark accessibility contract."""

from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]


def require(source: str, fragment: str, description: str) -> None:
    if fragment not in source:
        print(f"FAIL: {description}", file=sys.stderr)
        raise SystemExit(1)
    print(f"PASS: {description}")


page = (ROOT / "app/page.tsx").read_text(encoding="utf-8")
styles = (ROOT / "app/globals.css").read_text(encoding="utf-8")
demo = (ROOT / "lib/orbit/demo.ts").read_text(encoding="utf-8")
i18n = (ROOT / "lib/orbit/i18n.ts").read_text(encoding="utf-8")
telemetry = (ROOT / "app/components/TelemetryWorkspace.tsx").read_text(encoding="utf-8")

checks = [
    (page, '<a className="skip-link" href="#main-content">', "a first-in-order skip link targets the main landmark"),
    (page, '<main className="content-wrap" id="main-content" tabIndex={-1}>', "the primary page content has a focusable main landmark"),
    (page, '<nav className="sidebar-scroll" id="primary-navigation" aria-label={tr("primaryNavigation")}>', "primary navigation exposes a named navigation landmark"),
    (page, 'aria-current={activeNav === label ? "page" : undefined}', "the active navigation item is announced as the current page"),
    (page, '<h1 aria-live="polite" aria-atomic="true">', "client-side view changes announce the updated page heading"),
    (page, '<div className="range-switcher" role="group" aria-label={tr("timeRange")}>', "the time-range controls expose a correctly named group"),
    (page, "aria-pressed={range === item}", "the selected time-range option is announced to assistive technology"),
    (i18n, 'timeRange: "Time range"', "the time-range group label is localized in English"),
    (i18n, 'timeRange: "بازهٔ زمانی"', "the time-range group label is localized in Persian"),
    (telemetry, '<label className="telemetry-service-select telemetry-metric-select">', "the metric catalogue selector uses a native labelled form control"),
    (telemetry, 'aria-label={tr("metricLabel")}', "the metric selector has a localized accessible name"),
    (i18n, 'metricLabel: "METRIC"', "the metric selector label is localized in English"),
    (i18n, 'metricLabel: "شاخص"', "the metric selector label is localized in Persian"),
    (i18n, 'successfulRequestRate: "SUCCESSFUL REQUEST RATE"', "the successful request-rate option is localized in English"),
    (i18n, 'successfulRequestRate: "نرخ درخواست موفق"', "the successful request-rate option is localized in Persian"),
    (i18n, 'failedRequestRate: "FAILED REQUEST RATE"', "the failed request-rate option is localized in English"),
    (i18n, 'failedRequestRate: "نرخ درخواست ناموفق"', "the failed request-rate option is localized in Persian"),
    (telemetry, '<details className="telemetry-data-table">', "the live Metrics chart provides an expandable tabular data alternative"),
    (telemetry, '<caption>{tr("chartDataCaption").replace("{metric}", selectedMetricLabel).replace("{service}", selectedService).replace("{window}", windowRange)}</caption>', "the metric data table caption names the selected metric, service, and time window"),
    (telemetry, '<th scope="col">{tr("chartTime")}', "the metric data table exposes semantic column headers"),
    (telemetry, 'formatClock(row.timestamp * 1000, language)', "metric sample timestamps are presented in the selected locale"),
    (telemetry, "formatRate(row.value, language)", "metric sample values are formatted for the selected locale"),
    (i18n, 'showChartData: "View chart data"', "the metric data-table affordance is localized in English"),
    (i18n, 'showChartData: "نمایش داده‌های نمودار"', "the metric data-table affordance is localized in Persian"),
    (i18n, 'chartDataCaption: "{metric} samples for {service} across {window}"', "the metric data caption is localized in English"),
    (i18n, 'chartDataCaption: "نمونه‌های {metric} برای {service} در بازهٔ {window}"', "the metric data caption is localized in Persian"),
    (page, '<details className="telemetry-data-table overview-chart-data-table">', "the Overview demo chart exposes an expandable illustrative data alternative"),
    (page, '<caption>{tr("overviewChartDataCaption").replace("{range}", demoChartRangeLabel)}</caption>', "the Overview chart data table explains that values are illustrative and identifies the selected range"),
    (page, 'demoChartTickLabels.map((label, index)', "the Overview chart time axis follows the selected time window"),
    (page, 'demoChartTimeAt(sample.elapsedFraction)', "Overview chart table timestamps follow the selected time window and locale"),
    (page, "sample.requestsPerMinute", "Overview chart data table reads values from the chart's sample fixture"),
    (demo, "export const chartSamples =", "the Overview chart and table share one illustrative sample fixture"),
    (i18n, 'showOverviewChartData: "View illustrative chart data"', "the Overview chart data disclosure is localized in English"),
    (i18n, 'showOverviewChartData: "نمایش داده‌های نمایشی نمودار"', "the Overview chart data disclosure is localized in Persian"),
    (i18n, 'overviewChartDataCaption: "Illustrative request-rate samples across the selected {range} window.', "the Overview chart data caption identifies illustrative values in English"),
    (i18n, 'overviewChartDataCaption: "نمونه‌های نمایشی نرخ درخواست در بازهٔ انتخاب‌شدهٔ {range}.', "the Overview chart data caption identifies illustrative values in Persian"),
    (page, 'tr("overviewChartValue")', "the Overview data table identifies its request-per-minute units"),
    (i18n, 'overviewChartValue: "Requests / min (illustrative)"', "the Overview data-table unit is localized in English"),
    (i18n, 'overviewChartValue: "درخواست در دقیقه (نمایشی)"', "the Overview data-table unit is localized in Persian"),
    (page, "aria-pressed={range === item}", "the selected overview time range is announced"),
    (ROOT.joinpath("app/components/IncidentWorkspace.tsx").read_text(encoding="utf-8"), 'aria-pressed={filter === "active"}', "incident filters expose their selected state"),
    (ROOT.joinpath("app/components/IncidentWorkspace.tsx").read_text(encoding="utf-8"), "aria-pressed={selectedIncidentId === incident.id}", "the selected incident is announced"),
    (ROOT.joinpath("app/components/AlertWorkspace.tsx").read_text(encoding="utf-8"), 'aria-pressed={filter === "active"}', "alert filters expose their selected state"),
    (ROOT.joinpath("app/components/NotificationWorkspace.tsx").read_text(encoding="utf-8"), "aria-pressed={filter === item.value}", "notification delivery filters expose their selected state"),
    (ROOT.joinpath("app/components/SLOWorkspace.tsx").read_text(encoding="utf-8"), "aria-pressed={selectedSloId === item.id}", "the selected SLO objective is announced"),
    (page, 'aria-expanded={mobileNavOpen} aria-controls="primary-navigation"', "the mobile navigation trigger exposes its expanded state and controlled region"),
    (page, 'event.key === "Escape"', "Escape closes the mobile navigation"),
    (page, 'event.key !== "Tab"', "the mobile navigation handles keyboard tab movement"),
    (page, "last.focus();", "the mobile navigation keeps reverse tab movement inside the drawer"),
    (page, "first.focus();", "the mobile navigation keeps forward tab movement inside the drawer"),
    (page, "mobileMenuButtonRef.current?.focus()", "closing the drawer restores focus to its trigger"),
    (styles, ":focus-visible", "interactive controls have a visible keyboard focus indicator"),
    (styles, 'html[data-theme="light"] :is(button, a, input, select, textarea, [tabindex]):focus-visible { outline-color: #087b6a; }', "light theme keeps a distinct focus indicator against pale surfaces"),
    (styles, ".skip-link:focus", "the skip link becomes visible when focused"),
    (styles, "visibility: hidden", "the closed mobile drawer is removed from keyboard navigation"),
    (styles, "@media (prefers-reduced-motion: reduce)", "reduced-motion preferences are respected"),
    (styles, ".telemetry-data-table-scroll", "the metric data alternative has a horizontally scrollable narrow-screen container"),
    (page, '<div className="telemetry-data-table-scroll" role="region" aria-label={tr("overviewChartDataCaption").replace("{range}", demoChartRangeLabel)} tabIndex={0}>', "the Overview chart table scroll region is named and keyboard focusable"),
    (telemetry, '<div className="telemetry-data-table-scroll" role="region" aria-label={tr("chartDataCaption").replace("{metric}", selectedMetricLabel).replace("{service}", selectedService).replace("{window}", windowRange)} tabIndex={0}>', "the live Metrics table scroll region is named for the selected metric, service, and time window"),
    (styles, ".telemetry-data-table caption", "the metric data alternative styles a native table caption"),
    (i18n, 'primaryNavigation: "Primary navigation"', "the primary navigation label is localized in English"),
    (i18n, 'primaryNavigation: "پیمایش اصلی"', "the primary navigation label is localized in Persian"),
    (i18n, 'skipToMainContent: "Skip to main content"', "the skip link is localized in English"),
    (i18n, 'skipToMainContent: "رفتن به محتوای اصلی"', "the skip link is localized in Persian"),
    (page, 'className="workspace-switcher preview-unavailable" disabled', "the workspace switcher is not exposed as an inert action"),
    (page, '<button type="button" className="workspace-switcher preview-unavailable" disabled title={tr("notAvailableInPreview")}>', "the workspace switcher derives its accessible name from its visible text"),
    (page, '<div className="workspace-avatar" aria-hidden="true">O</div>', "the decorative workspace avatar is hidden from the accessibility tree"),
    (page, 'aria-expanded={utilityDialog === "settings"}', "Settings opens a working, state-announced preferences dialog"),
    (page, 'aria-expanded={utilityDialog === "help"}', "Help opens a working, state-announced support dialog"),
    (page, 'role="dialog"', "Settings and Help expose a dialog role"),
    (page, 'aria-modal="true"', "utility dialogs are announced as modal"),
    (page, 'utilityDialogCloseButtonRef.current?.focus()', "opening a dialog moves focus to its close control"),
    (page, 'focusable[0]', "utility dialogs contain keyboard focus"),
    (page, 'trigger?.focus()', "closing a dialog restores focus to its trigger"),
    (page, 'href="https://pooya-labs.github.io/pooya-labs-platform/"', "sidebar owner link opens the Pooya Labs Platform website"),
    (page, 'href="https://github.com/pooya-labs"', "footer creator profile links use the current GitHub account"),
    (page, 'href="https://pooya-labs.github.io/pooya-labs-platform/"', "project owner link opens the Pooya Labs Platform website"),
    (page, "const ownerMonogramSrc = `${process.env.NEXT_PUBLIC_ORBIT_BASE_PATH ?? \"\"}/brand/pooya-monogram.webp?v=2026-11-10`", "the official owner monogram is cache-versioned and follows the GitHub Pages base path"),
    (page, "src={ownerMonogramSrc}", "the official owner monogram is used in the dashboard"),
    (page, 'className="icon-button subtle-more preview-unavailable" disabled', "the unavailable incident-options control is explicitly disabled"),
    (page, 'onClick={() => chooseNav("Notifications")}', "the notification button opens the implemented Notifications view"),
    (page, '<a className="help-button" href="https://github.com/pooya-labs/orbit/tree/main/docs"', "the documentation affordance has a real destination"),
    (styles, 'html[data-theme="light"] .profile-avatar { color: #ffffff; background: #101820; border-color: #2b3b43; }', "owner monogram retains its dark contrast tile in light mode"),
    (styles, ".preview-unavailable:disabled", "remaining unavailable preview controls have a visible disabled treatment"),
    (styles, ".utility-dialog-backdrop", "Settings and Help dialogs have a responsive modal layout"),
    (i18n, 'settingsDialogTitle: "Workspace settings"', "Settings dialog copy is localized in English"),
    (i18n, 'settingsDialogTitle: "تنظیمات فضای کاری"', "Settings dialog copy is localized in Persian"),
    (i18n, 'helpDialogTitle: "Help & support"', "Help dialog copy is localized in English"),
    (i18n, 'helpDialogTitle: "راهنما و پشتیبانی"', "Help dialog copy is localized in Persian"),
    (i18n, 'notAvailableInPreview: "Not available in this preview"', "unavailable controls are described in English"),
    (i18n, 'notAvailableInPreview: "در این نسخهٔ پیش‌نمایش در دسترس نیست"', "unavailable controls are described in Persian"),
]

for source, fragment, description in checks:
    require(source, fragment, description)

if '<main className="app-shell"' in page:
    print("FAIL: the application shell must not replace the page's main landmark", file=sys.stderr)
    raise SystemExit(1)

print(f"Accessibility contract passed ({len(checks) + 1} checks).")

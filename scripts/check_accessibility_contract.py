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
i18n = (ROOT / "lib/orbit/i18n.ts").read_text(encoding="utf-8")

checks = [
    (page, '<a className="skip-link" href="#main-content">', "a first-in-order skip link targets the main landmark"),
    (page, '<main className="content-wrap" id="main-content" tabIndex={-1}>', "the primary page content has a focusable main landmark"),
    (page, '<nav className="sidebar-scroll" id="primary-navigation" aria-label={tr("primaryNavigation")}>', "primary navigation exposes a named navigation landmark"),
    (page, 'aria-current={activeNav === label ? "page" : undefined}', "the active navigation item is announced as the current page"),
    (page, '<h1 aria-live="polite" aria-atomic="true">', "client-side view changes announce the updated page heading"),
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
    (i18n, 'primaryNavigation: "Primary navigation"', "the primary navigation label is localized in English"),
    (i18n, 'primaryNavigation: "پیمایش اصلی"', "the primary navigation label is localized in Persian"),
    (i18n, 'skipToMainContent: "Skip to main content"', "the skip link is localized in English"),
    (i18n, 'skipToMainContent: "رفتن به محتوای اصلی"', "the skip link is localized in Persian"),
]

for source, fragment, description in checks:
    require(source, fragment, description)

if '<main className="app-shell"' in page:
    print("FAIL: the application shell must not replace the page's main landmark", file=sys.stderr)
    raise SystemExit(1)

print(f"Accessibility contract passed ({len(checks) + 1} checks).")

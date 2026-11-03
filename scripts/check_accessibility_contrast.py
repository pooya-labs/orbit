#!/usr/bin/env python3
"""Check ORBIT's base palette and explicit light-theme text colors for contrast.

This is a focused palette regression check, not a complete rendered-page
contrast audit or a WCAG conformance claim.
"""

from __future__ import annotations

from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
CSS_PATH = ROOT / "app" / "globals.css"

SURFACE_TINTS = (
    "#f5f8f9", "#e4f4ef", "#eff8f5", "#fbeee9", "#fff5e7",
    "#eaf6ef", "#e9f5f7", "#f0edfa", "#f2f6f7", "#eef4f5", "#f3f7f8",
)
RULE_PATTERN = re.compile(r"([^{}]+)\{([^{}]*)\}", re.DOTALL)
HEX_COLOR_PATTERN = re.compile(
    r"(?<![-\w])color\s*:\s*(#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?)\b"
)


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def normalize_hex(color: str) -> str:
    if len(color) == 4:
        return "#" + "".join(character * 2 for character in color[1:]).lower()
    return color.lower()


def declarations(block: str) -> dict[str, str]:
    return {
        name: value.lower()
        for name, value in re.findall(
            r"(--[a-zA-Z0-9_-]+)\s*:\s*(#[0-9a-fA-F]{6})\b",
            block,
        )
    }


def luminance(color: str) -> float:
    color = normalize_hex(color)
    channels = [int(color[index:index + 2], 16) / 255 for index in (1, 3, 5)]
    linear = [
        value / 12.92 if value <= 0.04045 else ((value + 0.055) / 1.055) ** 2.4
        for value in channels
    ]
    return 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2]


def contrast_ratio(foreground: str, background: str) -> float:
    lighter = max(luminance(foreground), luminance(background))
    darker = min(luminance(foreground), luminance(background))
    return (lighter + 0.05) / (darker + 0.05)


def minimum_ratio(foreground: str, backgrounds: tuple[str, ...]) -> float:
    return min(contrast_ratio(foreground, background) for background in backgrounds)


def theme_block(css: str, selector: str, label: str) -> str:
    match = re.search(re.escape(selector) + r"\s*\{([^}]*)\}", css)
    if not match:
        fail(f"missing {label} token block")
    return match.group(1)


def check_theme(label: str, tokens: dict[str, str]) -> tuple[str, ...]:
    required = ("--bg", "--panel", "--panel-raised", "--panel-soft")
    missing = [token for token in (*required, "--text", "--muted", "--subtle", "--mint") if token not in tokens]
    if missing:
        fail(f"{label} palette is missing token(s): {', '.join(missing)}")
    surfaces = tuple(tokens[key] for key in required)
    for token in ("--text", "--muted", "--subtle", "--mint"):
        foreground = tokens[token]
        ratio = minimum_ratio(foreground, surfaces)
        if ratio < 4.5:
            fail(f"{label} {token} contrast is {ratio:.2f}:1 (needs 4.5:1)")
        print(f"PASS: {label} {token} contrast >= 4.5:1 (minimum {ratio:.2f}:1)")
    return surfaces


def main() -> int:
    try:
        css = CSS_PATH.read_text(encoding="utf-8")
    except OSError as error:
        fail(f"cannot read {CSS_PATH}: {error}")

    root = declarations(theme_block(css, ":root", "dark theme"))
    light = declarations(theme_block(css, 'html[data-theme="light"]', "light theme"))
    dark_surfaces = check_theme("dark", root)
    light_surfaces = tuple(dict.fromkeys(check_theme("light", light) + SURFACE_TINTS))

    if ":focus-visible" not in css or "outline: 2px solid var(--mint)" not in css:
        fail("shared dark-theme keyboard focus indicator is missing")
    light_focus = re.search(
        r'html\[data-theme="light"\][^{]*:focus-visible\s*\{[^}]*outline-color\s*:\s*(#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?)',
        css,
    )
    if not light_focus:
        fail("light-theme focus outline color is missing")
    focus_color = normalize_hex(light_focus.group(1))
    for name, surfaces, color in (
        ("dark", dark_surfaces, root["--mint"]),
        ("light", light_surfaces, focus_color),
    ):
        ratio = minimum_ratio(color, surfaces)
        if ratio < 3.0:
            fail(f"{name} keyboard focus outline contrast is {ratio:.2f}:1 (needs 3.0:1)")
        print(f"PASS: {name} keyboard focus outline contrast >= 3.0:1 (minimum {ratio:.2f}:1)")

    primary_rule = next(
        (
            body for selector, body in RULE_PATTERN.findall(css)
            if 'html[data-theme="light"]' in selector
            and ".button-primary" in selector
            and re.search(r"background\s*:", body)
        ),
        "",
    )
    primary_background_match = re.search(
        r"background\s*:\s*(#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?)",
        primary_rule,
    )
    if not primary_background_match:
        fail("light primary button background is missing")
    primary_background = normalize_hex(primary_background_match.group(1))

    checked_light_colors = 0
    for match in RULE_PATTERN.finditer(css):
        selector, body = match.group(1), match.group(2)
        if 'html[data-theme="light"]' not in selector:
            continue
        for color_match in HEX_COLOR_PATTERN.finditer(body):
            foreground = normalize_hex(color_match.group(1))
            if foreground == "#ffffff":
                if ".button-primary" not in selector:
                    fail(f"unexpected white text in light theme selector: {selector.strip()}")
                if contrast_ratio(foreground, primary_background) < 4.5:
                    fail("light primary button white text does not reach 4.5:1")
                continue
            checked_light_colors += 1
            ratio = minimum_ratio(foreground, light_surfaces)
            if ratio < 4.5:
                fail(
                    f"light-theme text color {foreground} in {selector.strip()[:100]} "
                    f"has minimum contrast {ratio:.2f}:1"
                )

    if checked_light_colors == 0:
        fail("no explicit light-theme text colors were checked")
    print(f"PASS: {checked_light_colors} explicit light-theme text colors meet 4.5:1 across pale surfaces.")

    # axe-core's first rendered Chromium scan found secondary dark-theme copy
    # below 4.5:1 even though the base tokens passed. Keep each affected
    # selector tied to the tested --subtle token and its actual surface.
    dark_secondary_text = (
        (".nav-group-label", "#0d1319"),
        (".global-search input::placeholder", "#10161d"),
        (".metric-detail", "#10161d"),
        (".chart-summary-change span", "#10161d"),
        (".chart-y-labels", "#10161d"),
        (".chart-x-labels", "#10161d"),
        (".chart-footnote", "#10161d"),
        (".service-table-head", "#10161d"),
        (".service-name-cell span", "#10161d"),
        (".panel-footer", "#10161d"),
        (".incident-meta", "#10161d"),
        (".page-footer", "#090d12"),
    )
    dark_rules = RULE_PATTERN.findall(css)
    for selector, surface in dark_secondary_text:
        rules = [
            body
            for rule_selector, body in dark_rules
            if selector in rule_selector
            and 'html[data-theme="light"]' not in rule_selector
        ]
        if not rules or not any(
            re.search(r"color\s*:\s*var\(--subtle\)", body)
            for body in rules
        ):
            fail(f"dark secondary text selector {selector} must use --subtle")
        ratio = contrast_ratio(root["--subtle"], surface)
        if ratio < 4.5:
            fail(
                f"dark secondary text {selector} contrast is {ratio:.2f}:1 "
                f"on {surface} (needs 4.5:1)"
            )
        print(
            f"PASS: dark secondary text {selector} contrast >= 4.5:1 "
            f"(minimum {ratio:.2f}:1)"
        )

    print("This palette check does not replace rendered-state contrast sampling or assistive-technology testing.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

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
    "#f5f8f9",
    "#e4f4ef",
    "#eff8f5",
    "#fbeee9",
    "#fff5e7",
    "#eaf6ef",
    "#e9f5f7",
    "#f0edfa",
    "#f2f6f7",
    "#eef4f5",
    "#f3f7f8",
)

RULE_PATTERN = re.compile(r"([^{}]+)\{([^{}]*)\}", re.DOTALL)
HEX_COLOR_PATTERN = re.compile(r"(?<![-\w])color\s*:\s*(#[0-9a-fA-F]{6})\b")


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def declarations(block: str) -> dict[str, str]:
    return {
        name: value.lower()
        for name, value in re.findall(
            r"(--[a-zA-Z0-9_-]+)\s*:\s*(#[0-9a-fA-F]{6})\b",
            block,
        )
    }


def color_value(value: str, label: str) -> str:
    if not re.fullmatch(r"#[0-9a-f]{6}", value):
        fail(f"{label} must be a six-digit hex color, got {value!r}")
    return value


def normalize_hex(color: str) -> str:
    if len(color) == 4:
        return "#" + "".join(character * 2 for character in color[1:]).lower()
    return color.lower()


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


def check_theme(
    label: str,
    tokens: dict[str, str],
    minimum_text: float = 4.5,
) -> tuple[str, ...]:
    surfaces = tuple(tokens[key] for key in (
        "--bg", "--panel", "--panel-raised", "--panel-soft"
    ))
    for token in ("--text", "--muted", "--subtle", "--mint"):
        foreground = color_value(tokens.get(token, ""), f"{label} {token}")
        ratio = minimum_ratio(foreground, surfaces)
        if ratio < minimum_text:
            fail(f"{label} {token} contrast is {ratio:.2f}:1 (needs {minimum_text:.1f}:1)")
        print(f"PASS: {label} {token} contrast >= {minimum_text:.1f}:1 (minimum {ratio:.2f}:1)")
    return surfaces


def main() -> int:
    try:
        css = CSS_PATH.read_text(encoding="utf-8")
    except OSError as error:
        fail(f"cannot read {CSS_PATH}: {error}")

    root = declarations(theme_block(css, ":root", "dark theme"))
    light = declarations(theme_block(css, 'html[data-theme="light"]', "light theme"))

    dark_surfaces = check_theme("dark", root)
    light_surfaces = check_theme("light", light)
    light_surfaces = tuple(dict.fromkeys(light_surfaces + SURFACE_TINTS))

    if ":focus-visible" not in css:
        fail("shared keyboard focus indicator is missing")
    if "outline: 2px solid var(--mint)" not in css:
        fail("dark-theme focus outline must use the tested mint token")

    light_focus = re.search(
        r'html\[data-theme="light"\][^{]*:focus-visible\s*\{[^}]*outline-color\s*:\s*(#[0-9a-fA-F]{6})',
        css,
    )
    if not light_focus:
        fail("light-theme focus outline color is missing")
    focus_color = normalize_hex(light_focus.group(1))
    for name, surfaces in (("dark", dark_surfaces), ("light", light_surfaces)):
        ratio = minimum_ratio(
            root["--mint"] if name == "dark" else focus_color,
            surfaces,
        )
        if ratio < 3.0:
            fail(f"{name} keyboard focus outline contrast is {ratio:.2f}:1 (needs 3.0:1)")
        print(f"PASS: {name} keyboard focus outline contrast >= 3.0:1 (minimum {ratio:.2f}:1)")

    primary_rule = next(
        (
            body for selector, body in RULE_PATTERN.findall(css)
            if 'html[data-theme="light"]' in selector and ".button-primary" in selector
            and re.search(r"background\\s*:", body)
        ),
        "",
    )
    primary_background_match = re.search(r"background\\s*:\\s*(#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?)", primary_rule)
    if not primary_background_match:
        fail("light primary button background is missing")
    primary_background = normalize_hex(primary_background_match.group(1))

    checked_light_colors = 0
    for match in RULE_PATTERN.finditer(css):
        selector, body = match.group(1), match.group(2)
        if 'html[data-theme="light"]' not in selector:
            continue
        for color_match in HEX_COLOR_PATTERN.finditer(body):
            foreground = color_match.group(1).lower()
            # White is reserved for the filled primary action. Verify it
            # against that action's explicit background instead of pale cards.
            if foreground == "#ffffff":
                if ".button-primary" not in selector:
                    fail(f"unexpected white text in light theme selector: {selector.strip()}")
                background = re.search(r"background\s*:\s*(#[0-9a-fA-F]{6})", body)
                if not background or contrast_ratio(foreground, background.group(1).lower()) < 4.5:
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
    print("This palette check does not replace rendered-state contrast sampling or assistive-technology testing.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

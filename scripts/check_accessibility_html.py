#!/usr/bin/env python3
"""Audit generated static HTML for core accessible structure and names.

This dependency-free structural regression check does not emulate a browser's
accessibility tree and is not a WCAG conformance or screen-reader test.
"""

from __future__ import annotations

from html.parser import HTMLParser
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "out"
VOID_ELEMENTS = {
    "area", "base", "br", "col", "embed", "hr", "img", "input", "link",
    "meta", "param", "source", "track", "wbr",
}
INTERACTIVE_ROLES = {
    "button", "link", "checkbox", "radio", "switch", "tab", "menuitem",
    "option", "slider", "spinbutton", "textbox", "combobox",
}


class Element:
    def __init__(self, tag: str, attrs: dict[str, str | None], ancestors: list["Element"]):
        self.tag = tag
        self.attrs = attrs
        self.ancestors = ancestors
        self.text: list[str] = []

    @property
    def text_content(self) -> str:
        return " ".join(" ".join(self.text).split())


class DocumentAudit(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.stack: list[Element] = []
        self.elements: list[Element] = []
        self.ids: dict[str, list[Element]] = {}
        self.duplicates: list[str] = []
        self.skip_targets: list[str] = []

    def handle_starttag(self, tag: str, raw_attrs: list[tuple[str, str | None]]) -> None:
        attrs = {key.lower(): value for key, value in raw_attrs}
        node = Element(tag.lower(), attrs, list(self.stack))
        self.elements.append(node)
        element_id = attrs.get("id")
        if element_id:
            existing = self.ids.setdefault(element_id, [])
            existing.append(node)
            if len(existing) > 1 and element_id not in self.duplicates:
                self.duplicates.append(element_id)
        if tag.lower() == "a" and "skip-link" in (attrs.get("class") or "").split():
            href = attrs.get("href") or ""
            if href.startswith("#") and len(href) > 1:
                self.skip_targets.append(href[1:])
        if tag.lower() not in VOID_ELEMENTS:
            self.stack.append(node)

    def handle_startendtag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        self.handle_starttag(tag, attrs)
        self.handle_endtag(tag)

    def handle_endtag(self, tag: str) -> None:
        normalized = tag.lower()
        for index in range(len(self.stack) - 1, -1, -1):
            if self.stack[index].tag == normalized:
                del self.stack[index:]
                return

    def handle_data(self, data: str) -> None:
        if not data.strip():
            return
        for node in self.stack:
            node.text.append(data)


def accessible_name(node: Element, id_text: dict[str, str]) -> str:
    aria_label = (node.attrs.get("aria-label") or "").strip()
    if aria_label:
        return aria_label
    labelledby = (node.attrs.get("aria-labelledby") or "").split()
    if labelledby:
        parts = [id_text.get(reference, "") for reference in labelledby]
        name = " ".join(part for part in parts if part).strip()
        if name:
            return name
    if node.text_content:
        return node.text_content
    alt = (node.attrs.get("alt") or "").strip()
    if alt:
        return alt
    return (node.attrs.get("title") or "").strip()


def audit_file(path: Path) -> tuple[list[str], int]:
    parser = DocumentAudit()
    try:
        parser.feed(path.read_text(encoding="utf-8"))
        parser.close()
    except (OSError, UnicodeError) as error:
        return [f"{path.relative_to(ROOT)}: unable to parse HTML: {error}"], 0

    errors: list[str] = []
    prefix = str(path.relative_to(ROOT))
    elements = parser.elements
    id_text = {
        element_id: next((node.text_content for node in nodes if node.text_content), "")
        for element_id, nodes in parser.ids.items()
    }

    html_nodes = [node for node in elements if node.tag == "html"]
    language = html_nodes[0].attrs.get("lang") if html_nodes else None
    if not re.fullmatch(r"[a-zA-Z]{2,3}(?:-[a-zA-Z0-9]{2,8})*", language or ""):
        errors.append(f"{prefix}: document root must declare a valid lang attribute")

    if not any(node.text_content for node in elements if node.tag == "title"):
        errors.append(f"{prefix}: document title is missing or empty")

    mains = [node for node in elements if node.tag == "main" or node.attrs.get("role") == "main"]
    if len(mains) != 1:
        errors.append(f"{prefix}: expected exactly one main landmark, found {len(mains)}")

    if not any(node.text_content for node in elements if node.tag == "h1"):
        errors.append(f"{prefix}: page must expose a non-empty h1 heading")

    navs = [node for node in elements if node.tag == "nav" or node.attrs.get("role") == "navigation"]
    for index, nav in enumerate(navs, start=1):
        if not accessible_name(nav, id_text):
            errors.append(f"{prefix}: navigation landmark {index} has no accessible name")

    if not parser.skip_targets:
        errors.append(f"{prefix}: localized skip link targeting main content is missing")
    else:
        for target in parser.skip_targets:
            if target not in parser.ids:
                errors.append(f"{prefix}: skip link target #{target} does not exist")

    for element_id in parser.duplicates:
        errors.append(f"{prefix}: duplicate id '{element_id}'")

    label_for = {
        (node.attrs.get("for") or ""): node.text_content
        for node in elements
        if node.tag == "label" and node.attrs.get("for") and node.text_content
    }

    for node in elements:
        if node.tag == "img" and "alt" not in node.attrs:
            errors.append(f"{prefix}: img element is missing its alt attribute")
        if node.tag == "iframe" and not ((node.attrs.get("title") or "").strip() or accessible_name(node, id_text)):
            errors.append(f"{prefix}: iframe is missing an accessible title")

        role = (node.attrs.get("role") or "").lower()
        is_interactive = node.tag in {"button", "a", "summary"} or role in INTERACTIVE_ROLES
        if is_interactive:
            if node.tag == "a" and not (node.attrs.get("href") or "").strip():
                errors.append(f"{prefix}: anchor has no href destination")
            if not accessible_name(node, id_text):
                errors.append(f"{prefix}: {node.tag}[role={role or 'native'}] has no accessible name")

        if node.tag in {"input", "select", "textarea"}:
            input_type = (node.attrs.get("type") or "").lower()
            if node.tag == "input" and input_type in {"hidden", "submit", "button", "reset", "image"}:
                continue
            labelled = bool(accessible_name(node, id_text))
            control_id = node.attrs.get("id") or ""
            if control_id and label_for.get(control_id):
                labelled = True
            if any(ancestor.tag == "label" for ancestor in node.ancestors):
                labelled = True
            if not labelled:
                errors.append(f"{prefix}: {node.tag} form control has no associated accessible label")

        if role == "img" and not accessible_name(node, id_text):
            errors.append(f"{prefix}: role=img element has no accessible name")

    for node in elements:
        for reference_attr in ("aria-labelledby", "aria-describedby", "aria-controls"):
            for reference in (node.attrs.get(reference_attr) or "").split():
                if reference not in parser.ids:
                    errors.append(f"{prefix}: {reference_attr} references missing id '{reference}'")

    return errors, len(elements)


def main() -> int:
    if not OUTPUT.is_dir():
        print(f"FAIL: generated output directory not found: {OUTPUT}", file=sys.stderr)
        return 1
    html_files = sorted(OUTPUT.rglob("*.html"))
    if not html_files:
        print(f"FAIL: no generated HTML files found under {OUTPUT}", file=sys.stderr)
        return 1

    failures: list[str] = []
    total_elements = 0
    for html_file in html_files:
        page_failures, element_count = audit_file(html_file)
        failures.extend(page_failures)
        total_elements += element_count

    if failures:
        for failure in failures:
            print(f"FAIL: {failure}", file=sys.stderr)
        print(
            f"Generated HTML audit failed: {len(failures)} issue(s) across {len(html_files)} page(s).",
            file=sys.stderr,
        )
        return 1

    print(
        f"Generated HTML accessibility structure passed: {len(html_files)} page(s), "
        f"{total_elements} elements checked."
    )
    print("This structural audit does not replace browser, contrast, WCAG, or screen-reader testing.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

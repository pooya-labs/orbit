#!/usr/bin/env python3
"""Fail CI when a third-party GitHub Actions reference is not immutably pinned."""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
WORKFLOWS = ROOT / ".github" / "workflows"
USES_RE = re.compile(r"^\s*uses:\s*(?P<target>\S+?)(?:\s+#\s*(?P<comment>.*))?\s*$")
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
VERSION_COMMENT_RE = re.compile(r"^v\d+(?:\.\d+){0,2}(?:[-+][A-Za-z0-9.-]+)?$")


def validate_reference(target: str, comment: str | None, source: Path, line_number: int) -> str | None:
    """Return an error for mutable external refs or invalid local/docker refs."""
    location = f"{source.relative_to(ROOT)}:{line_number}"

    if target.startswith("./"):
        local_path = (ROOT / target).resolve()
        try:
            local_path.relative_to(ROOT.resolve())
        except ValueError:
            return f"{location}: local action path escapes the repository: {target}"
        if not local_path.exists():
            return f"{location}: local action path does not exist: {target}"
        return None

    if target.startswith("docker://"):
        if not re.search(r"@sha256:[0-9a-f]{64}$", target):
            return f"{location}: Docker actions must be pinned by sha256 digest: {target}"
        return None

    repository, separator, revision = target.rpartition("@")
    if not separator or not SHA_RE.fullmatch(revision):
        return f"{location}: external action must use a full 40-character commit SHA: {target}"
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        return f"{location}: external action must use an owner/repository reference: {target}"

    if comment is None or not VERSION_COMMENT_RE.fullmatch(comment.strip()):
        return f"{location}: add a version comment such as '# v4' after the pinned SHA"
    return None


def main() -> int:
    if not WORKFLOWS.is_dir():
        print("No .github/workflows directory found.", file=sys.stderr)
        return 1

    workflow_files = sorted([*WORKFLOWS.glob("*.yml"), *WORKFLOWS.glob("*.yaml")])
    if not workflow_files:
        print("No workflow YAML files found.", file=sys.stderr)
        return 1

    errors: list[str] = []
    checked = 0
    for path in workflow_files:
        for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            match = USES_RE.match(line)
            if not match:
                continue
            checked += 1
            error = validate_reference(
                match.group("target"),
                match.group("comment"),
                path,
                line_number,
            )
            if error:
                errors.append(error)

    if errors:
        print("GitHub Actions pin policy failed:")
        print("\n".join(f"- {error}" for error in errors))
        return 1

    print(f"Verified {checked} GitHub Actions references use immutable pins.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

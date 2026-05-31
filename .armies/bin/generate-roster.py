#!/usr/bin/env python3
"""
generate-roster.py — Build .armies/roster.jsonl and .armies/ROSTER.md
from profile YAML frontmatter.

Usage:
    bin/generate-roster.py           # write both output files
    bin/generate-roster.py --check   # validate only, no output written

Exit codes:
    0 — success (or --check passed)
    1 — schema violation found
"""

import argparse
import glob
import json
import os
import sys
from datetime import datetime, timezone

try:
    import yaml
    _HAS_YAML = True
except ImportError:
    _HAS_YAML = False


# ---------------------------------------------------------------------------
# Frontmatter parsing
# ---------------------------------------------------------------------------

def _parse_yaml_stdlib(text: str) -> dict:
    """Minimal YAML parser for simple key: value frontmatter (no yaml lib)."""
    result: dict = {}
    i = 0
    lines = text.splitlines()
    while i < len(lines):
        line = lines[i]
        # Skip blank / comment lines
        if not line.strip() or line.strip().startswith('#'):
            i += 1
            continue
        # List block: key followed by indented "- value" lines
        if ':' in line and not line.startswith(' '):
            key, _, rest = line.partition(':')
            key = key.strip()
            rest = rest.strip()
            if rest == '' and i + 1 < len(lines) and lines[i + 1].startswith('  -'):
                # multi-value list
                vals = []
                i += 1
                while i < len(lines) and lines[i].startswith('  -'):
                    vals.append(lines[i].lstrip('  -').strip())
                    i += 1
                result[key] = vals
                continue
            else:
                # Scalar — strip surrounding quotes
                v = rest.strip('"\'')
                # Handle inline >-/| block scalars as plain string
                if v in ('>', '|-', '|', '>-'):
                    v = ''
                result[key] = v
        i += 1
    return result


def parse_frontmatter(path: str) -> dict:
    """Return parsed YAML frontmatter dict from a profile file."""
    with open(path, encoding='utf-8') as fh:
        content = fh.read()

    # Split on first two '---' delimiters
    parts = content.split('---', 2)
    if len(parts) < 3:
        raise ValueError(f"No valid YAML frontmatter block in {path}")

    raw = parts[1]

    if _HAS_YAML:
        return yaml.safe_load(raw) or {}
    else:
        return _parse_yaml_stdlib(raw)


# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------

REQUIRED_FIELDS = ['name', 'display_name', 'status', 'branch', 'model', 'description']
VALID_STATUSES = {'active', 'bench'}
VALID_BRANCHES = {
    'Advisory & Coordination',
    'Air Power',
    'Design & Visual',
    'Ground Ops',
    'Intelligence',
    'Naval',
    'Org & Infra',
    'QA & Review',
    'Quality & Audit',
    'Writing & Journalism',
}


def validate(fm: dict, path: str) -> list[str]:
    """Return list of error strings; empty list means valid."""
    errors = []
    for field in REQUIRED_FIELDS:
        if field not in fm or fm[field] is None or fm[field] == '':
            errors.append(f"{path}: missing required field '{field}'")
    if 'status' in fm and fm['status'] not in VALID_STATUSES:
        errors.append(f"{path}: invalid status '{fm['status']}' (expected active|bench)")
    if 'branch' in fm and fm['branch'] not in VALID_BRANCHES:
        errors.append(f"{path}: invalid branch '{fm['branch']}'")
    return errors


# ---------------------------------------------------------------------------
# JSONL record builder
# ---------------------------------------------------------------------------

HOME = os.path.expanduser('~')


def _relative_profile_path(abs_path: str) -> str:
    """Return path relative to $HOME, e.g. .armies/profiles/nimitz.md"""
    rel = os.path.relpath(abs_path, HOME)
    return rel


def _single_line(text: str) -> str:
    """Collapse newlines/extra whitespace in a string to a single line."""
    return ' '.join(str(text).split())


def build_record(fm: dict, profile_path: str) -> dict:
    """Build a canonical roster record dict from frontmatter."""
    # Use ordered dict via insertion order (Python 3.7+)
    return {
        "name": str(fm['name']),
        "display_name": str(fm['display_name']),
        "status": str(fm['status']),
        "branch": str(fm['branch']),
        "model": str(fm['model']),
        "xp": int(fm.get('xp') or 0),
        "rank": str(fm.get('rank') or ''),
        "description": _single_line(fm.get('description') or ''),
        "profile_path": _relative_profile_path(profile_path),
    }


# ---------------------------------------------------------------------------
# ROSTER.md builder
# ---------------------------------------------------------------------------

_TRUNC = 120


def _trunc(text: str, n: int = _TRUNC) -> str:
    if len(text) <= n:
        return text
    return text[:n].rstrip() + '…'


def _md_escape(text: str) -> str:
    """Escape pipe characters so they don't break markdown tables."""
    return text.replace('|', '\\|')


def build_roster_md(records: list[dict], guidance_path: str | None) -> str:
    timestamp = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')

    active = [r for r in records if r['status'] == 'active']
    bench  = [r for r in records if r['status'] == 'bench']

    lines = [
        "# Armies Roster",
        "",
        "*Auto-generated by `bin/generate-roster.py` from `profiles/*.md`. Do not edit by hand — edit the profile and re-run.*",
        "",
        f"**Last generated:** {timestamp}",
        f"**Total:** {len(records)} generals ({len(active)} active, {len(bench)} bench)",
        "",
    ]

    def section(title: str, subset: list[dict]) -> None:
        lines.append(f"## {title} ({len(subset)})")
        lines.append("")

        # Group by branch, sorted alphabetically
        branches_present = sorted({r['branch'] for r in subset})
        for branch in branches_present:
            branch_records = sorted(
                [r for r in subset if r['branch'] == branch],
                key=lambda r: r['display_name'].lower()
            )
            lines.append(f"### {branch}")
            lines.append("")
            lines.append("| Name | Model | XP | Specialization |")
            lines.append("|------|-------|----|----------------|")
            for r in branch_records:
                name = _md_escape(r['display_name'])
                model = r['model']
                xp = str(r['xp'])
                spec = _md_escape(_trunc(r['description']))
                lines.append(f"| {name} | {model} | {xp} | {spec} |")
            lines.append("")

    section("Active", active)
    section("Bench", bench)

    lines.append("---")
    lines.append("")

    if guidance_path and os.path.exists(guidance_path):
        with open(guidance_path, encoding='utf-8') as fh:
            guidance = fh.read().rstrip('\n')
        lines.append(guidance)
        lines.append("")

    return '\n'.join(lines) + '\n'


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    parser = argparse.ArgumentParser(
        description="Generate armies roster files from profile frontmatter."
    )
    parser.add_argument(
        '--check',
        action='store_true',
        help="Validate profiles only; write no output files."
    )
    args = parser.parse_args()

    armies_dir = os.path.join(HOME, '.armies')
    profiles_dir = os.path.join(armies_dir, 'profiles')
    jsonl_out = os.path.join(armies_dir, 'roster.jsonl')
    md_out = os.path.join(armies_dir, 'ROSTER.md')
    guidance_path = os.path.join(armies_dir, 'roster-guidance.md')

    profile_paths = sorted(glob.glob(os.path.join(profiles_dir, '*.md')))
    if not profile_paths:
        print("ERROR: no .md files found in", profiles_dir, file=sys.stderr)
        return 1

    all_errors: list[str] = []
    records: list[dict] = []

    for path in profile_paths:
        try:
            fm = parse_frontmatter(path)
        except Exception as exc:
            all_errors.append(f"{path}: {exc}")
            continue

        errors = validate(fm, path)
        if errors:
            all_errors.extend(errors)
            continue

        records.append(build_record(fm, path))

    if all_errors:
        for err in all_errors:
            print(err, file=sys.stderr)
        return 1

    if args.check:
        # Validation passed; exit cleanly without writing files
        return 0

    # Sort by name for stable diffs
    records.sort(key=lambda r: r['name'])

    # Write JSONL
    jsonl_lines = [json.dumps(r, ensure_ascii=False) for r in records]
    with open(jsonl_out, 'w', encoding='utf-8') as fh:
        fh.write('\n'.join(jsonl_lines) + '\n')

    # Write ROSTER.md
    md_content = build_roster_md(records, guidance_path)
    with open(md_out, 'w', encoding='utf-8') as fh:
        fh.write(md_content)

    return 0


if __name__ == '__main__':
    sys.exit(main())

#!/usr/bin/env python3
"""add-failure-mode.py — append a new failure mode to the global catalog.

Usage: ~/bin/add-failure-mode.py
Requires: ~/docs/failure-modes-standard.md, ~/bin/sync-failure-modes.sh
"""

import re
import shutil
import subprocess
import sys
from pathlib import Path

CATALOG = Path.home() / "docs" / "failure-modes-standard.md"
SYNC    = Path.home() / "bin" / "sync-failure-modes.sh"

CLASSES = {
    "A": "Identity vs. Routing",
    "B": "Install / Config Layout",
    "C": "Portability",
    "D": "Fail-Loud",
    "E": "Thermal / Resource-Role Safety",
    "F": "Repo Hygiene / Canonical Home",
    "G": "Deploy / Verify Discipline",
}


def ask(question, default=None):
    suffix = f" [{default}]" if default else ""
    try:
        val = input(f"\n{question}{suffix}: ").strip()
    except (KeyboardInterrupt, EOFError):
        print("\nAborted.")
        sys.exit(0)
    return val if val else default


def next_fm_id(content):
    ids = re.findall(r'\| (FM-\d+) \|', content)
    if not ids:
        if len(content.strip()) > 200:
            print("WARNING: catalog has content but no FM-IDs found — check table format", file=sys.stderr)
        return "FM-01"
    last_num = max(int(i.split("-")[1]) for i in ids)
    return f"FM-{last_num + 1:02d}"


def append_catalog_row(content, fm_id, class_name, instance, check, auto):
    marker = "🤖" if auto else "Judgment"
    new_row = f"| {fm_id} | {class_name} | {instance} | {check} | {marker} |"
    lines = content.split("\n")
    last_fm = max(
        (i for i, l in enumerate(lines) if re.match(r'\| FM-\d+', l)),
        default=-1
    )
    if last_fm == -1:
        return content + f"\n{new_row}"
    lines.insert(last_fm + 1, new_row)
    return "\n".join(lines)


def append_check_to_class(content, class_letter, check, auto):
    prefix = "🤖 " if auto else ""
    item = f"- {prefix}**{check}**"
    header_pat = re.compile(rf'^### {re.escape(class_letter)}\. ')
    next_header_pat = re.compile(r'^### [A-Z]\.')
    lines = content.splitlines(keepends=True)
    in_section = False
    insert_at = None
    for i, line in enumerate(lines):
        if header_pat.match(line):
            in_section = True
            continue
        if in_section and next_header_pat.match(line):
            insert_at = i
            break
    else:
        if in_section:
            insert_at = len(lines)
    if insert_at is None:
        # Section not found — fallback before Case Studies or at EOF
        if "\n## Case Studies" in content:
            return content.replace("\n## Case Studies", f"\n{item}\n\n## Case Studies", 1)
        return content + f"\n{item}\n"
    lines.insert(insert_at, item + "\n")
    return "".join(lines)


def add_new_class_section(content, letter, name, instance, check, auto):
    prefix = "🤖 " if auto else ""
    section = (
        f"\n### {letter}. {name}\n\n"
        f"{instance}\n\n"
        f"- {prefix}**{check}**\n"
    )
    if "\n## Failure-Mode Catalog" in content:
        return content.replace("\n## Failure-Mode Catalog", section + "\n## Failure-Mode Catalog", 1)
    return content + section


def main():
    if not CATALOG.exists():
        print(f"ERROR: catalog not found at {CATALOG}", file=sys.stderr)
        sys.exit(1)

    content = CATALOG.read_text()

    print("\n─── add-failure-mode ─────────────────────────────────────────────────────")

    instance = ask("Describe the bug (concrete instance, 1–2 sentences)")
    if not instance:
        print("Aborted."); sys.exit(0)

    print("\nBug class:")
    for k, v in CLASSES.items():
        print(f"  {k}) {v}")
    print("  N) New class")
    class_input = ask("Class").upper()

    new_class_name = None
    if class_input == "N":
        new_class_name = ask("New class name (short, title-case)")
        class_input = chr(ord(sorted(CLASSES.keys())[-1]) + 1)
        CLASSES[class_input] = new_class_name

    if class_input not in CLASSES and class_input != "N":
        print(f"Invalid class: {class_input!r}. Choose from {sorted(CLASSES)} or N.", file=sys.stderr)
        sys.exit(1)
    class_name = CLASSES.get(class_input, class_input)
    check = ask("What check catches it? (one sentence)")
    if not check:
        print("Aborted."); sys.exit(0)

    auto = ask("Automatable? (y/n)", default="n").lower() == "y"
    new_item = ask("New check item (not just a new instance)? (y/n)", default="y").lower() == "y"
    issue = ask("GitHub Issue # (blank to skip)", default="")

    fm_id = next_fm_id(content)

    print(f"\n─────────────────────────────────────────────────────────────────────────")
    print(f"Assigned:  {fm_id}")
    print(f"Class:     {class_input} — {class_name}")

    content = append_catalog_row(content, fm_id, class_name, instance, check, auto)
    print(f"✓ Appended {fm_id} to catalog table")

    if new_item:
        if new_class_name:
            content = add_new_class_section(content, class_input, new_class_name,
                                            instance, check, auto)
            print(f"✓ Created new class {class_input} section")
        else:
            content = append_check_to_class(content, class_input, check, auto)
            print(f"✓ Appended check to class {class_input} section")

    if auto:
        print(f"\n⚠️  Automatable — add rg/grep pattern to ~/bin/checkin-lint-core.sh:")
        print(f"   # {fm_id}: {check}")
        print(f"   # Add a _check_<name>() function or extend an existing one.")

    shutil.copy2(CATALOG, CATALOG.with_suffix(".md.bak"))
    CATALOG.write_text(content)
    print(f"✓ Written to {CATALOG}")

    if issue:
        print(f"\nGitHub Issue: #{issue}")

    print(f"\nRunning sync-failure-modes.sh...")
    if SYNC.exists():
        try:
            result = subprocess.run([str(SYNC)])
            if result.returncode != 0:
                print("⚠️  Sync returned non-zero — check output above")
        except PermissionError:
            print(f"⚠️  {SYNC} is not executable — run: chmod +x {SYNC}", file=sys.stderr)
        except OSError as e:
            print(f"⚠️  Could not run sync script: {e}", file=sys.stderr)
    else:
        print(f"⚠️  {SYNC} not found — run manually once installed")

    print(f"\nDone. Commit failure-modes-standard.md + updated checkin-lint-core.sh files.")
    print(f"─────────────────────────────────────────────────────────────────────────")


if __name__ == "__main__":
    main()

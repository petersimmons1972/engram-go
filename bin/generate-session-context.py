#!/usr/bin/env python3
"""
Session startup context generator
Auto-generates fresh MEMORY.md from current state
Called by Claude Code session-start hook
"""

import signal
import subprocess
import sys
from datetime import datetime
from pathlib import Path
import os


def _alarm_handler(signum, frame):
    """Hard cap: exit cleanly if the script runs too long."""
    print("⚠️  generate-session-context: timed out, partial output written", file=sys.stderr)
    sys.exit(0)


signal.signal(signal.SIGALRM, _alarm_handler)
signal.alarm(12)  # 12-second hard cap — hook timeout is 15s

HOME = Path.home()
MEMORY_TEMPLATE = HOME / "bin/memory-template.md"
MEMORY_FILE = HOME / ".claude/projects/-home-psimmons/memory/MEMORY.md"


def run_command(cmd, shell=True, timeout=5):
    """Run shell command and return output, handle errors gracefully."""
    try:
        result = subprocess.run(
            cmd,
            shell=shell,
            capture_output=True,
            text=True,
            timeout=timeout,
            cwd=HOME
        )
        return result.stdout.strip() if result.returncode == 0 else ""
    except (subprocess.TimeoutExpired, Exception):
        return ""


def get_recent_commits():
    """Get compact activity digest: last commit + count of commits in last 7 days."""
    last = run_command(
        'git log -1 --pretty=format:"%ad: %s" --date=short 2>/dev/null'
    )
    count_7d = run_command(
        'git log --all --since="7 days ago" --oneline 2>/dev/null | wc -l'
    ).strip()
    if not last:
        return "No recent commits"
    try:
        n = int(count_7d)
    except ValueError:
        n = 0
    more = f" (+{n-1} more this week)" if n > 1 else ""
    return f"Last: {last}{more}"



def get_uncommitted_changes():
    """Check for uncommitted git changes."""
    diff = run_command("git diff --quiet 2>/dev/null; echo $?")
    diff_cached = run_command("git diff --cached --quiet 2>/dev/null; echo $?")

    if diff == "0" and diff_cached == "0":
        return "✅ No uncommitted changes"

    modified = len(run_command("git diff --name-only 2>/dev/null").splitlines())
    staged = len(run_command("git diff --cached --name-only 2>/dev/null").splitlines())
    return f"⚠️  {modified} modified, {staged} staged"


def get_cluster_health():
    """Get Kubernetes cluster health status."""
    if not run_command("command -v kubectl"):
        return "kubectl not available"

    nodes = run_command("kubectl get nodes --no-headers 2>/dev/null")
    if not nodes:
        return "Unable to query cluster"

    lines = nodes.splitlines()
    total = len(lines)
    not_ready = len([l for l in lines if " Ready " not in l])

    if not_ready == 0:
        return f"✅ All {total} nodes ready"
    return f"⚠️  {not_ready}/{total} nodes not ready"


def get_service_status():
    """Run health check and summarize."""
    health_script = HOME / "bin/health-check.sh"
    if not health_script.exists():
        return "health-check.sh not found"

    output = run_command(str(health_script), timeout=5)
    if not output:
        return "health-check failed to run"

    success = output.count("✅")
    fail = output.count("❌")
    warn = output.count("⚠️")

    if fail > 0 or warn > 0:
        return f"⚠️  {success} OK, {fail} failed, {warn} warnings"
    return f"✅ All critical services healthy ({success} checked)"


def check_active_warnings():
    """Check for active warning conditions."""
    warnings = []

    # Check Proxmox storage
    zp3_status = run_command(
        'ssh -o ConnectTimeout=2 psimmons@192.168.0.100 "pvesm status" 2>/dev/null | grep zp3'
    )
    if zp3_status:
        try:
            usage = int(zp3_status.split()[5].rstrip('%'))
            if usage > 85:
                warnings.append(f"⚠️  CRITICAL: Proxmox zp3 storage at {usage}% (>85% threshold)")
        except (IndexError, ValueError):
            pass

    # Check Homepage restarts
    if run_command("command -v kubectl"):
        restarts = run_command(
            'kubectl get pods -n default -l app.kubernetes.io/name=homepage --no-headers 2>/dev/null | awk \'{print $4}\' | head -1'
        )
        try:
            restart_count = int(restarts) if restarts else 0
            if restart_count > 3:
                warnings.append(f"⚠️  Homepage restarts: {restart_count} (>3 threshold)")
        except ValueError:
            pass

    return "; ".join(warnings) if warnings else "None detected"


def get_recent_failures():
    """Extract last 3 failures from failure-history.yaml."""
    failure_file = HOME / ".homelab/knowledge/failure-history.yaml"
    if not failure_file.exists():
        return "No failures logged"

    try:
        content = failure_file.read_text()
        # Simple extraction - look for timestamp and service lines
        lines = content.splitlines()
        failures = []
        current_failure = []

        for line in lines:
            if "timestamp:" in line.strip():
                if current_failure:
                    failures.append(" | ".join(current_failure))
                current_failure = [line.strip()]
            elif "service:" in line.strip() and current_failure:
                current_failure.append(line.strip())

        if current_failure:
            failures.append(" | ".join(current_failure))

        recent = failures[:3] if failures else []
        return "\n".join(f"- {f}" for f in recent) if recent else "No failures logged in last 30 days"
    except Exception:
        return "Unable to read failure history"


def get_intelligence_summary() -> str:
    """Read J-2 Intelligence Estimate and return top conditions summary."""
    from pathlib import Path
    import json
    try:
        state_path = Path.home() / "projects/generals/intelligence/state/intelligence-estimate.json"
        if not state_path.exists():
            return "J-2: No estimate available"
        with open(state_path) as f:
            data = json.load(f)
        conditions = data.get("conditions", [])
        active = [c for c in conditions if c.get("active", False)]
        if not active:
            return "J-2: All clear"
        # Sort by severity (CRITICAL first) then patrol_count
        sev_order = {"CRITICAL": 0, "WARNING": 1, "INFO": 2}
        active.sort(key=lambda c: (sev_order.get(c.get("severity", "INFO"), 9), -c.get("patrol_count", 0)))
        top3 = active[:3]
        parts = [f"{c['name']} ({c['severity']}, {c.get('patrol_count', 1)} patrol(s))" for c in top3]
        last_patrol = data.get("last_patrol", "unknown")
        return f"J-2 INTEL [{last_patrol[:16]}]: " + "; ".join(parts)
    except Exception:
        return "J-2: No estimate available"


def get_malus_status():
    """Get brief malus status from generals accountability system."""
    malus_script = HOME / "projects/generals/bin/malus-report.py"
    if not malus_script.exists():
        return "malus-report.py not found"
    output = run_command(
        f"cd {HOME / 'projects/generals'} && python3 bin/malus-report.py --brief",
        timeout=5
    )
    return output if output else "Unable to read malus status"


def main():
    """Generate session context."""
    if not MEMORY_TEMPLATE.exists():
        print("⚠️  MEMORY-TEMPLATE.md not found, skipping auto-generation")
        return 0

    # Read template
    template = MEMORY_TEMPLATE.read_text()

    # Generate replacements
    timestamp = datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
    session_number = datetime.now().strftime("%Y%m%d-%H%M%S")

    replacements = {
        "{{TIMESTAMP}}": timestamp,
        "{{SESSION_NUMBER}}": session_number,
        "{{RECENT_COMMITS}}": get_recent_commits(),
        "{{UNCOMMITTED_CHANGES}}": get_uncommitted_changes(),
        "{{CLUSTER_HEALTH}}": get_cluster_health(),
        "{{SERVICE_STATUS}}": get_service_status(),
        "{{ACTIVE_WARNINGS}}": check_active_warnings(),
        "{{MALUS_STATUS}}": get_malus_status(),
        "{{INTELLIGENCE_SUMMARY}}": get_intelligence_summary(),
    }

    # Apply replacements
    content = template
    for placeholder, value in replacements.items():
        content = content.replace(placeholder, value)

    # Write to MEMORY.md
    MEMORY_FILE.write_text(content)

    print(f"✅ Session context generated: {session_number}")
    print(f"📍 Updated: {MEMORY_FILE}")

    # Run memory janitor (dry run — reports only, never auto-archives)
    janitor = HOME / "bin/memory-janitor.py"
    if janitor.exists():
        janitor_output = run_command(f"python3 {janitor}", timeout=5)
        if janitor_output:
            print(janitor_output)

    return 0


if __name__ == "__main__":
    sys.exit(main())

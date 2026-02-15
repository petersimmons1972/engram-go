---
name: homelab-troubleshoot-hardware
description: Hardware-specific troubleshooting for mouse, GPU, and peripheral issues. Identifies device, applies quick fixes, tracks issue frequency, and suggests permanent solutions after recurring problems.
---

# Troubleshoot Hardware Issues

## Overview

Hardware-specific troubleshooting skill that handles mouse, GPU, USB, and other peripheral problems. Tracks issue frequency and escalates to permanent fixes after repeated occurrences.

**Purpose**: Fast hardware issue resolution with systematic tracking to identify when temporary fixes should become permanent solutions.

## When to Use

**Triggers**:
- "Mouse frozen" / "Mouse not working" / "Mouse stopped"
- "GPU freeze" / "Screen frozen" / "Display issues"
- "Keyboard not responding"
- "USB device not detected"
- Hardware-specific problems after idle/suspend

**When NOT to use**:
- Software configuration issues (use `superpowers:systematic-debugging`)
- Network connectivity issues (use network troubleshooting)
- Kubernetes/container issues (use K8s debugging)

## Hardware Device Identification

### Step 1: Identify the Device

**From user description, match to known device**:

| User Says | Device Category | Device Name |
|-----------|-----------------|-------------|
| "mouse", "cursor", "pointer" | mouse | Logitech MX Master 3 |
| "GPU", "display", "screen freeze", "monitors" | gpu | AMD Radeon RX 7900 XT |
| "keyboard", "keys", "typing" | keyboard | Das Keyboard 6 Pro |
| "USB", "dongle", "receiver" | usb | Various USB devices |

**If unclear, ask**:
```
Which hardware device is having issues?
- Mouse (Logitech MX Master 3)
- GPU/Display (AMD RX 7900 XT)
- Keyboard (Das Keyboard 6 Pro)
- Other USB device (describe)
```

## Device-Specific Troubleshooting

### Mouse (Logitech MX Master 3)

**Hardware Details**:
- Device: Logitech MX Master 3
- Connection: USB Unifying Receiver (046d:c52b)
- Known Issue: Driver enters bad state after suspend/idle
- Root Cause: HID driver regression + USB autosuspend

**Quick Fix** (apply first):
```bash
# Run the mouse fix script
~/bin/mouse.sh

# Or manually reload driver modules:
sudo rmmod hid_logitech_hidpp hid_logitech_dj && sleep 1 && sudo modprobe hid_logitech_dj && sudo modprobe hid_logitech_hidpp
```

**Permanent Fix** (if recurring >10 times):
```bash
# Disable USB autosuspend permanently (Pop_OS with systemd-boot)
sudo kernelstub -a "usbcore.autosuspend=-1"
sudo reboot

# Alternative: Disable system suspend entirely
sudo systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target
```

**Documentation**:
- Full troubleshooting log: `/home/psimmons/archive/mouse/MOUSE-TROUBLESHOOTING-LOG.md`
- Root cause analysis: `/home/psimmons/projects/mouse-configuration/docs/MOUSE-ROOT-CAUSE-ANALYSIS.md`

**Critical Note**: NEVER pair mouse via both Bluetooth AND USB Unifying Receiver on the same computer. Choose ONE connection method only.

---

### GPU (AMD Radeon RX 7900 XT)

**Hardware Details**:
- Device: AMD Radeon RX 7900 XT/XTX (RDNA3 Navi 31)
- Device ID: 83:00.0
- Monitors: 4x 27" displays (DisplayPort)
- Known Issue: GFXOFF feature crashes during DisplayPort power management

**Quick Fix** (temporary, lost after reboot):
```bash
# Disable GFXOFF temporarily
echo "0xffff7fff" | sudo tee /sys/module/amdgpu/parameters/ppfeaturemask

# Verify
cat /sys/module/amdgpu/parameters/ppfeaturemask
# Should show: 0xffff7fff
```

**Permanent Fix** (if recurring):
```bash
# Disable GFXOFF permanently via kernel parameter
sudo kernelstub --add-options "amdgpu.ppfeaturemask=0xffff7fff"

# Optional: Set idle timeout to 30 minutes
gsettings set org.gnome.desktop.session idle-delay 1800

# Reboot required
sudo reboot

# Verify after reboot
cat /proc/cmdline | grep ppfeaturemask
```

**If still freezing, try more aggressive masks**:
```bash
# Disable GFXOFF + STUTTER_MODE
sudo kernelstub --delete-options "amdgpu.ppfeaturemask=0xffff7fff"
sudo kernelstub --add-options "amdgpu.ppfeaturemask=0xfffd7fff"

# Or disable even more features
sudo kernelstub --add-options "amdgpu.ppfeaturemask=0xfffd3fff"
```

**Documentation**:
- Full troubleshooting log: `/home/psimmons/archive/gpu/AMD-GPU-FREEZE-TROUBLESHOOTING-LOG.md`
- Session investigation: `/home/psimmons/archive/gpu/SESSION-2026-01-03-GPU-FREEZE-INVESTIGATION.md`

---

### Keyboard (Das Keyboard 6 Pro)

**Hardware Details**:
- Device: Das Keyboard 6 Pro
- Connection: USB wired (24f0:20a1)
- Port: USB 3.1 hub

**Quick Fix**:
```bash
# Check if USB device is detected
lsusb | grep -i "das\|keyboard"

# Check input devices
cat /proc/bus/input/devices | grep -A5 -i keyboard

# Unplug and replug (different USB port if possible)
```

**If USB hub related**:
```bash
# Reset USB hub
echo "1" | sudo tee /sys/bus/usb/devices/3-1/authorized
echo "0" | sudo tee /sys/bus/usb/devices/3-1/authorized
echo "1" | sudo tee /sys/bus/usb/devices/3-1/authorized
```

---

### Generic USB Device Issues

**Diagnostic Commands**:
```bash
# List all USB devices
lsusb

# Check USB device tree
lsusb -t

# Check dmesg for USB errors
dmesg | grep -i usb | tail -20

# Check USB power management
cat /sys/bus/usb/devices/*/power/control
```

**Common Fixes**:
```bash
# Reset specific USB device (replace X-Y with device path)
echo "0" | sudo tee /sys/bus/usb/devices/X-Y/authorized
echo "1" | sudo tee /sys/bus/usb/devices/X-Y/authorized

# Reset entire USB controller
echo "1" | sudo tee /sys/bus/pci/devices/0000:00:14.0/reset
```

## Issue Frequency Tracking

### Step 2: Check and Update Frequency

**Check failure history**:
```bash
# Count occurrences of this device issue in last 30 days
grep -c "device: mouse" /home/psimmons/.homelab/knowledge/failure-history.yaml 2>/dev/null || echo "0"
```

**Log to failure-history.yaml** (append entry):
```yaml
- id: hardware-{device}-{YYYYMMDD-HHMM}
  timestamp: {current ISO8601}
  service: hardware-{device}
  tier: 3  # Hardware issues are usually Tier 3
  detected_by: user_report

  symptoms:
    - "{description of symptom}"

  diagnosis:
    root_cause: "{cause}"
    root_cause_category: hardware
    technical_details: "{technical explanation}"

  resolution:
    actual_mttr_seconds: {time to fix}
    commands_attempted:
      - command: "{command run}"
        outcome: success|failed
        reason: "{why}"

  learning:
    pattern_tags:
      - "hardware"
      - "{device}"
      - "{specific-issue}"

    automation_candidate: {true if >5 occurrences}
    automation_priority: {high if >10 occurrences}
```

### Step 3: Threshold-Based Recommendations

**Occurrence thresholds**:

| Count | Action |
|-------|--------|
| 1-5 | Apply quick fix, monitor |
| 6-10 | Suggest considering permanent fix |
| 11+ | **Strongly recommend** permanent fix |
| 20+ | Permanent fix is **urgent** |

## Output Format

**Standard output** (after running fix):
```
HARDWARE ISSUE: {Device Name}

Symptoms: {user-reported symptoms}
Quick fix: {command or script}

Running quick fix...
{command output}

Result: {success/failure}

---
FREQUENCY TRACKING
This is occurrence #{N} in the past 30 days.

{if N <= 5}
Status: Within normal range. Quick fix applied.
Next time: Run ~/bin/mouse.sh (or equivalent)

{if N > 5 AND N <= 10}
NOTE: Issue becoming frequent ({N} occurrences).
Consider implementing permanent fix:
{permanent fix command}
See: {documentation path}

{if N > 10}
RECOMMENDATION: Implement permanent fix NOW
This issue has occurred {N} times in 30 days.

Permanent solution:
{permanent fix command}

See: {documentation path}
```

**Example output**:
```
HARDWARE ISSUE: Mouse (Logitech MX Master 3)

Symptoms: Mouse frozen, cursor not moving
Quick fix: ~/bin/mouse.sh

Running quick fix...
Mouse driver modules reloaded. Your mouse should work now.

Result: SUCCESS

---
FREQUENCY TRACKING
This is occurrence #15 in the past 30 days.

RECOMMENDATION: Implement permanent fix NOW
This issue has occurred 15 times in 30 days.

Permanent solution:
sudo kernelstub -a "usbcore.autosuspend=-1"
sudo reboot

See: /home/psimmons/archive/mouse/MOUSE-TROUBLESHOOTING-LOG.md
```

## Diagnostic Information Gathering

**When quick fix fails, gather diagnostics**:

### Mouse Diagnostics
```bash
# USB detection
lsusb | grep -i logitech

# Input devices
cat /proc/bus/input/devices | grep -A10 "Logitech"

# Kernel modules
lsmod | grep hid_logitech

# USB power management
cat /sys/bus/usb/devices/*/product 2>/dev/null | grep -i receiver
find /sys/bus/usb/devices -name "product" -exec grep -l "Receiver" {} \; 2>/dev/null | xargs -I{} dirname {} | xargs -I{} cat {}/power/control

# Recent dmesg
dmesg | grep -i "logitech\|hidpp\|hid_dj" | tail -20
```

### GPU Diagnostics
```bash
# GPU info
lspci | grep -i vga

# AMD GPU driver status
cat /sys/class/drm/card0/device/power_dpm_state
cat /sys/class/drm/card0/device/power_dpm_force_performance_level
cat /sys/module/amdgpu/parameters/ppfeaturemask

# Recent GPU errors
dmesg | grep -i "amdgpu\|drm" | tail -30
journalctl -b -p err | grep -i "amdgpu\|drm\|gpu"

# COSMIC compositor status (if applicable)
systemctl --user status cosmic-comp 2>/dev/null || echo "Not using COSMIC"
```

## Integration with Other Skills

**This skill triggers**:
- `homelab:log-incident` - After resolving hardware incident (if significant)
- Pattern detection in failure-history.yaml

**Called from**:
- User direct invocation
- CLAUDE.md triage decision tree (Mouse? -> ~/bin/mouse.sh)

**Files accessed**:
- `/home/psimmons/bin/mouse.sh` (execute)
- `/home/psimmons/.homelab/knowledge/failure-history.yaml` (read/append)
- `/home/psimmons/archive/mouse/MOUSE-TROUBLESHOOTING-LOG.md` (reference)
- `/home/psimmons/archive/gpu/AMD-GPU-FREEZE-TROUBLESHOOTING-LOG.md` (reference)

## Critical Reminders

1. **Always try quick fix first** - Don't jump to permanent solutions
2. **Track frequency** - Pattern detection reveals systemic issues
3. **User must run sudo commands** - Claude cannot execute interactive sudo
4. **Reboot may be required** - For kernel parameter changes
5. **Document failures** - If quick fix doesn't work, gather diagnostics
6. **Check hardware connections** - Sometimes it's just a loose cable

## Known Anti-Patterns

| Tempting But Wrong | Why Wrong | Do Instead |
|-------------------|-----------|------------|
| Re-pair mouse | Driver state, not pairing | Run mouse.sh |
| Replace batteries | Mouse lights up = not batteries | Check driver state |
| Restart system immediately | Destroys diagnostic info | Check logs first |
| Install new drivers | Usually kernel module issue | Reload existing modules |
| Blame hardware failure | Usually software/driver | Systematic debugging |

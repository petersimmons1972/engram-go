---
name: Audio / PipeWire / Zoom conferencing fix
description: Root cause and fix for Zoom/Meet audio glitching on leviathan. Critical for interviews.
type: project
Category: homelab
originSessionId: ddb48371-b18e-453b-a52e-c938a3cc1033
---
# PipeWire Zoom Audio Fix

**Symptom:** VoiceEngine overruns every 2–10s, others hear chop/skip.

**Root cause:** quantum was **2048** (42.7ms) in `~/.config/pipewire/pipewire.conf.d/conferencing.conf`. Large quantum delivers mic audio in bursts → Zoom capture buffer (max 382ms) overflows → drops up to 2.5s.

**Fix (2026-04-10):** quantum = **512** (10.7ms). 256 crashes PipeWire (SIGFPE).

| File | Change |
|---|---|
| `pipewire.conf.d/conferencing.conf` | quantum 2048→512, min 1024→512, max 8192→1024 |
| `pipewire.conf.d/echo-cancel.conf` | node.latency 1024→512 |
| `pipewire-pulse.conf.d/10-zoom-fix.conf` | NEW — 512-fragment min for Zoom + Chrome |

**Audio routing:**
- **Speakers (Audioengine 2+):** mic = `echo-cancel-source` (virtual AEC). Turn OFF Zoom's own AEC.
- **Headphones:** mic = `Logitech C930e` direct. No echo to cancel.

**Pre-interview checklist:**
1. `systemctl --user status pipewire pipewire-pulse wireplumber` — all three `active (running)`
2. Zoom → Settings → Audio → Test Mic (no gaps in meter)
3. Confirm input is `echo-cancel-source` (speakers) or `C930e` (headphones)

**If it breaks again:**
```bash
systemctl --user restart pipewire wireplumber pipewire-pulse
journalctl --user -u pipewire-pulse -f | grep overrun
# Nuclear: rm conferencing.conf && restart  (default is 1024, NOT 2048)
```

---
name: usb-audio-conferencing
description: USB bus contention causes audio issues in Zoom/Teams — diagnosis, fix patterns, and Leviathan-specific port mapping
type: reference
Category: reference
---

# USB Audio & Conferencing — Leviathan (Thelio)

## Root Cause Pattern: "Bubbles" / Choppy Audio in Video Calls

**Symptom:** Remote party hears "bubbles," garbled audio, or cutting out. Jerky mouse on local machine despite powerful CPU.
**Root cause:** USB bus contention — too many devices on one controller, especially when webcam streams video+audio simultaneously.
**Not the cause:** CPU (Threadripper), network, app settings.

## Leviathan USB Controller Map

| Controller | PCI Address | Bus | Physical Location | Current Devices |
|------------|-------------|-----|-------------------|-----------------|
| AMD CPU-direct | `c1:00.4` | 1/2 | **Red** back panel ports | Monitors (daisy-chain), Audioengine 2+ |
| AMD CPU-direct | `06:00.4` | 3/4 | Separate back panel port | C930e webcam, Logitech receiver |
| Intel Thunderbolt | `52:00.0` | 5/6 | USB-C / Thunderbolt port | Empty (VIA hub) |
| AMD Chipset | `5c:00.0` (via `41:00.0`) | 7/8 | **Blue** back panel ports | Gigabyte DAC, Das Keyboard, Bluetooth, RGB, Thelio Io |

## Rules

1. **Webcam on its own controller** — never share a bus with 10+ other devices. Currently on Bus 3 (06:00.4) with only the Logitech receiver.
2. **Monitors = low bandwidth only** — chargers, keyboard, mouse, Yubikey. Never webcam or streaming audio. Monitor USB hubs daisy-chain through 4+ layers.
3. **Audioengine 2+ runs at USB 1.1 (12Mbps)** through monitor hub chain. Fine for audio output, but if playback issues occur, move to direct Thelio port.
4. **AirPods (Magma & Ash) use A2DP mode only for calls** — HFP mode drops both input and output to phone-call quality. Use C930e webcam mic instead.

## PipeWire Conferencing Config

Files in `~/.config/pipewire/pipewire.conf.d/`:

- **conferencing.conf** — quantum 2048 (default was 1024 pro-audio). Trades ~10ms latency for stable buffering.
- **echo-cancel.conf** — WebRTC echo canceller. Creates virtual "Echo Cancel Source/Sink" devices. Essential when using speakers instead of headphones.

## Ideal Call Setup

| Role | Device | Bus/Connection |
|------|--------|----------------|
| Video + Mic | C930e webcam | Bus 3 (dedicated controller) |
| Headphones | Magma & Ash (AirPod Max, A2DP) | Bluetooth |
| Fallback speakers | Audioengine 2+ | Bus 7 (monitor chain) |

## Bluetooth: AirPod Max on Linux

- Custom name: "Magma & Ash"
- A2DP vs HFP tradeoff: A2DP = stereo high quality, no mic. HFP = mono phone quality, mic works. Always use A2DP + webcam mic.
- AirPods refuse audio connection if actively connected to iPhone — disconnect phone first.
- If pairing goes stale (AVDTP "Connection refused 111"): `bluetoothctl remove <mac>`, put AirPods in pairing mode (hold noise control button 5s until white flash), re-pair.
- Das Keyboard udev rule (`/etc/udev/rules.d/50-das-keyboard.rules`) throws errors on BT connect — cosmetic, not functional.

## Diagnostic Commands

```bash
# Check USB topology — verify webcam is on separate bus
lsusb -t

# Check PipeWire quantum
pw-metadata -n settings | grep quantum

# Check mic volume
pactl get-source-volume alsa_input.usb-046d_Logitech_Webcam_C930e_40EF62FE-02.pro-input-0

# Check echo cancellation loaded
pw-cli list-objects Node | grep -i echo

# Check AirPods connection
bluetoothctl info 08:FF:44:4F:A9:48

# Check USB IRQ pressure (high count = overloaded)
cat /proc/interrupts | grep xhci
```

## Pre-Call Checklist

1. Connect Magma & Ash, verify they're on A2DP (check `pactl list short sinks | grep bluez`)
2. Disconnect AirPods from phone first if needed
3. In Zoom/Teams: mic = C930e, speaker = Magma & Ash
4. Zoom: disable "auto adjust mic volume", noise suppression = Low, HD video = OFF for important calls

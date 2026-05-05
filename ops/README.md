# Operations Files

This directory contains systemd unit files, health checks, and operational configuration for engram deployments.

## Files

### `instinct.timer` and `instinct.service`
Systemd timer and service units for running the instinct memory consolidation loop on a schedule.

### `engram-instinct-alert.service`
Alert service triggered when `instinct.timer` fails. Logs the timer status and the last 20 lines of instinct.service to journald for diagnostic purposes.

**Setup**:
```bash
sudo cp ops/engram-instinct-alert.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable engram-instinct-alert.service
```

The alert is automatically triggered when `instinct.timer` enters a failed state.

### `engram-instinct.logrotate`
Logrotate configuration for instinct run logs. Rotates `/var/lib/instinct/run.log` weekly, keeping 4 weeks of compressed history.

**Setup**:
```bash
sudo cp ops/engram-instinct.logrotate /etc/logrotate.d/engram-instinct
```

Rotation runs automatically via the system logrotate cron job (typically daily at 6:30 AM).

### `healthcheck.sh`
Bash script that checks whether the instinct consolidator has run recently. Used as a readiness/liveness probe for monitoring.

Exit codes:
- `0` = healthy (instinct ran within the window)
- `1` = stale or missing (no recent run)

Usage:
```bash
./ops/healthcheck.sh
echo $?
```

Environment variables:
- `INSTINCT_MAX_GAP` = maximum allowed gap between runs in seconds (default: 7200 = 2 hours)

## Monitoring

### Journal inspection
View instinct service logs:
```bash
journalctl -u instinct.service -n 50 -f
```

View timer activity:
```bash
journalctl -u instinct.timer -n 50 -f
```

View alerts when the timer fails:
```bash
journalctl -u engram-instinct-alert.service -n 50 -f
```

### Timer status
Check whether the timer is active and when it last ran:
```bash
systemctl status instinct.timer
systemctl list-timers instinct.timer
```

## Troubleshooting

### Timer not running
```bash
systemctl status instinct.timer
systemctl enable instinct.timer
systemctl start instinct.timer
```

### Service failing repeatedly
Check the last few lines of the service log:
```bash
journalctl -u instinct.service -n 20 -e
```

### Log files growing too large
Verify logrotate is configured:
```bash
logrotate -d /etc/logrotate.d/engram-instinct
```

Force rotation for testing:
```bash
sudo logrotate -f /etc/logrotate.d/engram-instinct
```

## Issues

- #556: healthcheck.sh exit code not monitored; no log rotation
- #557: SIGHUP runtime feature-flag mechanism (deferred)

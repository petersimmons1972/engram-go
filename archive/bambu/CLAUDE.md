---
project: bambu
purpose: MCP server exposing Bambu Lab P1S printer control to Claude Code via Docker.
stack: [docker, nodejs]
status: archived
entrypoints:
  - docker-compose.yml
related: [3dprint]
notes: Lives in ~/archive/ — superseded or parked.
---

# Bambu Lab P1S MCP Server

## Printer Details
- **Model:** Bambu Lab P1S
- **Serial:** 01P09C4C1300519
- **IP:** 192.168.0.51 (DHCP reservation)
- **AMS:** AMS 2 Pro attached

## Service Management
```bash
# Start
cd ~/projects/bambu && docker compose up -d

# Stop
cd ~/projects/bambu && docker compose down

# Rebuild (after repo updates)
cd ~/projects/bambu && docker compose up --build -d

# Logs
docker logs bambu-mcp-server

# Test endpoint
curl -s -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
```

## MCP Registration
Registered as `bambu-printer` in user scope via:
```bash
claude mcp add --transport http -s user bambu-printer http://localhost:3000/mcp
```

## Architecture
- Docker container (`bambu-mcp-server`) runs the MCP server
- `streamable-http` transport on `http://localhost:3000/mcp`
- `network_mode: host` for MQTT (8883) and FTP (990) access to printer on LAN
- MQTT maintains persistent connection to printer for real-time status
- Temp files stored in `./temp` volume mount

## Available MCP Tools
The server provides tools for:
- Printer status monitoring (temperatures, print progress, state)
- File management (upload, list, delete files on printer)
- Print control (start, pause, resume, cancel prints)
- Temperature control (set hotend and bed temperatures)

## Troubleshooting
- If tools show "printer offline", verify printer IP with `ping 192.168.0.51`
- If container won't start, check port 3000 isn't in use: `ss -tlnp | grep 3000`
- For MQTT connection issues, verify printer is on and connected to WiFi

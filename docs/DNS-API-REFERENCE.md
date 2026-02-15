# DNS API Reference

Complete curl examples and workflows for DNS management across Unifi Controller, Cloudflare, and Pi-hole APIs.

---

## Table of Contents

1. [Unifi Controller API](#unifi-controller-api)
2. [Cloudflare DNS API](#cloudflare-dns-api)
3. [Pi-hole API](#pi-hole-api)
4. [Decision Tree](#decision-tree)
5. [Common Patterns](#common-patterns)
6. [Troubleshooting](#troubleshooting)

---

## Unifi Controller API

**Purpose**: Manage internal network DNS records (*.petersimmons.com)

**Endpoint**: `https://192.168.0.1/proxy/network/v2/api/site/default/static-dns`

**Authentication**: X-API-KEY header

**Credentials Location**: `~/.claude/.unifi-credentials`

**API Key**: `${UNIFI_API_KEY}`

### Create Static DNS Record

```bash
#!/bin/bash
# Add a static DNS record to Unifi Controller

API_KEY="${UNIFI_API_KEY}"
UNIFI_HOST="192.168.0.1"
RECORD_NAME="subdomain.petersimmons.com"
RECORD_TYPE="CNAME"  # or "A", "AAAA", "MX", "TXT"
RECORD_VALUE="target.petersimmons.com"

curl -k -X POST "https://${UNIFI_HOST}/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "'"${RECORD_NAME}"'",
    "record_type": "'"${RECORD_TYPE}"'",
    "value": "'"${RECORD_VALUE}"'"
  }'
```

### Create A Record (IP Address)

```bash
#!/bin/bash
# Create an A record pointing to an IP address

API_KEY="${UNIFI_API_KEY}"
UNIFI_HOST="192.168.0.1"
RECORD_NAME="server.petersimmons.com"
IP_ADDRESS="192.168.0.200"

curl -k -X POST "https://${UNIFI_HOST}/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "'"${RECORD_NAME}"'",
    "record_type": "A",
    "value": "'"${IP_ADDRESS}"'"
  }'
```

### Create CNAME Record (Alias)

```bash
#!/bin/bash
# Create a CNAME record for service aliases

API_KEY="${UNIFI_API_KEY}"
UNIFI_HOST="192.168.0.1"
ALIAS_NAME="api.petersimmons.com"
TARGET_NAME="backend.petersimmons.com"

curl -k -X POST "https://${UNIFI_HOST}/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "'"${ALIAS_NAME}"'",
    "record_type": "CNAME",
    "value": "'"${TARGET_NAME}"'"
  }'
```

### List All Static DNS Records

```bash
#!/bin/bash
# Retrieve all static DNS records from Unifi

API_KEY="${UNIFI_API_KEY}"
UNIFI_HOST="192.168.0.1"

curl -k -X GET "https://${UNIFI_HOST}/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${API_KEY}" \
  -H "Content-Type: application/json" | jq '.'
```

### Update Existing DNS Record

```bash
#!/bin/bash
# Update an existing static DNS record (requires record ID)

API_KEY="${UNIFI_API_KEY}"
UNIFI_HOST="192.168.0.1"
RECORD_ID="5f8c9d3e2b1a4c5d9e8f7g6h"
NEW_VALUE="192.168.0.210"

curl -k -X PATCH "https://${UNIFI_HOST}/proxy/network/v2/api/site/default/static-dns/${RECORD_ID}" \
  -H "X-API-KEY: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "value": "'"${NEW_VALUE}"'"
  }'
```

### Delete DNS Record

```bash
#!/bin/bash
# Delete a static DNS record by ID

API_KEY="${UNIFI_API_KEY}"
UNIFI_HOST="192.168.0.1"
RECORD_ID="5f8c9d3e2b1a4c5d9e8f7g6h"

curl -k -X DELETE "https://${UNIFI_HOST}/proxy/network/v2/api/site/default/static-dns/${RECORD_ID}" \
  -H "X-API-KEY: ${API_KEY}" \
  -H "Content-Type: application/json"
```

### Response Format (GET)

```json
[
  {
    "_id": "5f8c9d3e2b1a4c5d9e8f7g6h",
    "enabled": true,
    "key": "subdomain.petersimmons.com",
    "record_type": "CNAME",
    "value": "target.petersimmons.com",
    "site_id": "default"
  },
  {
    "_id": "6g9d0e4f3c2b5d6e0f9g8h7i",
    "enabled": true,
    "key": "server.petersimmons.com",
    "record_type": "A",
    "value": "192.168.0.200"
  }
]
```

---

## Cloudflare DNS API

**Purpose**: Manage public DNS records (external access, propagation to internet)

**Endpoint**: `https://api.cloudflare.com/client/v4/zones/{zone_id}/dns_records`

**Authentication**: Bearer token in Authorization header

**Credentials Location**: `~/projects/kubernetes/cert-manager/.env`

**API Token**: `${CF_TOKEN}`

### Managed Zones

| Domain | Zone ID |
|--------|---------|
| petersimmons.com | 460653bc26ff94fdc0910a13defa4afb |
| clearwatchresearch.com | 684f1eed00746f0fc7f2b718b21d983c |
| clearwatchintelligence.com | 96a43875170aff855a6bf9a033d234f1 |
| clearwatch.io | 018e8292ffd3e2e501771617bf82ba71 |

### Create DNS Record

```bash
#!/bin/bash
# Add a DNS record to Cloudflare

CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"  # petersimmons.com
RECORD_NAME="api"  # Creates api.petersimmons.com
RECORD_TYPE="CNAME"
RECORD_VALUE="target.petersimmons.com"
TTL=1  # Auto TTL
PROXIED=false  # DNS only (true = proxied through Cloudflare)

curl -X POST "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "'"${RECORD_TYPE}"'",
    "name": "'"${RECORD_NAME}"'",
    "content": "'"${RECORD_VALUE}"'",
    "ttl": '"${TTL}"',
    "proxied": '"${PROXIED}"'
  }'
```

### Create A Record (IP Address)

```bash
#!/bin/bash
# Add an A record to Cloudflare

CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"
RECORD_NAME="server"
IP_ADDRESS="203.0.113.42"  # Public IP
TTL=3600
PROXIED=true  # Route through Cloudflare for DDoS protection

curl -X POST "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "A",
    "name": "'"${RECORD_NAME}"'",
    "content": "'"${IP_ADDRESS}"'",
    "ttl": '"${TTL}"',
    "proxied": '"${PROXIED}"'
  }'
```

### Create MX Record (Mail)

```bash
#!/bin/bash
# Add an MX record for mail routing

CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"
MAIL_SERVER="mail.petersimmons.com"
PRIORITY=10  # Lower = higher priority

curl -X POST "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "MX",
    "name": "@",
    "content": "'"${MAIL_SERVER}"'",
    "priority": '"${PRIORITY}"',
    "ttl": 3600
  }'
```

### List DNS Records for Zone

```bash
#!/bin/bash
# Get all DNS records in a zone

CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"

curl -X GET "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" | jq '.'
```

### Search DNS Record by Name

```bash
#!/bin/bash
# Search for a specific DNS record

CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"
RECORD_NAME="api.petersimmons.com"

curl -X GET "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records?name=${RECORD_NAME}" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" | jq '.'
```

### Update DNS Record

```bash
#!/bin/bash
# Update an existing DNS record

CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"
RECORD_ID="372e67954025e0ba6aaa6d586b9e0b59"  # From GET request
NEW_VALUE="203.0.113.50"

curl -X PUT "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records/${RECORD_ID}" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "A",
    "name": "api.petersimmons.com",
    "content": "'"${NEW_VALUE}"'",
    "ttl": 1,
    "proxied": false
  }'
```

### Delete DNS Record

```bash
#!/bin/bash
# Delete a DNS record

CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"
RECORD_ID="372e67954025e0ba6aaa6d586b9e0b59"

curl -X DELETE "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records/${RECORD_ID}" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json"
```

### Response Format (GET)

```json
{
  "success": true,
  "errors": [],
  "messages": [],
  "result": [
    {
      "id": "372e67954025e0ba6aaa6d586b9e0b59",
      "type": "CNAME",
      "name": "api.petersimmons.com",
      "content": "target.petersimmons.com",
      "proxiable": true,
      "proxied": false,
      "ttl": 1,
      "locked": false,
      "zone_id": "460653bc26ff94fdc0910a13defa4afb",
      "zone_name": "petersimmons.com",
      "created_on": "2023-01-01T12:00:00Z",
      "modified_on": "2023-01-02T13:30:00Z"
    }
  ],
  "result_info": {
    "page": 1,
    "per_page": 20,
    "total_pages": 1,
    "count": 1,
    "total_count": 1
  }
}
```

---

## Pi-hole API

**Purpose**: Monitoring and adblock management (DNS records NOT supported by API v6)

**Endpoints**:
- Primary: `https://192.168.0.231/admin/api.php`
- Secondary: `https://192.168.0.232/admin/api.php`

**Authentication**: App Password in query parameter

**Credentials Location**: `~/.claude/pihole-api-credentials.md`

**Primary App Password**: `gLS37GXVK1pG04BW5/GLvHUb+noHHHKI4ulI8zAIpTQ=`

### Get Summary Statistics

```bash
#!/bin/bash
# Retrieve overall Pi-hole statistics

PIHOLE_HOST="192.168.0.231"
APP_PASSWORD="gLS37GXVK1pG04BW5/GLvHUb+noHHHKI4ulI8zAIpTQ="

curl -k "https://${PIHOLE_HOST}/admin/api.php?summaryRaw&auth=${APP_PASSWORD}"
```

### Get DNS Query Logs

```bash
#!/bin/bash
# Retrieve DNS query logs

PIHOLE_HOST="192.168.0.231"
APP_PASSWORD="gLS37GXVK1pG04BW5/GLvHUb+noHHHKI4ulI8zAIpTQ="
LIMIT=100

curl -k "https://${PIHOLE_HOST}/admin/api.php?getQuerySources&limit=${LIMIT}&auth=${APP_PASSWORD}" | jq '.'
```

### Get Top Clients

```bash
#!/bin/bash
# Get clients making most DNS queries

PIHOLE_HOST="192.168.0.231"
APP_PASSWORD="gLS37GXVK1pG04BW5/GLvHUb+noHHHKI4ulI8zAIpTQ="

curl -k "https://${PIHOLE_HOST}/admin/api.php?topClients&auth=${APP_PASSWORD}" | jq '.'
```

### Get Top Blocked Domains

```bash
#!/bin/bash
# Get domains most frequently blocked

PIHOLE_HOST="192.168.0.231"
APP_PASSWORD="gLS37GXVK1pG04BW5/GLvHUb+noHHHKI4ulI8zAIpTQ="

curl -k "https://${PIHOLE_HOST}/admin/api.php?topBlocked&auth=${APP_PASSWORD}" | jq '.'
```

### Get Adlists

```bash
#!/bin/bash
# Retrieve configured adblock lists

PIHOLE_HOST="192.168.0.231"
APP_PASSWORD="gLS37GXVK1pG04BW5/GLvHUb+noHHHKI4ulI8zAIpTQ="

curl -k "https://${PIHOLE_HOST}/admin/api.php?adlist&auth=${APP_PASSWORD}" | jq '.'
```

### Enable/Disable Gravity

```bash
#!/bin/bash
# Enable gravity (blocking enabled)

PIHOLE_HOST="192.168.0.231"
APP_PASSWORD="gLS37GXVK1pG04BW5/GLvHUb+noHHHKI4ulI8zAIpTQ="

# Enable
curl -k -X POST "https://${PIHOLE_HOST}/admin/api.php" \
  -d "auth=${APP_PASSWORD}&enable" \
  -H "Content-Type: application/x-www-form-urlencoded"

# Disable
curl -k -X POST "https://${PIHOLE_HOST}/admin/api.php" \
  -d "auth=${APP_PASSWORD}&disable" \
  -H "Content-Type: application/x-www-form-urlencoded"
```

### Important Limitation

**Pi-hole API v6 does NOT support managing local DNS records (A, CNAME, MX, TXT, etc.)**

To add local DNS records in Pi-hole, you must:
1. SSH into Pi-hole server
2. Edit dnsmasq configuration: `/etc/dnsmasq.d/`
3. Restart dnsmasq service

**Example Manual Method:**

```bash
ssh root@192.168.0.231

# Add local DNS record
echo "address=/subdomain.petersimmons.com/192.168.0.200" \
  > /etc/dnsmasq.d/05-petersimmons.conf

# Restart dnsmasq
systemctl restart dnsmasq
```

---

## Decision Tree

### When to Use Which API?

```
Need to add/update DNS record?
│
├─ Internal-only access (*.petersimmons.com from inside network)?
│  └─ Use: UNIFI Controller API
│     ├─ Fastest: Immediate in-network resolution
│     ├─ No propagation delay
│     └─ Best for: Internal services, lab hostnames
│
├─ Public-facing domain (external access needed)?
│  └─ Use: CLOUDFLARE API
│     ├─ Routes through Cloudflare nameservers
│     ├─ Handles external + internal access
│     ├─ DDoS protection available (proxied=true)
│     └─ Best for: External-facing services, production domains
│
├─ Both internal AND external access needed?
│  └─ Use: BOTH Unifi + Cloudflare
│     ├─ Add Unifi record for instant internal access
│     ├─ Add Cloudflare record for external/backup access
│     ├─ Internal DNS resolved locally (fast)
│     ├─ External DNS falls back to Cloudflare
│     └─ Example: Traefik ingress, public API endpoints
│
└─ Monitoring/blocking management only?
   └─ Use: PI-HOLE API (read-only operations)
      ├─ Query logs, statistics
      ├─ Adlist management
      ├─ NOT for adding DNS records
      └─ For local DNS: Use SSH + dnsmasq edit
```

### Decision Tree Flowchart

```
┌─────────────────────────────┐
│ Adding DNS Record?          │
└─────────────────────────────┘
              │
         ┌────┴────┐
         │          │
   Internal?   External?
         │          │
        YES        YES?
         │          │
     ┌───┴──┐   ┌───┴──┐
     │      │   │      │
    Only  Both YES  Only
    Unifi Both/  Cloud Cloud
     Only      Cloud  Only
     │        │      │
  Unifi    Both   Cloud
  Only    APIs    Only
     │        │      │
   Fast   Most      Safe
  Local   Flexible Backup
```

---

## Common Patterns

### Pattern 1: Internal Service with External Fallback

**Scenario**: Kubernetes service that internal clients should reach via fast Unifi DNS, but external clients need Cloudflare fallback.

**Unifi (Internal - Fast)**
```bash
curl -k -X POST "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${UNIFI_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "dashboard.petersimmons.com",
    "record_type": "A",
    "value": "192.168.0.135"
  }'
```

**Cloudflare (External - Backup)**
```bash
CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"

curl -X POST "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "A",
    "name": "dashboard",
    "content": "203.0.113.42",
    "ttl": 3600,
    "proxied": true
  }'
```

**Result**:
- Internal clients (192.168.0.x) → fast Unifi → 192.168.0.135 (immediate)
- External clients → Cloudflare → 203.0.113.42 (internet accessible)

### Pattern 2: Service Alias (CNAME Chain)

**Scenario**: Multiple domain names pointing to same service.

```bash
# Main service
curl -k -X POST "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${UNIFI_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "api.petersimmons.com",
    "record_type": "A",
    "value": "192.168.0.136"
  }'

# Alias 1
curl -k -X POST "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${UNIFI_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "backend.petersimmons.com",
    "record_type": "CNAME",
    "value": "api.petersimmons.com"
  }'

# Alias 2
curl -k -X POST "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${UNIFI_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "service.petersimmons.com",
    "record_type": "CNAME",
    "value": "api.petersimmons.com"
  }'
```

**Result**: `backend.petersimmons.com` and `service.petersimmons.com` both resolve to `api.petersimmons.com` → 192.168.0.136

### Pattern 3: Multi-Zone Management (Cloudflare)

**Scenario**: Managing multiple public domains with centralized DNS.

```bash
#!/bin/bash
# Helper function to add record to any Cloudflare zone

add_cloudflare_record() {
  local zone_id=$1
  local record_name=$2
  local record_type=$3
  local record_value=$4

  CF_TOKEN="${CF_TOKEN}"

  curl -X POST "https://api.cloudflare.com/client/v4/zones/${zone_id}/dns_records" \
    -H "Authorization: Bearer ${CF_TOKEN}" \
    -H "Content-Type: application/json" \
    --data '{
      "type": "'"${record_type}"'",
      "name": "'"${record_name}"'",
      "content": "'"${record_value}"'",
      "ttl": 1,
      "proxied": false
    }'
}

# Add same hostname to multiple zones
add_cloudflare_record "460653bc26ff94fdc0910a13defa4afb" "api" "CNAME" "backend.petersimmons.com"
add_cloudflare_record "684f1eed00746f0fc7f2b718b21d983c" "api" "CNAME" "backend.petersimmons.com"
add_cloudflare_record "96a43875170aff855a6bf9a033d234f1" "api" "CNAME" "backend.petersimmons.com"
```

### Pattern 4: Blue-Green Deployment (Update with No Downtime)

**Scenario**: Switch traffic between two service instances.

```bash
#!/bin/bash
# Get current record ID
CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"
RECORD_NAME="service.petersimmons.com"

# Fetch current record
RECORD=$(curl -s -X GET "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records?name=${RECORD_NAME}" \
  -H "Authorization: Bearer ${CF_TOKEN}" | jq '.result[0]')

RECORD_ID=$(echo "$RECORD" | jq -r '.id')
CURRENT_IP=$(echo "$RECORD" | jq -r '.content')

echo "Current IP: ${CURRENT_IP}"

# Update to new IP (blue-green switch)
NEW_IP="203.0.113.99"

curl -X PUT "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records/${RECORD_ID}" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "A",
    "name": "'"${RECORD_NAME}"'",
    "content": "'"${NEW_IP}"'",
    "ttl": 1,
    "proxied": false
  }'

echo "Updated to IP: ${NEW_IP}"
```

---

## Troubleshooting

### Record Not Resolving (Unifi)

**Symptom**: Record added to Unifi, but DNS queries fail.

**Debug Steps**:

```bash
# 1. Verify record exists in Unifi
curl -k -X GET "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${UNIFI_API_KEY}" | jq '.[] | select(.key=="subdomain.petersimmons.com")'

# 2. Check if record is enabled
# (enabled: true must be present in response)

# 3. Query DNS directly from Pi-hole
ssh root@192.168.0.231
nslookup subdomain.petersimmons.com 192.168.0.1

# 4. If nslookup fails, check Pi-hole dnsmasq
ssh root@192.168.0.231
cat /etc/dnsmasq.d/* | grep subdomain
systemctl status dnsmasq
systemctl restart dnsmasq

# 5. Query Pi-hole itself
nslookup subdomain.petersimmons.com 192.168.0.231
```

### Record Not Resolving (Cloudflare)

**Symptom**: Cloudflare record created, but external DNS lookup fails.

**Debug Steps**:

```bash
# 1. Verify record exists
CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"

curl -s -X GET "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records?name=api.petersimmons.com" \
  -H "Authorization: Bearer ${CF_TOKEN}" | jq '.result'

# 2. Check propagation status
# Cloudflare updates within minutes, but DNS propagation can take 24-48 hours
# Use external tool: https://whatsmydns.net/

# 3. Verify TTL is appropriate
# TTL 1 = Cloudflare auto (recommended for testing)
# TTL 3600 = 1 hour caching
# Higher TTL = slower updates but lower API load

# 4. If proxied=true, verify orange cloud is active
# (proxied=true = proxied through Cloudflare)
# (proxied=false = DNS only)

# 5. Test external resolution
dig @1.1.1.1 api.petersimmons.com +short
```

### Authentication Failures

**Symptom**: API returns 401 Unauthorized or invalid auth.

**Solutions**:

```bash
# Unifi: Verify API key
cat ~/.claude/.unifi-credentials
# Should contain: API_KEY=${UNIFI_API_KEY}

# Cloudflare: Verify token format
cat ~/projects/kubernetes/cert-manager/.env | grep CLOUDFLARE_API_TOKEN
# Token should be: ${CF_TOKEN}

# Pi-hole: Verify app password
cat ~/.claude/pihole-api-credentials.md
# App password should be: gLS37GXVK1pG04BW5/GLvHUb+noHHHKI4ulI8zAIpTQ=

# Test with curl -v for verbose output
curl -v -k -X GET "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${UNIFI_API_KEY}" 2>&1 | head -20
```

### SSL Certificate Errors

**Symptom**: `curl: (60) SSL certificate problem: self signed certificate`

**Solution**:

```bash
# Use -k flag to skip SSL verification (self-signed certs)
curl -k -X GET "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${UNIFI_API_KEY}"

# For production: Install CA certificate
# This is safe for internal IPs (192.168.x.x)
```

### Rate Limiting (Cloudflare)

**Symptom**: API returns 429 Too Many Requests.

**Solutions**:

```bash
# Cloudflare free plan: 1200 requests/min
# Pro plan: Higher limits

# Add delays between requests
for zone_id in "460653bc26ff94fdc0910a13defa4afb" "684f1eed00746f0fc7f2b718b21d983c"; do
  curl -X GET "https://api.cloudflare.com/client/v4/zones/${zone_id}/dns_records" \
    -H "Authorization: Bearer ${CF_TOKEN}"
  sleep 1  # 1 second delay between requests
done
```

---

## Quick Reference Commands

```bash
# Add Unifi record (fastest for internal)
curl -k -X POST "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${UNIFI_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"enabled":true,"key":"name.petersimmons.com","record_type":"A","value":"192.168.0.X"}'

# Add Cloudflare record (public/external access)
CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"
curl -X POST "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{"type":"A","name":"name","content":"203.0.113.X","ttl":1,"proxied":false}'

# Query Pi-hole stats (monitoring only)
curl -k "https://192.168.0.231/admin/api.php?summaryRaw&auth=gLS37GXVK1pG04BW5/GLvHUb+noHHHKI4ulI8zAIpTQ="
```


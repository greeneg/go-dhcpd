# API Documentation

The go-dhcpd daemon exposes a REST API on port 18467 (configurable) for monitoring and management.

## Base URL

```
http://localhost:18467
```

## Endpoints

### Health & Status

#### GET /health

Check if the server is healthy and responding.

**Response:**
```json
{
  "status": "healthy",
  "time": "2026-08-22T11:42:00Z"
}
```

**Status Codes:**
- `200 OK` - Server is healthy

---

#### GET /version

Get server version information.

**Response:**
```json
{
  "name": "go-dhcpd",
  "version": "1.0.0",
  "author": "greeneg"
}
```

---

### Metrics & Statistics

#### GET /metrics

Get overall server metrics including uptime and packet statistics.

**Response:**
```json
{
  "uptime_seconds": 3600.5,
  "uptime_string": "1h0m0.5s",
  "packets_received": 1234,
  "packets_sent": 567,
  "errors": 2,
  "active_leases": 45,
  "static_leases": 10,
  "expired_leases": 123,
  "denied_addresses": 3
}
```

---

#### GET /metrics/dhcp

Get detailed DHCP protocol statistics.

**Response:**
```json
{
  "start_time": "2026-08-22T10:42:00Z",
  "uptime_seconds": 3600.5,
  "discover_count": 234,
  "offer_count": 234,
  "request_count": 200,
  "ack_count": 195,
  "nak_count": 5,
  "release_count": 12,
  "inform_count": 8,
  "decline_count": 1,
  "packets_received": 1234,
  "packets_sent": 567,
  "errors": 2
}
```

**Fields:**
- `discover_count` - Number of DHCPDISCOVER messages received
- `offer_count` - Number of DHCPOFFER messages sent
- `request_count` - Number of DHCPREQUEST messages received
- `ack_count` - Number of DHCPACK messages sent
- `nak_count` - Number of DHCPNAK messages sent
- `release_count` - Number of DHCPRELEASE messages received
- `inform_count` - Number of DHCPINFORM messages received
- `decline_count` - Number of DHCPDECLINE messages received

---

#### GET /metrics/database

Get database statistics.

**Response:**
```json
{
  "active_leases": 45,
  "static_leases": 10,
  "expired_leases": 123,
  "denied_addresses": 3
}
```

---

### Configuration

#### GET /config

Get the current server configuration.

**Response:**
```json
{
  "global": {
    "lease_time": 86400,
    "netbios_node_type": 8,
    "ping_timeout": 1000,
    "ping_retries": 2,
    "database_path": "/var/lib/go-dhcpd/dhcpd.db",
    "listen_address": "0.0.0.0",
    "api_port": 18467
  },
  "subnets": [...],
  "static": [...]
}
```

**Note:** Configuration is read-only via the API. To modify configuration, edit the config file and restart the daemon.

---

### Leases

#### GET /leases

Get all leases (active and expired).

**Response:**
```json
{
  "count": 168,
  "leases": [
    {
      "ID": 1,
      "MACAddress": "aa:bb:cc:dd:ee:ff",
      "IPAddress": "192.168.1.10",
      "Hostname": "server1",
      "LeaseStart": "2026-08-22T10:00:00Z",
      "LeaseEnd": "2026-08-23T10:00:00Z",
      "State": "active",
      "IsStatic": true
    },
    ...
  ]
}
```

---

#### GET /leases/active

Get only active leases (non-expired).

**Response:**
```json
{
  "count": 45,
  "leases": [...]
}
```

---

#### GET /leases/:mac

Get lease information for a specific MAC address.

**Parameters:**
- `mac` (path) - MAC address in format `aa:bb:cc:dd:ee:ff`

**Example:**
```bash
curl http://localhost:18467/leases/aa:bb:cc:dd:ee:ff
```

**Response:**
```json
{
  "ID": 1,
  "MACAddress": "aa:bb:cc:dd:ee:ff",
  "IPAddress": "192.168.1.10",
  "Hostname": "server1",
  "LeaseStart": "2026-08-22T10:00:00Z",
  "LeaseEnd": "2026-08-23T10:00:00Z",
  "State": "active",
  "IsStatic": true
}
```

**Status Codes:**
- `200 OK` - Lease found
- `404 Not Found` - No lease for this MAC address

---

### Denied Addresses

#### GET /deny

Get all denied (martian) addresses detected via ping check.

**Response:**
```json
{
  "count": 3,
  "addresses": [
    {
      "ID": 1,
      "IPAddress": "192.168.1.50",
      "MACAddress": "11:22:33:44:55:66",
      "Reason": "Responded to ping check, possible squatter. Detected MAC: 11:22:33:44:55:66",
      "DetectedAt": "2026-08-22T09:30:00Z"
    },
    ...
  ]
}
```

**Fields:**
- `IPAddress` - The denied IP address
- `MACAddress` - MAC address of the device using the IP (if detected)
- `Reason` - Why this address was denied
- `DetectedAt` - When this address was detected as in use

---

## Example Usage

### Monitoring Script

```bash
#!/bin/bash

# Check health
if curl -sf http://localhost:18467/health > /dev/null; then
    echo "Server is healthy"
else
    echo "Server is down!"
    exit 1
fi

# Get active lease count
ACTIVE=$(curl -s http://localhost:18467/metrics | jq -r '.active_leases')
echo "Active leases: $ACTIVE"

# Get error count
ERRORS=$(curl -s http://localhost:18467/metrics/dhcp | jq -r '.errors')
echo "Errors: $ERRORS"
```

### Get All Active Leases

```bash
curl -s http://localhost:18467/leases/active | jq '.leases[] | {ip: .IPAddress, mac: .MACAddress, hostname: .Hostname}'
```

### Get All Non-static Leases

```bash
curl -s http://localhost:18467/leases | jq '.leases[] | select(.IsStatic==false)'
```

### Find Lease for Specific MAC

```bash
MAC="aa:bb:cc:dd:ee:ff"
curl -s "http://localhost:18467/leases/$MAC" | jq
```

### Check for Denied Addresses

```bash
curl -s http://localhost:18467/deny | jq '.addresses[] | {ip: .IPAddress, reason: .Reason}'
```

### Monitor Real-time Statistics

```bash
watch -n 5 'curl -s http://localhost:18467/metrics/dhcp | jq'
```

---

## Response Codes

All endpoints return standard HTTP status codes:

- `200 OK` - Request successful
- `404 Not Found` - Resource not found
- `500 Internal Server Error` - Server error

## Content Type

All responses are in JSON format with `Content-Type: application/json`.

## Authentication

Currently, the API does not require authentication. It is recommended to:
- Bind the API to localhost only (or use firewall rules)
- Use a reverse proxy with authentication if exposing externally
- Consider implementing authentication in future versions

## Rate Limiting

No rate limiting is currently implemented. The API is designed for administrative use and monitoring.

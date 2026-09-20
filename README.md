# go-dhcpd

A modern DHCP and BootP daemon written in Golang with comprehensive RFC compliance, web API, and robust address management.

## Features

- **IPv4 DHCP Server**
  - Dynamic address allocation for unregistered systems
  - Static address assignment for registered systems
  - Full RFC compliance for DHCP and BootP protocols
  
- **Advanced Configuration**
  - JSON5 configuration files (with comments!)
  - Subnet definitions with flexible IP range specifications
  - Per-subnet DHCP options (DNS, routers, NTP, NetBIOS, etc.)
  - Enable/disable BootP per subnet
  
- **Address Management**
  - SQLite3 database for lease tracking
  - Automatic ping check before allocation
  - Martian address detection and blocking
  - MAC address tracking for address squatters
  
- **Web API**
  - REST API on port 18467 (configurable)
  - Health and readiness endpoints
  - Real-time performance metrics
  - Lease information retrieval
  - Configuration viewing
  
- **Logging**
  - Standard syslog integration
  - Categorized log levels (EMERG, ALERT, CRIT, ERR, WARNING, NOTICE, INFO, DEBUG)
  - Optional stdout logging for development

## Installation

### Prerequisites

- Go 1.21 or later
- Linux system with syslog support
- Root/sudo access (for binding to port 67 and raw socket binding for broadcast traffic)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/greeneg/go-dhcpd.git
cd go-dhcpd

# Build the binary
make build

# Install system-wide (requires sudo)
sudo make install
```

The binary will be installed to `/usr/local/bin/dhcpd`.

## Configuration

Create a configuration file at `/etc/go-dhcpd/config.json5`:

```json5
{
  "global": {
    "lease_time": 86400,           // 24 hours
    "netbios_node_type": 8,        // H-node
    "ping_timeout": 1000,          // milliseconds
    "ping_retries": 2,
    "database_path": "/var/lib/go-dhcpd/dhcpd.db",
    "listen_address": "0.0.0.0",   // Deprecated: use listen_interface instead
    "listen_interface": "",        // Network interface (e.g., "eth0", "ens33"), leave empty for all
    "api_port": 18467
  },
  
  "subnets": [
    {
      "network": "192.168.1.0",
      "netmask": "255.255.255.0",
      "dynamic_ranges": [
        {
          "start_address":"192.168.1.100",
          "end_address":"192.168.1.164"
        }
      ],
      "enable_bootp": false,
      "domain_name_servers": ["192.168.1.1", "8.8.8.8"],
      "domain_name": "example.local",
      "routers": ["192.168.1.1"]
    }
  ],
  
  "static": [
    {
      "mac_address": "aa:bb:cc:dd:ee:ff",
      "ip_address": "192.168.1.10",
      "hostname": "server1.example.local"
    }
  ]
}
```

See `config.example.json5` for a complete configuration example.

### Network Interface Binding

The daemon uses a dual-listener architecture for receiving DHCP packets:

- **Raw Socket (AF_PACKET)**: Captures broadcast DHCP packets at the Ethernet layer from clients without IP addresses (0.0.0.0 → 255.255.255.255)
- **Port 67 (UDP)**: Listens for unicast packets from DHCP relay agents and direct unicast requests on the configured IP address

The raw socket is necessary because DHCP clients that don't have an IP address yet send broadcasts from 0.0.0.0, which the kernel won't deliver through normal UDP sockets.

Both listeners can be restricted to a specific network interface:

- **Listen on all interfaces**: Leave `listen_interface` empty or set to `""`
- **Listen on specific interface**: Set to interface name (e.g., `"eth0"`, `"ens33"`, `"enp0s3"`)

The `listen_address` setting determines which IP address to bind to for port 67 (unicast/relay traffic). The raw socket listens at the Ethernet layer and can be restricted to a specific interface using `listen_interface`.

**Note**: Raw sockets require root/CAP_NET_RAW privileges.

To find your interface names:
```bash
ip link show
# or
ifconfig -a
```

## Usage

### Running the Daemon

**Note:** The daemon requires root privileges (or CAP_NET_RAW + CAP_NET_BIND_SERVICE capabilities) because it uses raw sockets to capture broadcast DHCP packets and binds to privileged port 67.

```bash
# Run with default config location
sudo dhcpd

# Run with custom config
sudo dhcpd -config /path/to/config.json5

# Run with stdout logging (for development)
sudo dhcpd -stdout

# Show version
dhcpd -version
```

### Using the Makefile

```bash
# Build only
make build

# Run with example config (development mode)
make run-dev

# Run tests
make test

# Format and vet code
make lint
```

## Web API

The daemon exposes a REST API for monitoring and management:

### Health & Status

```bash
# Health check
curl http://localhost:18467/health

# Overall metrics
curl http://localhost:18467/metrics

# DHCP-specific statistics
curl http://localhost:18467/metrics/dhcp

# Database statistics
curl http://localhost:18467/metrics/database

# Version information
curl http://localhost:18467/version
```

### Configuration

```bash
# View current configuration
curl http://localhost:18467/config
```

### Leases

```bash
# Get all leases
curl http://localhost:18467/leases

# Get active leases only
curl http://localhost:18467/leases/active

# Get lease for specific MAC address
curl http://localhost:18467/leases/aa:bb:cc:dd:ee:ff
```

### Denied Addresses

```bash
# Get all denied (martian) addresses
curl http://localhost:18467/deny
```

## Architecture

### Components

- **cmd/dhcpd** - Main daemon binary
- **internal/config** - Configuration loading and management
- **internal/db** - SQLite3 database layer for lease storage
- **internal/dhcp** - DHCP protocol implementation
- **internal/allocator** - IP address allocation logic with ping checks
- **internal/api** - Gin-based REST API server
- **internal/logger** - Syslog integration

### Database Schema

Two main tables:

1. **leases** - Stores all DHCP leases (active, expired, and static)
2. **deny_addresses** - Martian addresses detected via ping check

## Supported DHCP Options

- Subnet Mask (1)
- Router (3)
- DNS Servers (6)
- Hostname (12)
- Domain Name (15)
- Interface MTU (26)
- NTP Servers (42)
- NetBIOS Name Servers (44)
- NetBIOS DD Server (45)
- NetBIOS Node Type (46)
- Domain Search (119)

## Development

### Project Structure

```
go-dhcpd/
├── cmd/
│   └── dhcpd/           # Main binary
├── internal/
│   ├── allocator/       # IP allocation logic
│   ├── api/             # REST API
│   ├── config/          # Configuration
│   ├── db/              # Database layer
│   ├── dhcp/            # DHCP protocol
│   └── logger/          # Logging
├── Docs/                # Documentation
├── config.example.json5 # Example config
├── Makefile            # Build automation
└── README.md           # This file
```

### Running Tests

```bash
make test
```

### Code Formatting

```bash
make fmt
make vet
make lint
```

## License

See LICENSE file for details.

## Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

## References

- [RFC 2131](https://tools.ietf.org/html/rfc2131) - DHCP
- [RFC 2132](https://tools.ietf.org/html/rfc2132) - DHCP Options
- [RFC 951](https://tools.ietf.org/html/rfc951) - BootP


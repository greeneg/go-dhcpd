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

- **Plugin Architecture**
  - Subnets and static leases are supplied by an external, independently
    executed config-provider plugin
  - Bundled `file-config` plugin reads them from `config.json5` (read-only)
  - Bundled `sqlite3-config` plugin stores subnets and static hosts in a
    SQLite3 database with full CRUD support (`writable: true`)
  
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
  - HTTP Basic Auth (via local PAM) protecting all mutating/config endpoints
  - Optional TLS (HTTPS) transport for the API
  
- **Logging**
  - Standard syslog integration
  - Categorized log levels (EMERG, ALERT, CRIT, ERR, WARNING, NOTICE, INFO, DEBUG)
  - Optional stdout logging for development

## Installation

### Prerequisites

- Go 1.21 or later
- Linux system with syslog support
- Root/sudo access (for binding to port 67 and raw socket binding for broadcast traffic)
- A C compiler and PAM development headers (`pam-devel` on RHEL/Fedora/openSUSE,
  `libpam0g-dev` on Debian/Ubuntu) since the API's authentication uses cgo

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

The binary will be installed to `/usr/local/bin/dhcpd`, with man pages for
the daemon (`man dhcpd`), the migration tool (`man go-dhcpd-migration-tool`),
the configuration file format (`man go-dhcpd-config.json5`), and the
bundled plugins (`man 3 go-dhcpd-file-config`, `man 3 go-dhcpd-sqlite3-config`).

### Install via RPM Package

An RPM spec (`go-dhcpd.spec`) is provided for RHEL/Fedora/openSUSE-family
systems. It builds via the same `make build`/`make install` targets
(staged under `%{buildroot}` via `DESTDIR`), and additionally packages a
systemd unit and the correct PAM profile for the target distribution:

```bash
rpmbuild -bb go-dhcpd.spec
```

See the spec file for `BuildRequires` and the exact file layout it installs.

## API Authentication

`/health`, `/version` and the `/metrics` endpoints are always unauthenticated.
Every other endpoint (`/config`, `/subnets`, `/static`, `/leases`, `/deny`)
requires HTTP Basic Auth. Credentials are verified against the host's local
users via PAM, using the `go-dhcpd` PAM service installed to
`/etc/pam.d/go-dhcpd` by `make install` (see `pam.d/` in the repo).

`root` is always authorized. Other accounts must be explicitly allowed via
`auth.allowed_users` and/or `auth.allowed_groups` in `config.json5`:

```json5
"auth": {
  "allowed_users": ["admin"],
  "allowed_groups": ["go-dhcpd-admins"]
}
```

If both lists are empty, only `root` can reach the authenticated endpoints.

## API Transport Security (TLS)

Basic Auth credentials are sent in the clear over plain HTTP, so **enabling
TLS is strongly recommended** whenever the API is reachable over anything
but a fully trusted loopback/local link. Set `global.tls` in `config.json5`:

```json5
"tls": {
  "enabled": true,
  "cert_file": "/etc/go-dhcpd/tls/cert.pem",
  "key_file": "/etc/go-dhcpd/tls/key.pem"
}
```

`cert_file`/`key_file` must be PEM-encoded and readable by the daemon (which
already runs as root). The daemon enforces a minimum of TLS 1.2. When
`enabled` is `false` (the default), the API serves plain HTTP instead.

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
    "api_port": 18467,
    "tls": {
      "enabled": true,
      "cert_file": "/etc/go-dhcpd/tls/cert.pem",
      "key_file": "/etc/go-dhcpd/tls/key.pem"
    }
  },

  "plugins": {
    "config_provider": {
      "name": "file-config",
      // "path" is optional; defaults to
      // "<install_prefix>/lib/go-dhcpd/plugins/<name>.plugin", where
      // install_prefix is whatever INSTALL_PREFIX `make install` used
      // (baked into the binary at build time, default /usr/local)
      "settings": {
        "config_path": "/etc/go-dhcpd/config.json5"
      }
    }
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

See `config.example.json5` for a complete configuration example, or
`man go-dhcpd-config.json5` for the full option reference.

## Plugin Architecture

Subnets and static host leases are not parsed directly by the daemon.
Instead, they are supplied by a "config-provider" plugin: an independent
executable configured under `plugins.config_provider` in `config.json5`:

```json5
"plugins": {
  "config_provider": {
    "name": "file-config",
    // "path" is optional; defaults to
    // "<install_prefix>/lib/go-dhcpd/plugins/<name>.plugin", where
    // install_prefix is whatever INSTALL_PREFIX `make install` used
    // (baked into the binary at build time, default /usr/local)
    "settings": {
      "config_path": "/etc/go-dhcpd/config.json5"
    }
  }
}
```

At startup, the daemon executes the configured plugin binary once per
request, sending a JSON request on the plugin's stdin and reading a JSON
response from its stdout (see `internal/pluginapi` for the protocol). This
keeps plugins fully decoupled from the daemon process.

The bundled `file-config` plugin (`cmd/plugins/file-config`) reproduces today's
behavior: it reads the `subnets` and `static` arrays from `config.json5`
(or another file specified via the `config_path` setting). It is
intentionally **read-only** — the `/subnets` and `/static` write endpoints
in the API return `403 Forbidden` when it is the active plugin. See
`man 3 go-dhcpd-file-config` for the full plugin interface.

The bundled `sqlite3-config` plugin (`cmd/plugins/sqlite3-config`) stores
both **subnet** and **static host** definitions in a SQLite3 database and
supports full CRUD through the `/subnets` and `/static` APIs
(`writable: true`). Static hosts are keyed by (case-insensitive) MAC
address, stored in a dedicated `static_hosts` table. Configure it with:

```json5
"plugins": {
  "config_provider": {
    "name": "sqlite3-config",
    "settings": {
      "db_path": "/var/lib/go-dhcpd/networks.db"
    }
  }
}
```

Each of `SubnetConfig`'s array-valued fields (`dynamic_ranges` and the
various `[]string` server-list fields such as `domain_name_servers`,
`ntp_servers`, `routers`, etc.) is normalized into its own child table
keyed by `subnet_id`, since SQLite has no native array column type; see
`cmd/plugins/sqlite3-config/db.go` for the schema, or `man 3 go-dhcpd-sqlite3-config`
for the full plugin interface.

### Offline Migration Tool

`go-dhcpd-migration-tool` (`cmd/go-dhcpd-migration-tool`) is a standalone,
offline utility that migrates the `subnets` and `static` arrays from a
`config.json5` file into a `sqlite3-config` plugin database, for switching
an existing deployment from the `file-config` plugin to `sqlite3-config`.
It writes directly to the SQLite database file via `internal/sqlite3store`
(the same schema and validation code the `sqlite3-config` plugin uses); the
go-dhcpd daemon, its API, and the plugin process are not involved and don't
need to be running.

```bash
# Build it (included in `make build`)
make build

# Validate the configuration without writing anything
./build/go-dhcpd-migration-tool --config config.json5 --database /var/lib/go-dhcpd/networks.db --dry-run

# Migrate subnets and static hosts, creating the database/schema if needed
./build/go-dhcpd-migration-tool --config config.json5 --database /var/lib/go-dhcpd/networks.db
```

Flags (GNU-style; each has a short and long form):

- `-c, --config FILE` - Path to the `config.json5` file to read (required)
- `-d, --database FILE` - Path to the sqlite3 database file to create/update (required)
- `-n, --dry-run` - Validate the configuration without writing to the database
- `-h, --help` - Show help and exit
- `-v, --version` - Show version information and exit

Subnets and static hosts are each validated and migrated independently; a
failure in one does not stop the other, and the tool exits non-zero if any
record failed validation or insertion. After migrating, point
`plugins.config_provider` at `sqlite3-config` with `db_path` set to the
database you migrated into.

See `man go-dhcpd-migration-tool` (installed to
`/usr/local/share/man/man8/go-dhcpd-migration-tool.8` by `make install`)
for the full reference.

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

## BootP Support

`dhcpd` answers plain RFC 951 BOOTREQUESTs on any subnet with
`enable_bootp` set. BootP has no dynamic-allocation concept, so this
differs from DHCP in an important way: **a BOOTREQUEST is answered only
if the client has a matching entry in `static`** — clients without one are
silently ignored rather than given an address from `dynamic_ranges`.
BOOTREPLYs omit every DHCP-only option (message type, lease/renewal/
rebinding times, server identifier) but still carry the subnet's other
options (router, DNS, NTP, etc.).

Three additional per-subnet fields supply boot-server information in the
reply:

```json5
{
  "enable_bootp": true,
  "next_server": "192.168.1.5",     // siaddr: TFTP/boot server IP
  "server_hostname": "tftphost",    // sname (max 63 bytes)
  "boot_filename": "pxelinux.0"     // file (max 127 bytes)
}
```

All three are optional. If `boot_filename` is set but `next_server` isn't,
`next_server` defaults to this daemon's own address rather than the
otherwise-unusable `0.0.0.0`. See `man go-dhcpd-config.json5` for details.

## Usage

### Running the Daemon

**Note:** The daemon requires root privileges (or CAP_NET_RAW + CAP_NET_BIND_SERVICE capabilities) because it uses raw sockets to capture broadcast DHCP packets and binds to privileged port 67.

```bash
# Run with default config location
sudo dhcpd

# Run with custom config (-c/--config are equivalent)
sudo dhcpd --config /path/to/config.json5

# Run with stdout logging instead of syslog (-o/--stdout)
sudo dhcpd --stdout

# Show version (-v/--version) or help (-h/--help)
dhcpd --version
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

# Stage an install under a packaging buildroot instead of the real system
make install DESTDIR=/path/to/buildroot INSTALL_PREFIX=/usr
```

## Web API

The daemon exposes a REST API for monitoring and management:

### Health & Status

```bash
# Health check
curl https://localhost:18467/health

# Overall metrics
curl https://localhost:18467/metrics

# DHCP-specific statistics
curl https://localhost:18467/metrics/dhcp

# Database statistics
curl https://localhost:18467/metrics/database

# Version information
curl https://localhost:18467/version
```

### Configuration

```bash
# View current configuration
curl https://localhost:18467/config
```

### Leases

```bash
# Get all leases
curl https://localhost:18467/leases

# Get active leases only
curl https://localhost:18467/leases/active

# Get lease for specific MAC address
curl https://localhost:18467/leases/aa:bb:cc:dd:ee:ff
```

### Denied Addresses

```bash
# Get all denied (martian) addresses
curl https://localhost:18467/deny
```

## Architecture

### Components

- **cmd/dhcpd** - Main daemon binary
- **cmd/plugins/file-config**, **cmd/plugins/sqlite3-config** - Bundled config-provider plugins
- **cmd/go-dhcpd-migration-tool** - Offline `config.json5` -> `sqlite3-config` database migration tool
- **internal/config** - Configuration loading and management
- **internal/db** - SQLite3 database layer for lease storage
- **internal/dhcp** - DHCP protocol implementation
- **internal/allocator** - IP address allocation logic with ping checks
- **internal/api** - Gin-based REST API server
- **internal/pluginapi** - Daemon <-> plugin stdin/stdout JSON protocol
- **internal/sqlite3store** - Shared schema/validation used by sqlite3-config and go-dhcpd-migration-tool
- **internal/logger** - Syslog integration

### Database Schema

Two main tables:

1. **leases** - Stores all DHCP leases (active, expired, and static)
2. **deny_addresses** - Martian addresses detected via ping check

## Supported DHCP Options

- Subnet Mask (1)
- Router (3)
- Time Server (4)
- DNS Servers (6)
- LPR Server (9)
- Hostname (12)
- Domain Name (15)
- Interface MTU (26)
- NTP Servers (42)
- NetBIOS Name Servers (44)
- NetBIOS DD Server (45)
- NetBIOS Node Type (46)
- SMTP Server (69)
- Domain Search (119)

See [Docs/UnimplementedDHCPOptions.md](Docs/UnimplementedDHCPOptions.md)
for options that are *not* supported (including a few defined in code but
not fully functional, like Hostname's one-way read-only handling above).

## Development

### Project Structure

```
go-dhcpd/
├── cmd/
│   ├── dhcpd/                  # Main daemon binary
│   ├── plugins/
│   │   ├── file-config/        # Bundled read-only config-provider plugin
│   │   └── sqlite3-config/     # Bundled SQLite3-backed config-provider plugin
│   └── go-dhcpd-migration-tool/ # Offline JSON5 -> sqlite3-config migration tool
├── internal/
│   ├── allocator/       # IP allocation logic
│   ├── api/             # REST API
│   ├── config/          # Configuration
│   ├── db/              # Database layer
│   ├── dhcp/            # DHCP protocol
│   ├── pluginapi/       # Daemon <-> plugin stdin/stdout protocol
│   ├── sqlite3store/    # Shared sqlite3-config / migration-tool storage layer
│   └── logger/          # Logging
├── Docs/                # Documentation
├── man/                 # man3/man5/man8 sources, installed by `make install`
├── config.example.json5 # Example config
├── Makefile            # Build automation
├── go-dhcpd.spec        # RPM packaging spec
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


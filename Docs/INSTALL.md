# Installation Guide

## Prerequisites

- Linux system (tested on RHEL/CentOS/Fedora, Ubuntu/Debian, openSUSE)
- Go 1.21 or later (for building from source)
- A C compiler and PAM development headers (`pam-devel` on RHEL/Fedora/openSUSE,
  `libpam0g-dev` on Debian/Ubuntu) - the API's Basic Auth uses PAM via cgo
- Root/sudo access
- Network interface to serve DHCP requests

## Installation Steps

### 1. Build the Binary

```bash
cd /Users/greeneg/Development/go-dhcpd
make build
```

This will create the `dhcpd` binary and the `go-dhcpd-migration-tool`
utility in the `build/` directory, along with the bundled `file-config`
and `sqlite3-config` config-provider plugins in `build/plugins/`.

### 2. Install System-Wide

```bash
sudo make install
```

By default this installs under `/usr/local` (`INSTALL_PREFIX`). To install
elsewhere, pass `INSTALL_PREFIX` to both `build` and `install` so the
binary is compiled knowing where its plugin directory will live, e.g.
`sudo make build install INSTALL_PREFIX=/opt/go-dhcpd`. Packagers building
a staged install (e.g. into an RPM `%{buildroot}`) instead of the real
system should also pass `DESTDIR`, e.g. `make install DESTDIR=/path/to/root
INSTALL_PREFIX=/usr` (see `go-dhcpd.spec` below for a worked example).

This will:
- Install `dhcpd` and `go-dhcpd-migration-tool` to `$(INSTALL_PREFIX)/bin/`
- Install the `file-config` and `sqlite3-config` config-provider plugins to
  `$(INSTALL_PREFIX)/lib/go-dhcpd/plugins/`
- Install man pages for the daemon, the migration tool, the configuration
  file format, and both plugins under `$(INSTALL_PREFIX)/share/man/`
- Install the `go-dhcpd` PAM service to `/etc/pam.d/go-dhcpd` (auto-detecting
  a `system-auth` or `common-auth` style stack; see `pam.d/` if neither is
  found and it needs installing manually)
- Create configuration directory at `/etc/go-dhcpd/`
- Create data directory at `/var/lib/go-dhcpd/`
- Copy example configuration

### 2a. Alternative: Install via RPM Package

On RHEL/Fedora/openSUSE-family systems, `go-dhcpd.spec` builds a package
using the same `make build`/`make install` targets, and additionally
installs a systemd unit (with `ExecStart` already pointed at the packaged
binary path) and the PAM profile matching the target distribution:

```bash
mkdir -p "$(rpm --eval '%{_sourcedir}')"
curl -fL https://github.com/greeneg/go-dhcpd/archive/refs/tags/v1.9.1.tar.gz \
  -o "$(rpm --eval '%{_sourcedir}')/go-dhcpd-1.9.1.tar.gz"
rpmbuild -bb go-dhcpd.spec
sudo rpm -i ~/rpmbuild/RPMS/*/go-dhcpd-*.rpm
```

If you install this way, skip steps 1-2 and the unit-copy/daemon-reload
commands in step 5; the package handles those. Continue from step 3 to
configure the server, then run `sudo systemctl enable --now go-dhcpd`
before proceeding to step 6.

### 3. Configure the Server

Edit `/etc/go-dhcpd/config.json5`:

```bash
sudo cp /etc/go-dhcpd/config.json5.example /etc/go-dhcpd/config.json5
sudo vim /etc/go-dhcpd/config.json5
```

At minimum, configure:
- `global.tls` - **Strongly recommended:** enable HTTPS for the API so Basic Auth credentials aren't sent in the clear (see below)
- `auth.allowed_users` / `auth.allowed_groups` - Non-root accounts allowed to reach authenticated API endpoints (root is always allowed)
- `plugins.config_provider` - Points at the config-provider plugin binary (defaults to the installed `file-config` plugin)
- `subnets` - Define your network ranges (read by the `file-config` plugin)
- `dynamic_ranges` - DHCP pool using CIDR notation
- `domain_name_servers` - DNS servers for clients
- `routers` - Default gateway for clients

To instead store subnets/static hosts in a SQLite3 database with full
read/write API support, switch `plugins.config_provider.name` to
`sqlite3-config` (set its `db_path` setting) and, if migrating an existing
config, run `go-dhcpd-migration-tool` first to copy your `subnets`/`static`
data into that database — see `man go-dhcpd-migration-tool` and `man 3
go-dhcpd-sqlite3-config`.

#### Enable BootP (Optional)

Set `enable_bootp: true` on a subnet to answer plain RFC 951 BOOTREQUESTs
on it. Unlike DHCP, BootP only ever serves clients with a matching entry
in `static` — add one before enabling it, or clients will be silently
ignored. `next_server`/`server_hostname`/`boot_filename` optionally supply
boot-server information in the reply; see `man go-dhcpd-config.json5` for
the full field reference.

#### Enable TLS for the API

Generate or install a certificate/key pair (a self-signed pair is fine for
internal use; use a CA-issued one where possible), e.g.:

```bash
sudo mkdir -p /etc/go-dhcpd/tls
sudo openssl req -x509 -newkey rsa:2048 -nodes -days 825 \
  -keyout /etc/go-dhcpd/tls/key.pem -out /etc/go-dhcpd/tls/cert.pem \
  -subj "/CN=$(hostname -f)"
sudo chmod 600 /etc/go-dhcpd/tls/key.pem
```

Then set in `config.json5`:

```json5
"global": {
  ...
  "tls": {
    "enabled": true,
    "cert_file": "/etc/go-dhcpd/tls/cert.pem",
    "key_file": "/etc/go-dhcpd/tls/key.pem"
  }
}
```

### 4. Create Database Directory

```bash
sudo mkdir -p /var/lib/go-dhcpd
sudo chmod 755 /var/lib/go-dhcpd
```

### 5. Install Systemd Service (Optional)

```bash
sudo cp go-dhcpd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable go-dhcpd
sudo systemctl start go-dhcpd
```

**Note:** the shipped unit's `ExecStart` hard-codes
`/usr/local/bin/dhcpd`. If you installed with a custom `INSTALL_PREFIX`,
edit `ExecStart` to match before starting the service (the RPM package
does this automatically).

### 6. Verify Installation

Check the service status:

```bash
sudo systemctl status go-dhcpd
```

Check the API:

```bash
curl https://localhost:18467/health
curl https://localhost:18467/metrics
```

View logs:

```bash
# If using syslog
sudo journalctl -u go-dhcpd -f

# Or check system logs
sudo tail -f /var/log/messages | grep go-dhcpd
```

## Manual Running

To run manually (useful for testing):

```bash
# Run with stdout logging
sudo /usr/local/bin/dhcpd --config /etc/go-dhcpd/config.json5 --stdout

# Run with syslog
sudo /usr/local/bin/dhcpd --config /etc/go-dhcpd/config.json5
```

## Firewall Configuration

Ensure UDP port 67 is open for DHCP:

```bash
# For firewalld (RHEL/CentOS/Fedora)
sudo firewall-cmd --permanent --add-service=dhcp
sudo firewall-cmd --reload

# For ufw (Ubuntu/Debian)
sudo ufw allow 67/udp
sudo ufw allow 18467/tcp  # For API

# For iptables
sudo iptables -A INPUT -p udp --dport 67 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 18467 -j ACCEPT
```

## Network Configuration

Make sure no other DHCP server is running on the network:

```bash
# Stop and disable other DHCP servers
sudo systemctl stop dhcpd isc-dhcp-server dnsmasq
sudo systemctl disable dhcpd isc-dhcp-server dnsmasq
```

## Testing

1. Check that the server is listening:
```bash
sudo netstat -ulnp | grep :67
```

2. Test DHCP from a client machine or use `dhcping`:
```bash
sudo dhcping -s 192.168.1.1
```

3. Check the API for leases:
```bash
curl https://localhost:18467/leases
```

## Troubleshooting

### Server won't start

1. Check if port 67 is already in use:
```bash
sudo netstat -ulnp | grep :67
```

2. Check configuration syntax:
```bash
sudo /usr/local/bin/dhcpd --config /etc/go-dhcpd/config.json5 --stdout
```

3. Check permissions on database directory:
```bash
ls -ld /var/lib/go-dhcpd
```

### No leases being issued

1. Check firewall rules
2. Verify network connectivity
3. Check subnet configuration matches your network
4. Review logs for errors

### API not accessible

1. Check if API port is configured correctly in config
2. Verify API is listening:
```bash
sudo netstat -tlnp | grep :18467
```

## Uninstallation

```bash
sudo systemctl stop go-dhcpd
sudo systemctl disable go-dhcpd
sudo make uninstall
sudo rm -rf /var/lib/go-dhcpd
sudo rm -rf /etc/go-dhcpd
sudo rm /etc/systemd/system/go-dhcpd.service
sudo systemctl daemon-reload
```

If installed via the RPM package instead, use `sudo rpm -e go-dhcpd`
(or your package manager's equivalent) in place of `make uninstall` and
the manual systemd-unit removal above; it is wired to the usual
`%post`/`%preun`/`%postun` systemd scriptlets.

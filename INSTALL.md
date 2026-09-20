# Installation Guide

## Prerequisites

- Linux system (tested on RHEL/CentOS/Fedora, Ubuntu/Debian)
- Go 1.21 or later (for building from source)
- Root/sudo access
- Network interface to serve DHCP requests

## Installation Steps

### 1. Build the Binary

```bash
cd /Users/greeneg/Development/go-dhcpd
make build
```

This will create the `dhcpd` binary in the `build/` directory.

### 2. Install System-Wide

```bash
sudo make install
```

This will:
- Install the binary to `/usr/local/bin/dhcpd`
- Create configuration directory at `/etc/go-dhcpd/`
- Create data directory at `/var/lib/go-dhcpd/`
- Copy example configuration

### 3. Configure the Server

Edit `/etc/go-dhcpd/config.json5`:

```bash
sudo cp /etc/go-dhcpd/config.json5.example /etc/go-dhcpd/config.json5
sudo vim /etc/go-dhcpd/config.json5
```

At minimum, configure:
- `subnets` - Define your network ranges
- `dynamic_ranges` - DHCP pool using CIDR notation
- `domain_name_servers` - DNS servers for clients
- `routers` - Default gateway for clients

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

### 6. Verify Installation

Check the service status:

```bash
sudo systemctl status go-dhcpd
```

Check the API:

```bash
curl http://localhost:18467/health
curl http://localhost:18467/metrics
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
sudo /usr/local/bin/dhcpd -config /etc/go-dhcpd/config.json5 -stdout

# Run with syslog
sudo /usr/local/bin/dhcpd -config /etc/go-dhcpd/config.json5
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
curl http://localhost:18467/leases
```

## Troubleshooting

### Server won't start

1. Check if port 67 is already in use:
```bash
sudo netstat -ulnp | grep :67
```

2. Check configuration syntax:
```bash
sudo /usr/local/bin/dhcpd -config /etc/go-dhcpd/config.json5 -stdout
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

# Integration Test Harness for go-dhcpd

This directory contains a complete integration test harness for go-dhcpd using libvirt/KVM virtual machines.

## Overview

The test harness creates two Fedora Server VMs on an isolated network:

- **go-dhcpd-server**: Runs the DHCP server (192.168.100.10)
- **go-dhcpd-client**: Acts as a DHCP client (receives dynamic IP)

The VMs communicate over a private virtual network (`go-dhcpd-test-net` - 192.168.100.0/24) with no external connectivity, ensuring isolated testing.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Host System                           │
│                                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │         go-dhcpd-test-net (192.168.100.0/24)       │ │
│  │                (virbr-dhcpd bridge)                 │ │
│  │                                                     │ │
│  │  ┌──────────────────┐      ┌──────────────────┐  │ │
│  │  │  go-dhcpd-server │      │  go-dhcpd-client │  │ │
│  │  │                  │      │                  │  │ │
│  │  │  192.168.100.10  │      │  DHCP (dynamic)  │  │ │
│  │  │                  │      │                  │  │ │
│  │  │  - go-dhcpd      │      │  - dhclient      │  │ │
│  │  │  - API :18467    │      │  - test tools    │  │ │
│  │  │  - SQLite DB     │      │                  │  │ │
│  │  └──────────────────┘      └──────────────────┘  │ │
│  │                                                     │ │
│  └────────────────────────────────────────────────────┘ │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Requirements

### Host System

- Linux host with KVM support
- libvirt and QEMU installed
- virsh, virt-install, qemu-img, virt-manager
- At least 10GB free disk space
- At least 4GB RAM (preferably 8GB+)
- sudo/root privileges

### Installation (Fedora/RHEL)

```bash
sudo dnf install libvirt qemu-kvm virt-install virt-manager virt-viewer
sudo systemctl enable --now libvirtd
sudo usermod -aG libvirt $USER
# Re-login for group changes to take effect
```

### Installation (Ubuntu/Debian)

```bash
sudo apt install libvirt-daemon qemu-kvm virtinst virt-manager virt-viewer
sudo systemctl enable --now libvirtd
sudo usermod -aG libvirt $USER
# Re-login for group changes to take effect
```

## Quick Start

### 1. Initial Setup

```bash
cd tests/integration

# Download Fedora ISO, create network, and define VMs
./provision.sh setup
```

This will:
- Download Fedora Server ISO (~2-3 GB)
- Create the isolated test network
- Define both VMs (but not start them)

### 2. Install Fedora on VMs

You need to manually install Fedora on both VMs. You can do this in two ways:

#### Option A: Using virt-manager (GUI)

```bash
# Start virt-manager
virt-manager

# For each VM (server and client):
# 1. Right-click VM → Open
# 2. Click "Add Hardware" → Storage → CDROM
# 3. Select the Fedora ISO
# 4. Start the VM and complete Fedora installation
# 5. After installation, remove the CDROM device
```

#### Option B: Using virsh (CLI)

```bash
# Attach ISO to server VM
sudo virsh attach-disk go-dhcpd-server \
  /var/lib/libvirt/images/fedora-41-server.iso \
  sda --type cdrom --mode readonly

# Start the VM
sudo virsh start go-dhcpd-server

# Connect to console (or use VNC)
sudo virsh console go-dhcpd-server
# Or: virt-viewer go-dhcpd-server

# Complete Fedora installation
# After installation, reboot and detach ISO:
sudo virsh detach-disk go-dhcpd-server sda

# Repeat for client VM
```

### 3. Configure VMs

After Fedora installation, run the setup scripts inside each VM:

#### Server VM

```bash
# SSH or console into the server VM
sudo virsh console go-dhcpd-server

# Transfer and run setup script
# (Copy setup-server.sh to the VM, then run:)
sudo bash setup-server.sh
```

#### Client VM

```bash
# SSH or console into the client VM
sudo virsh console go-dhcpd-client

# Transfer and run setup script
# (Copy setup-client.sh to the VM, then run:)
sudo bash setup-client.sh
```

### 4. Deploy go-dhcpd Binary

```bash
# Build and deploy to server VM
./deploy-server.sh
```

This will:
- Build the go-dhcpd binary
- Copy it to the server VM
- Install it to `/usr/local/bin/dhcpd`
- Set required capabilities
- Start the systemd service

### 5. Run Tests

```bash
# Run automated integration tests
./run-tests.sh
```

## Files

### VM Definitions

- `dhcpd-server.xml` - Libvirt domain definition for server VM
- `dhcpd-client.xml` - Libvirt domain definition for client VM  
- `test-network.xml` - Isolated network definition

### Scripts

- `provision.sh` - Main provisioning script (create/manage VMs)
- `setup-server.sh` - Server VM configuration script (run inside VM)
- `setup-client.sh` - Client VM configuration script (run inside VM)
- `deploy-server.sh` - Build and deploy binary to server
- `run-tests.sh` - Automated integration test suite

## Usage

### Managing VMs

```bash
# Check status
./provision.sh status

# Start VMs
./provision.sh start

# Stop VMs
./provision.sh stop

# Destroy VMs and network (careful!)
./provision.sh destroy

# Connect to VM console
./provision.sh console-server
./provision.sh console-client
```

### Testing DHCP

#### On Client VM

```bash
# Test DHCP lease acquisition
test-dhcp.sh

# Capture DHCP traffic
capture-dhcp.sh

# Manual DHCP operations
sudo dhclient -r eth0    # Release lease
sudo dhclient -v eth0    # Request lease (verbose)
ip addr show eth0        # Show IP address
```

#### From Host

```bash
# Check server API
curl http://192.168.100.10:18467/health
curl http://192.168.100.10:18467/config
curl http://192.168.100.10:18467/leases
curl http://192.168.100.10:18467/metrics

# Run full test suite
./run-tests.sh
```

### Monitoring

#### Server Logs

```bash
# Connect to server console
./provision.sh console-server

# View service logs
sudo journalctl -u go-dhcpd -f

# Check service status
sudo systemctl status go-dhcpd
```

#### Network Traffic

```bash
# On server VM
sudo tcpdump -i eth0 port 67 or port 68 -v

# On client VM
sudo tcpdump -i eth0 port 67 or port 68 -v
```

## Test Configuration

The server is configured with:

- **Network**: 192.168.100.0/24
- **Server IP**: 192.168.100.10
- **Dynamic Range**: 192.168.100.100 - 192.168.100.200
- **Lease Time**: 3600 seconds (1 hour)
- **DNS Servers**: 192.168.100.10, 8.8.8.8
- **Gateway**: 192.168.100.1
- **Domain**: dhcpd.test

Configuration file: `/etc/go-dhcpd/config.json5` on server VM

## Automated Tests

The `run-tests.sh` script performs:

1. ✓ Server health check (API endpoint)
2. ✓ API endpoint tests (/config, /metrics, /version)
3. ✓ DHCP lease acquisition
4. ✓ Lease database verification
5. ✓ DHCP options delivery (DNS, gateway)

## Troubleshooting

### VMs won't start

```bash
# Check libvirtd status
sudo systemctl status libvirtd

# Check VM status
sudo virsh list --all

# View VM logs
sudo journalctl -u libvirtd -f
```

### DHCP server not starting

```bash
# Connect to server console
./provision.sh console-server

# Check service status
sudo systemctl status go-dhcpd

# View detailed logs
sudo journalctl -u go-dhcpd -n 100

# Check if binary has capabilities
getcap /usr/local/bin/dhcpd
```

### Client not getting IP

```bash
# On client VM
sudo dhclient -r eth0
sudo dhclient -v eth0

# Check network interface
ip addr show eth0
ip link show eth0

# Capture DHCP traffic
sudo tcpdump -i eth0 port 67 or port 68 -v
```

### Network connectivity issues

```bash
# Check if network is running
sudo virsh net-list --all

# Restart network
sudo virsh net-destroy go-dhcpd-test-net
sudo virsh net-start go-dhcpd-test-net

# Check bridge
ip addr show virbr-dhcpd
```

## Customization

### Change Fedora Version

```bash
FEDORA_VERSION=44 ./provision.sh download
```

### Change Network Range

Edit `test-network.xml` and `setup-server.sh` to use different IP ranges.

### Add Static Leases

Edit `/etc/go-dhcpd/config.json5` on server VM:

```json5
"static": [
  {
    "mac_address": "52:54:00:xx:xx:xx",
    "ip_address": "192.168.100.50",
    "hostname": "testhost.dhcpd.test"
  }
]
```

Restart the service: `sudo systemctl restart go-dhcpd`

## Cleanup

```bash
# Stop VMs
./provision.sh stop

# Destroy everything (VMs, network, disks)
./provision.sh destroy

# Remove downloaded ISO (optional)
sudo rm /var/lib/libvirt/images/fedora-*.iso
```

## Contributing

When adding new tests:

1. Add test functions to `run-tests.sh`
2. Update test count in summary
3. Document any new configuration requirements
4. Update this README

## Notes

- VMs use 2GB RAM (server) and 1GB RAM (client) - adjust in XML if needed
- Disk images are in qcow2 format (20GB each, thin-provisioned)
- VNC console is available on localhost (use virt-viewer)
- Network is completely isolated - no internet access from VMs
- SELinux/AppArmor may require additional configuration depending on your host

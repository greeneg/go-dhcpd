#!/bin/bash
#
# Setup script for go-dhcpd-server VM
#
# Run this script INSIDE the server VM after Fedora installation
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    log_error "Please run as root (or with sudo)"
    exit 1
fi

log_info "Setting up go-dhcpd server VM..."

# Update system
log_info "Updating system packages..."
dnf update -y

# Install required packages
log_info "Installing required packages..."
dnf install -y \
    golang \
    git \
    make \
    gcc \
    sqlite \
    tcpdump \
    vim \
    net-tools \
    iproute \
    iputils \
    bind-utils \
    qemu-guest-agent

# Start and enable qemu-guest-agent
systemctl enable --now qemu-guest-agent

# Configure static IP
log_info "Configuring static IP address..."

# Find the network interface (usually eth0 or enp1s0)
IFACE=$(ip -o link show | awk -F': ' '{print $2}' | grep -v lo | head -1)
log_info "Using interface: $IFACE"

# Create NetworkManager connection for static IP
nmcli connection modify "$IFACE" \
    ipv4.method manual \
    ipv4.addresses 192.168.100.10/24 \
    ipv4.gateway 192.168.100.1 \
    ipv4.dns "8.8.8.8 8.8.4.4"

nmcli connection up "$IFACE"

log_success "Static IP configured: 192.168.100.10"

# Create go-dhcpd user
log_info "Creating go-dhcpd user..."
if ! id -u go-dhcpd &>/dev/null; then
    useradd -r -s /bin/bash -d /var/lib/go-dhcpd -m go-dhcpd
fi

# Create directories
log_info "Creating directories..."
mkdir -p /etc/go-dhcpd
mkdir -p /var/lib/go-dhcpd
mkdir -p /var/log/go-dhcpd

# Set permissions
chown -R go-dhcpd:go-dhcpd /var/lib/go-dhcpd
chown -R go-dhcpd:go-dhcpd /var/log/go-dhcpd

# Create test configuration
log_info "Creating test configuration..."
cat > /etc/go-dhcpd/config.json5 << 'EOF'
{
  "global": {
    "lease_time": 3600,                // 1 hour for testing
    "netbios_node_type": 8,
    "ping_timeout": 1000,
    "ping_retries": 2,
    "database_path": "/var/lib/go-dhcpd/dhcpd.db",
    "listen_address": "192.168.100.10",
    "listen_interface": "",             // Listen on all interfaces
    "api_port": 18467
  },
  
  "subnets": [
    {
      "network": "192.168.100.0",
      "netmask": "255.255.255.0",
      "dynamic_ranges": [
        {
          "start_address": "192.168.100.100",
          "end_address": "192.168.100.200"
        }
      ],
      "enable_bootp": false,
      "domain_name_servers": ["192.168.100.10", "8.8.8.8"],
      "domain_name": "dhcpd.test",
      "domain_search": ["dhcpd.test"],
      "interface_mtu": 1500,
      "ntp_servers": ["192.168.100.10"],
      "routers": ["192.168.100.1"],
      "netbios_name_servers": ["192.168.100.10"],
      "netbios_dd_servers": ["192.168.100.10"],
      "netbios_node_type": 8
    }
  ],
  
  "static": []
}
EOF

chown go-dhcpd:go-dhcpd /etc/go-dhcpd/config.json5

# Add firewall rules
log_info "Configuring firewall..."
if command -v firewall-cmd &> /dev/null; then
    firewall-cmd --permanent --add-service=dhcp
    firewall-cmd --permanent --add-port=18467/tcp  # API port
    firewall-cmd --reload
fi

# Set up capabilities for running without root
log_info "Setting up capabilities..."
log_warning "Binary needs to be built first!"
log_info "After building go-dhcpd, run:"
log_info "  sudo setcap cap_net_raw,cap_net_bind_service=+ep /path/to/dhcpd"

# Create systemd service file
log_info "Creating systemd service..."
cat > /etc/systemd/system/go-dhcpd.service << 'EOF'
[Unit]
Description=Go DHCP Server
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/dhcpd -config /etc/go-dhcpd/config.json5
Restart=on-failure
RestartSec=5s

# Security settings
CapabilityBoundingSet=CAP_NET_RAW CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_RAW CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload

log_success "Server VM setup complete!"
log_info ""
log_info "Next steps:"
log_info "  1. Transfer go-dhcpd binary to /usr/local/bin/dhcpd"
log_info "  2. Or build from source:"
log_info "     cd /opt && git clone <repo-url> go-dhcpd"
log_info "     cd go-dhcpd && make build"
log_info "     sudo cp build/dhcpd /usr/local/bin/"
log_info "  3. Start service: sudo systemctl start go-dhcpd"
log_info "  4. Enable on boot: sudo systemctl enable go-dhcpd"
log_info "  5. Check status: sudo systemctl status go-dhcpd"
log_info "  6. View logs: sudo journalctl -u go-dhcpd -f"

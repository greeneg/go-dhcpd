#!/bin/bash
#
# Setup script for go-dhcpd-client VM
#
# Run this script INSIDE the client VM after Fedora installation
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

log_info "Setting up go-dhcpd client VM..."

# Update system
log_info "Updating system packages..."
dnf update -y

# Install required packages
log_info "Installing required packages..."
dnf install -y \
    tcpdump \
    vim \
    net-tools \
    iproute \
    iputils \
    bind-utils \
    dhcp-client \
    qemu-guest-agent

# Start and enable qemu-guest-agent
systemctl enable --now qemu-guest-agent

# Configure DHCP on network interface
log_info "Configuring network interface for DHCP..."

# Find the network interface (usually eth0 or enp1s0)
IFACE=$(ip -o link show | awk -F': ' '{print $2}' | grep -v lo | head -1)
log_info "Using interface: $IFACE"

# Create NetworkManager connection for DHCP
nmcli connection modify "$IFACE" \
    ipv4.method auto \
    ipv4.dhcp-timeout 300

# Ensure the interface comes up on boot
nmcli connection modify "$IFACE" connection.autoconnect yes

log_success "Network interface configured for DHCP"

# Create test script
log_info "Creating test scripts..."

cat > /usr/local/bin/test-dhcp.sh << 'EOF'
#!/bin/bash
#
# Test DHCP functionality
#

IFACE=$(ip -o link show | awk -F': ' '{print $2}' | grep -v lo | head -1)

echo "=== DHCP Test ==="
echo "Interface: $IFACE"
echo ""

# Show current IP
echo "Current IP configuration:"
ip addr show "$IFACE"
echo ""

# Release current lease
echo "Releasing current DHCP lease..."
sudo dhclient -r "$IFACE" 2>/dev/null || true
sleep 2

# Request new lease
echo "Requesting new DHCP lease..."
sudo dhclient -v "$IFACE"

echo ""
echo "New IP configuration:"
ip addr show "$IFACE"
echo ""

echo "Routing table:"
ip route
echo ""

echo "DNS servers:"
cat /etc/resolv.conf
echo ""

echo "DHCP lease info:"
if [ -f "/var/lib/dhclient/dhclient--${IFACE}.lease" ]; then
    cat "/var/lib/dhclient/dhclient--${IFACE}.lease"
elif [ -f "/var/lib/NetworkManager/dhclient-${IFACE}.lease" ]; then
    cat "/var/lib/NetworkManager/dhclient-${IFACE}.lease"
else
    echo "Lease file not found"
fi
EOF

chmod +x /usr/local/bin/test-dhcp.sh

cat > /usr/local/bin/capture-dhcp.sh << 'EOF'
#!/bin/bash
#
# Capture DHCP traffic for debugging
#

IFACE=$(ip -o link show | awk -F': ' '{print $2}' | grep -v lo | head -1)
OUTPUT_FILE="/tmp/dhcp-capture-$(date +%Y%m%d-%H%M%S).pcap"

echo "Capturing DHCP traffic on $IFACE..."
echo "Output file: $OUTPUT_FILE"
echo "Press Ctrl+C to stop"
echo ""

sudo tcpdump -i "$IFACE" -w "$OUTPUT_FILE" '(port 67 or port 68)' -v
EOF

chmod +x /usr/local/bin/capture-dhcp.sh

log_success "Client VM setup complete!"
log_info ""
log_info "Available test commands:"
log_info "  test-dhcp.sh       - Test DHCP lease acquisition"
log_info "  capture-dhcp.sh    - Capture DHCP traffic with tcpdump"
log_info ""
log_info "Manual DHCP testing:"
log_info "  Release lease:   sudo dhclient -r $IFACE"
log_info "  Request lease:   sudo dhclient -v $IFACE"
log_info "  Show IP:         ip addr show $IFACE"
log_info "  Restart network: sudo nmcli connection down $IFACE && sudo nmcli connection up $IFACE"

#!/bin/bash
#
# Deploy go-dhcpd binary to the server VM
#
# This script builds the go-dhcpd binary and deploys it to the server VM
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

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

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Server VM connection details
SERVER_VM="go-dhcpd-server"
SERVER_IP="192.168.100.10"
SERVER_USER="${SERVER_USER:-root}"

# Check if VM is running
if ! sudo virsh list --state-running | grep -q "$SERVER_VM"; then
    log_error "Server VM is not running"
    log_info "Start it with: ./provision.sh start"
    exit 1
fi

# Build the binary
log_info "Building go-dhcpd binary..."
cd "$PROJECT_ROOT"
make build

if [ ! -f "$PROJECT_ROOT/build/dhcpd" ]; then
    log_error "Build failed - binary not found"
    exit 1
fi

log_success "Binary built successfully"

# Get the VM's IP address (may take a moment for DHCP if client is set up)
log_info "Connecting to server VM at ${SERVER_IP}..."

# Create temporary script to copy and install
TEMP_SCRIPT=$(mktemp)
cat > "$TEMP_SCRIPT" << 'EOFSCRIPT'
#!/bin/bash
set -e

# Stop service if running
systemctl stop go-dhcpd 2>/dev/null || true

# Install binary
install -m 755 /tmp/dhcpd /usr/local/bin/dhcpd

# Set capabilities
setcap cap_net_raw,cap_net_bind_service=+ep /usr/local/bin/dhcpd

# Start service
systemctl start go-dhcpd

# Show status
systemctl status go-dhcpd --no-pager

echo ""
echo "Binary deployed and service restarted"
EOFSCRIPT

# Copy binary to VM
log_info "Copying binary to VM..."
sudo virt-copy-in -d "$SERVER_VM" "$PROJECT_ROOT/build/dhcpd" /tmp/

# Copy and run install script
log_info "Installing binary on VM..."
sudo virt-copy-in -d "$SERVER_VM" "$TEMP_SCRIPT" /tmp/
sudo virt-customize -d "$SERVER_VM" --run-command "chmod +x /tmp/$(basename $TEMP_SCRIPT) && /tmp/$(basename $TEMP_SCRIPT)"

rm -f "$TEMP_SCRIPT"

log_success "Deployment complete!"
log_info ""
log_info "Check service status:"
log_info "  ./provision.sh console-server"
log_info "  sudo systemctl status go-dhcpd"
log_info ""
log_info "View logs:"
log_info "  sudo journalctl -u go-dhcpd -f"
log_info ""
log_info "Check API:"
log_info "  curl http://192.168.100.10:18467/health"

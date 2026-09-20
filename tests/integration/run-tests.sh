#!/bin/bash
#
# Run integration tests for go-dhcpd
#
# This script runs automated tests against the DHCP server
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

log_test() {
    echo -e "${YELLOW}[TEST]${NC} $*"
}

# Test results
TESTS_PASSED=0
TESTS_FAILED=0

test_result() {
    if [ $1 -eq 0 ]; then
        log_success "✓ $2"
        ((TESTS_PASSED++))
    else
        log_error "✗ $2"
        ((TESTS_FAILED++))
    fi
}

# Check VMs are running
check_vms() {
    log_info "Checking VM status..."
    
    for vm in go-dhcpd-server go-dhcpd-client; do
        if ! sudo virsh list --state-running | grep -q "$vm"; then
            log_error "$vm is not running"
            log_info "Start VMs with: ./provision.sh start"
            return 1
        fi
    done
    
    log_success "Both VMs are running"
}

# Test server health
test_server_health() {
    log_test "Testing server health endpoint..."
    
    if curl -s -f http://192.168.100.10:18467/health > /dev/null 2>&1; then
        test_result 0 "Server health endpoint responds"
    else
        test_result 1 "Server health endpoint unreachable"
    fi
}

# Test server API
test_server_api() {
    log_test "Testing server API endpoints..."
    
    # Test /config
    if curl -s -f http://192.168.100.10:18467/config | grep -q "subnets"; then
        test_result 0 "Server /config endpoint returns valid data"
    else
        test_result 1 "Server /config endpoint failed"
    fi
    
    # Test /metrics
    if curl -s -f http://192.168.100.10:18467/metrics > /dev/null 2>&1; then
        test_result 0 "Server /metrics endpoint responds"
    else
        test_result 1 "Server /metrics endpoint failed"
    fi
    
    # Test /version
    if curl -s -f http://192.168.100.10:18467/version | grep -q "version"; then
        test_result 0 "Server /version endpoint returns valid data"
    else
        test_result 1 "Server /version endpoint failed"
    fi
}

# Test DHCP lease acquisition
test_dhcp_lease() {
    log_test "Testing DHCP lease acquisition on client..."
    
    # Create test script to run in client VM
    cat > /tmp/test-dhcp-client.sh << 'EOF'
#!/bin/bash
IFACE=$(ip -o link show | awk -F': ' '{print $2}' | grep -v lo | head -1)

# Release current lease
dhclient -r "$IFACE" 2>/dev/null || true
sleep 2

# Request new lease with timeout
timeout 30 dhclient -v "$IFACE" 2>&1

# Check if we got an IP in the expected range
IP=$(ip addr show "$IFACE" | grep -oP '(?<=inet\s)\d+\.\d+\.\d+\.\d+' | head -1)

if [[ "$IP" =~ ^192\.168\.100\.(10[0-9]|1[0-9][0-9]|200)$ ]]; then
    echo "SUCCESS: Got IP $IP in expected range"
    exit 0
else
    echo "FAILED: Got IP $IP outside expected range (192.168.100.100-200)"
    exit 1
fi
EOF
    
    # Copy script to client VM and execute
    if sudo virt-copy-in -d go-dhcpd-client /tmp/test-dhcp-client.sh /tmp/ && \
       sudo virt-customize -d go-dhcpd-client --run-command "chmod +x /tmp/test-dhcp-client.sh && /tmp/test-dhcp-client.sh" 2>&1 | grep -q "SUCCESS"; then
        test_result 0 "Client acquired DHCP lease in expected range"
    else
        test_result 1 "Client failed to acquire DHCP lease"
    fi
    
    rm -f /tmp/test-dhcp-client.sh
}

# Test lease database
test_lease_database() {
    log_test "Testing lease database..."
    
    # Check if leases are recorded
    if curl -s http://192.168.100.10:18467/leases | grep -q "ip_address"; then
        test_result 0 "Lease database contains entries"
    else
        test_result 1 "Lease database is empty or unreachable"
    fi
}

# Test DHCP options
test_dhcp_options() {
    log_test "Testing DHCP options delivery..."
    
    # Create test script to check received options
    cat > /tmp/test-dhcp-options.sh << 'EOF'
#!/bin/bash

# Check resolv.conf for DNS servers
if grep -q "192.168.100.10" /etc/resolv.conf; then
    echo "DNS server configured correctly"
else
    echo "DNS server not configured"
    exit 1
fi

# Check routing table for gateway
if ip route | grep -q "default via 192.168.100.1"; then
    echo "Default gateway configured correctly"
else
    echo "Default gateway not configured"
    exit 1
fi

echo "SUCCESS: DHCP options applied correctly"
exit 0
EOF
    
    if sudo virt-copy-in -d go-dhcpd-client /tmp/test-dhcp-options.sh /tmp/ && \
       sudo virt-customize -d go-dhcpd-client --run-command "chmod +x /tmp/test-dhcp-options.sh && /tmp/test-dhcp-options.sh" 2>&1 | grep -q "SUCCESS"; then
        test_result 0 "DHCP options (DNS, gateway) configured correctly"
    else
        test_result 1 "DHCP options not configured correctly"
    fi
    
    rm -f /tmp/test-dhcp-options.sh
}

# Show test summary
show_summary() {
    echo ""
    echo "======================================"
    echo "Test Summary"
    echo "======================================"
    echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"
    echo "Total:  $((TESTS_PASSED + TESTS_FAILED))"
    echo "======================================"
    
    if [ $TESTS_FAILED -eq 0 ]; then
        log_success "All tests passed!"
        return 0
    else
        log_error "Some tests failed"
        return 1
    fi
}

# Main test execution
main() {
    log_info "Starting integration tests for go-dhcpd"
    echo ""
    
    if ! check_vms; then
        exit 1
    fi
    
    echo ""
    
    # Run tests
    test_server_health
    test_server_api
    test_dhcp_lease
    test_lease_database
    test_dhcp_options
    
    # Show results
    show_summary
}

main "$@"

#!/bin/bash
#
# Integration Test VM Provisioner for go-dhcpd
# 
# This script provisions two Fedora VMs for DHCP integration testing:
# - go-dhcpd-server: Runs the DHCP server
# - go-dhcpd-client: Acts as a DHCP client
#
# Requirements:
# - libvirt and KVM installed
# - virsh command available
# - qemu-img command available
# - sudo privileges
# - At least 10GB free disk space
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Configuration
FEDORA_VERSION="${FEDORA_VERSION:-41}"
FEDORA_ISO_URL="https://download.fedoraproject.org/pub/fedora/linux/releases/${FEDORA_VERSION}/Server/x86_64/iso/Fedora-Server-dvd-x86_64-${FEDORA_VERSION}-1.7.iso"
ISO_PATH="/var/lib/libvirt/images/fedora-${FEDORA_VERSION}-server.iso"
SERVER_DISK="/var/lib/libvirt/images/go-dhcpd-server.qcow2"
CLIENT_DISK="/var/lib/libvirt/images/go-dhcpd-client.qcow2"
DISK_SIZE="20G"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

check_requirements() {
    log_info "Checking requirements..."
    
    local missing_deps=()
    
    for cmd in virsh qemu-img virt-install; do
        if ! command -v "$cmd" &> /dev/null; then
            missing_deps+=("$cmd")
        fi
    done
    
    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing required commands: ${missing_deps[*]}"
        log_info "Please install libvirt, qemu, and virt-install"
        log_info "Fedora/RHEL: sudo dnf install libvirt qemu-kvm virt-install"
        log_info "Ubuntu/Debian: sudo apt install libvirt-daemon qemu-kvm virtinst"
        return 1
    fi
    
    # Check if libvirtd is running
    if ! systemctl is-active --quiet libvirtd; then
        log_error "libvirtd is not running"
        log_info "Start it with: sudo systemctl start libvirtd"
        return 1
    fi
    
    # Check if user is in libvirt group
    if ! groups | grep -q libvirt; then
        log_warning "Current user is not in 'libvirt' group"
        log_warning "Add with: sudo usermod -aG libvirt \$USER (then re-login)"
    fi
    
    log_success "All requirements met"
}

download_fedora_iso() {
    if [ -f "$ISO_PATH" ]; then
        log_info "Fedora ISO already exists: $ISO_PATH"
        return 0
    fi
    
    log_info "Downloading Fedora ${FEDORA_VERSION} Server ISO..."
    log_info "URL: $FEDORA_ISO_URL"
    log_warning "This may take a while (ISO is ~2-3 GB)"
    
    sudo curl -L -o "$ISO_PATH" "$FEDORA_ISO_URL" || {
        log_error "Failed to download Fedora ISO"
        return 1
    }
    
    log_success "Fedora ISO downloaded"
}

create_network() {
    log_info "Creating isolated test network..."
    
    if sudo virsh net-list --all | grep -q go-dhcpd-test-net; then
        log_info "Network already exists, destroying old one..."
        sudo virsh net-destroy go-dhcpd-test-net 2>/dev/null || true
        sudo virsh net-undefine go-dhcpd-test-net
    fi
    
    sudo virsh net-define "${SCRIPT_DIR}/test-network.xml"
    sudo virsh net-start go-dhcpd-test-net
    sudo virsh net-autostart go-dhcpd-test-net
    
    log_success "Test network created and started"
}

create_vm_disk() {
    local disk_path="$1"
    local vm_name="$2"
    
    if [ -f "$disk_path" ]; then
        log_warning "Disk already exists: $disk_path"
        read -p "Delete and recreate? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            sudo rm -f "$disk_path"
        else
            log_info "Using existing disk"
            return 0
        fi
    fi
    
    log_info "Creating $DISK_SIZE disk for $vm_name..."
    sudo qemu-img create -f qcow2 "$disk_path" "$DISK_SIZE"
    log_success "Disk created: $disk_path"
}

install_vm() {
    local vm_name="$1"
    local xml_file="$2"
    local disk_path="$3"
    
    log_info "Installing $vm_name..."
    
    # Check if VM already exists
    if sudo virsh list --all | grep -q "$vm_name"; then
        log_warning "VM $vm_name already exists"
        read -p "Destroy and recreate? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            sudo virsh destroy "$vm_name" 2>/dev/null || true
            sudo virsh undefine "$vm_name" --remove-all-storage 2>/dev/null || true
        else
            log_info "Skipping VM installation"
            return 0
        fi
    fi
    
    # Create disk
    create_vm_disk "$disk_path" "$vm_name"
    
    log_info "Defining VM from XML..."
    sudo virsh define "$xml_file"
    
    log_warning "VM defined but not started - manual installation required"
    log_info "To install $vm_name:"
    log_info "  1. Attach the ISO: sudo virt-manager (or use virsh attach-disk)"
    log_info "  2. Start the VM: sudo virsh start $vm_name"
    log_info "  3. Connect via console: sudo virsh console $vm_name"
    log_info "     Or use VNC: virt-viewer $vm_name"
    log_info "  4. Complete Fedora installation"
    log_info "  5. Run setup script inside VM (see setup-*.sh files)"
}

install_vms() {
    log_info "Installing VMs..."
    
    install_vm "go-dhcpd-server" "${SCRIPT_DIR}/dhcpd-server.xml" "$SERVER_DISK"
    install_vm "go-dhcpd-client" "${SCRIPT_DIR}/dhcpd-client.xml" "$CLIENT_DISK"
    
    log_success "VM definitions created"
}

start_vms() {
    log_info "Starting VMs..."
    
    for vm in go-dhcpd-server go-dhcpd-client; do
        if sudo virsh list --state-running | grep -q "$vm"; then
            log_info "$vm is already running"
        else
            log_info "Starting $vm..."
            sudo virsh start "$vm" || log_error "Failed to start $vm"
        fi
    done
    
    log_success "VMs started"
}

stop_vms() {
    log_info "Stopping VMs..."
    
    for vm in go-dhcpd-server go-dhcpd-client; do
        if sudo virsh list --state-running | grep -q "$vm"; then
            log_info "Stopping $vm..."
            sudo virsh shutdown "$vm"
        else
            log_info "$vm is not running"
        fi
    done
    
    log_success "Shutdown commands sent"
}

destroy_vms() {
    log_info "Destroying VMs..."
    
    read -p "This will delete all VMs and data. Are you sure? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Cancelled"
        return 0
    fi
    
    for vm in go-dhcpd-server go-dhcpd-client; do
        sudo virsh destroy "$vm" 2>/dev/null || true
        sudo virsh undefine "$vm" --remove-all-storage 2>/dev/null || true
    done
    
    # Explicitly remove disk images
    log_info "Removing disk images..."
    if [ -f "$SERVER_DISK" ]; then
        sudo rm -f "$SERVER_DISK"
        log_info "Removed $SERVER_DISK"
    fi
    if [ -f "$CLIENT_DISK" ]; then
        sudo rm -f "$CLIENT_DISK"
        log_info "Removed $CLIENT_DISK"
    fi
    
    sudo virsh net-destroy go-dhcpd-test-net 2>/dev/null || true
    sudo virsh net-undefine go-dhcpd-test-net 2>/dev/null || true
    
    log_success "VMs destroyed and disk images removed"
}

status() {
    log_info "VM Status:"
    sudo virsh list --all | grep -E "go-dhcpd-(server|client)" || log_info "No VMs found"
    
    echo
    log_info "Network Status:"
    sudo virsh net-list --all | grep go-dhcpd-test-net || log_info "Network not found"
}

show_help() {
    cat << EOF
Integration Test VM Provisioner for go-dhcpd

Usage: $0 [COMMAND]

Commands:
    setup           Full setup: download ISO, create network, install VMs
    download        Download Fedora ISO only
    network         Create test network only
    install         Install/define VMs
    start           Start VMs
    stop            Stop VMs gracefully
    destroy         Destroy VMs and network
    status          Show VM and network status
    console-server  Connect to server console
    console-client  Connect to client console
    help            Show this help

Environment Variables:
    FEDORA_VERSION  Fedora version to use (default: 41)

Examples:
    $0 setup        # Initial setup
    $0 start        # Start VMs
    $0 status       # Check status

EOF
}

main() {
    case "${1:-help}" in
        setup)
            check_requirements
            download_fedora_iso
            create_network
            install_vms
            log_success "Setup complete!"
            log_info "Next steps:"
            log_info "  1. Manually install Fedora on both VMs"
            log_info "  2. Run setup scripts inside each VM"
            log_info "  3. Use './provision.sh start' to start the VMs"
            ;;
        download)
            check_requirements
            download_fedora_iso
            ;;
        network)
            check_requirements
            create_network
            ;;
        install)
            check_requirements
            install_vms
            ;;
        start)
            start_vms
            ;;
        stop)
            stop_vms
            ;;
        destroy)
            destroy_vms
            ;;
        status)
            status
            ;;
        console-server)
            sudo virsh console go-dhcpd-server
            ;;
        console-client)
            sudo virsh console go-dhcpd-client
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log_error "Unknown command: $1"
            show_help
            exit 1
            ;;
    esac
}

main "$@"

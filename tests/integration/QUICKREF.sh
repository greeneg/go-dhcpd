#!/bin/bash
# Quick Reference - Integration Test Commands
# ============================================

# INITIAL SETUP
# -------------
./provision.sh setup              # Download ISO, create network, define VMs
# Then manually install Fedora on both VMs using virt-manager or console

# VM MANAGEMENT
# -------------
./provision.sh status             # Check VM and network status
./provision.sh start              # Start both VMs
./provision.sh stop               # Stop both VMs gracefully
./provision.sh destroy            # Delete everything (careful!)
./provision.sh console-server     # Connect to server console
./provision.sh console-client     # Connect to client console

# DEPLOYMENT
# ----------
./deploy-server.sh                # Build and deploy go-dhcpd to server VM

# TESTING
# -------
./run-tests.sh                    # Run automated test suite

# SERVER VM COMMANDS
# ------------------
# (Run these inside the server VM via console or SSH)

sudo systemctl status go-dhcpd    # Check DHCP service status
sudo systemctl restart go-dhcpd   # Restart DHCP service
sudo journalctl -u go-dhcpd -f    # View service logs
sudo tcpdump -i eth0 port 67 or port 68 -v  # Capture DHCP traffic

# Check API endpoints
curl http://192.168.100.10:18467/health
curl http://192.168.100.10:18467/config
curl http://192.168.100.10:18467/leases
curl http://192.168.100.10:18467/metrics

# CLIENT VM COMMANDS
# ------------------
# (Run these inside the client VM via console or SSH)

test-dhcp.sh                      # Test DHCP lease acquisition
capture-dhcp.sh                   # Capture DHCP traffic to file

# Manual DHCP operations
sudo dhclient -r eth0             # Release current lease
sudo dhclient -v eth0             # Request new lease (verbose)
ip addr show eth0                 # Show current IP
ip route                          # Show routing table
cat /etc/resolv.conf              # Show DNS configuration

# HOST COMMANDS
# -------------
# (Run these on your host machine)

# Check VMs
sudo virsh list --all
sudo virsh dominfo go-dhcpd-server
sudo virsh dominfo go-dhcpd-client

# Check network
sudo virsh net-list --all
sudo virsh net-info go-dhcpd-test-net

# Access server API from host
curl http://192.168.100.10:18467/health

# View VM resources
sudo virsh domstats go-dhcpd-server
sudo virsh domstats go-dhcpd-client

# NETWORK INFORMATION
# -------------------
Network:      192.168.100.0/24
Server IP:    192.168.100.10
Dynamic Pool: 192.168.100.100 - 192.168.100.200
Gateway:      192.168.100.1
Domain:       dhcpd.test

# TROUBLESHOOTING
# ---------------
# VMs won't start
sudo systemctl status libvirtd
sudo virsh list --all
sudo journalctl -u libvirtd -f

# DHCP server issues
./provision.sh console-server
sudo systemctl status go-dhcpd
sudo journalctl -u go-dhcpd -n 100
getcap /usr/local/bin/dhcpd

# Client not getting IP
./provision.sh console-client
sudo dhclient -r eth0 && sudo dhclient -v eth0
sudo tcpdump -i eth0 port 67 or port 68 -v

# Network problems
sudo virsh net-destroy go-dhcpd-test-net
sudo virsh net-start go-dhcpd-test-net
ip addr show virbr-dhcpd

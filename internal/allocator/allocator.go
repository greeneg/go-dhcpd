package allocator

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/db"
	"github.com/greeneg/go-dhcpd/internal/logger"
)

// Allocator handles IP address allocation
type Allocator struct {
	config *config.Config
	db     *db.Database
}

// NewAllocator creates a new IP allocator
func NewAllocator(cfg *config.Config, database *db.Database) *Allocator {
	return &Allocator{
		config: cfg,
		db:     database,
	}
}

// AllocateIP allocates an IP address for a MAC address
func (a *Allocator) AllocateIP(macAddr string, requestedIP net.IP) (net.IP, error) {
	// Check if there's a static assignment
	staticIP := a.getStaticIP(macAddr)
	if staticIP != nil {
		return staticIP, nil
	}

	// Check if there's an existing lease
	existingLease, err := a.db.GetLeaseByMAC(macAddr)
	if err != nil {
		return nil, err
	}

	if existingLease != nil && time.Now().Before(existingLease.LeaseEnd) {
		logger.Info(fmt.Sprintf("Reusing existing lease for MAC %s: %s", macAddr, existingLease.IPAddress))
		return net.ParseIP(existingLease.IPAddress), nil
	}

	// Try to allocate the requested IP if provided
	if requestedIP != nil && !requestedIP.IsUnspecified() {
		if a.isIPAvailable(requestedIP, macAddr) {
			if err := a.pingCheck(requestedIP, macAddr); err == nil {
				return requestedIP, nil
			}
		}
	}

	// Find an available IP from dynamic ranges
	for _, subnet := range a.config.Subnets {
		for _, dynamicRange := range subnet.DynamicRanges {
			ip, err := a.findAvailableIPInRange(dynamicRange.StartAddress, dynamicRange.EndAddress, macAddr)
			if err != nil {
				continue
			}
			if ip != nil {
				return ip, nil
			}
		}
	}

	return nil, fmt.Errorf("no available IP addresses")
}

// getStaticIP checks if there's a static assignment for the MAC address
func (a *Allocator) getStaticIP(macAddr string) net.IP {
	macAddr = strings.ToLower(macAddr)
	for _, static := range a.config.Static {
		if strings.ToLower(static.MACAddress) == macAddr {
			// Try to parse as IP first
			ip := net.ParseIP(static.IPAddress)
			if ip != nil {
				return ip
			}

			// If not an IP, try to resolve as hostname
			ips, err := net.LookupIP(static.IPAddress)
			if err == nil && len(ips) > 0 {
				return ips[0]
			}
		}
	}
	return nil
}

// isIPAvailable checks if an IP is available for allocation
func (a *Allocator) isIPAvailable(ip net.IP, macAddr string) bool {
	// Check if IP is in deny list
	denied, err := a.db.IsDenyAddress(ip.String())
	if err != nil || denied {
		return false
	}

	// Check if IP is already leased to someone else
	lease, err := a.db.GetLeaseByIP(ip.String())
	if err != nil {
		return false
	}

	if lease == nil {
		return true
	}

	// If lease exists, check if it's for the same MAC or expired
	if lease.MACAddress == macAddr {
		return true
	}

	return time.Now().After(lease.LeaseEnd)
}

// findAvailableIPInRange finds an available IP in the range between start and end addresses
func (a *Allocator) findAvailableIPInRange(startAddr, endAddr string, macAddr string) (net.IP, error) {
	// Parse start and end IP addresses
	startIP := net.ParseIP(startAddr)
	endIP := net.ParseIP(endAddr)

	if startIP == nil || endIP == nil {
		return nil, fmt.Errorf("invalid IP addresses in range: %s - %s", startAddr, endAddr)
	}

	// Convert to IPv4
	startIPv4 := startIP.To4()
	endIPv4 := endIP.To4()

	if startIPv4 == nil || endIPv4 == nil {
		return nil, fmt.Errorf("invalid IPv4 addresses in range: %s - %s", startAddr, endAddr)
	}

	// Convert start and end IPs to uint32 for arithmetic
	startInt := uint32(startIPv4[0])<<24 | uint32(startIPv4[1])<<16 | uint32(startIPv4[2])<<8 | uint32(startIPv4[3])
	endInt := uint32(endIPv4[0])<<24 | uint32(endIPv4[1])<<16 | uint32(endIPv4[2])<<8 | uint32(endIPv4[3])

	// Ensure start is not greater than end
	if startInt > endInt {
		return nil, fmt.Errorf("start address %s is greater than end address %s", startAddr, endAddr)
	}

	// Iterate through the range
	currentInt := startInt
	for currentInt <= endInt {
		// Convert current int back to IP
		ip := net.IPv4(byte(currentInt>>24), byte(currentInt>>16), byte(currentInt>>8), byte(currentInt))

		if a.isIPAvailable(ip, macAddr) {
			// Perform ping check
			if err := a.pingCheck(ip, macAddr); err == nil {
				return ip, nil
			}
		}

		currentInt++
	}

	return nil, fmt.Errorf("no available IP in range %s - %s", startAddr, endAddr)
}

// pingCheck performs a ping check to ensure the IP is not in use
func (a *Allocator) pingCheck(ip net.IP, _ string) error {
	timeout := time.Duration(a.config.Global.PingTimeout) * time.Millisecond
	retries := a.config.Global.PingRetries

	for i := 0; i < retries; i++ {
		// Use system ping command
		cmd := exec.Command("ping", "-c", "1", "-W", fmt.Sprintf("%d", timeout/time.Millisecond/1000+1), ip.String())
		err := cmd.Run()

		if err != nil {
			// Ping failed, IP is available
			return nil
		}

		// Ping succeeded, IP may be in use - continue retrying
		if i < retries-1 {
			time.Sleep(100 * time.Millisecond) // Brief pause between retries
		}
	}

	// All pings succeeded, IP is definitely in use
	logger.Notice(fmt.Sprintf("IP %s responded to ping, marking as martian", ip.String()))

	// Try to get MAC address using ARP
	detectedMAC := a.getMACFromARP(ip)

	// Add to deny list
	deny := &db.DenyAddress{
		IPAddress:  ip.String(),
		MACAddress: detectedMAC,
		Reason:     fmt.Sprintf("Responded to ping check, possible squatter. Detected MAC: %s", detectedMAC),
		DetectedAt: time.Now(),
	}

	if err := a.db.AddDenyAddress(deny); err != nil {
		logger.Error(fmt.Sprintf("Failed to add deny address: %v", err))
	}

	return fmt.Errorf("IP %s is in use", ip.String())
}

// getMACFromARP attempts to get MAC address from ARP table
func (a *Allocator) getMACFromARP(ip net.IP) string {
	// Try to get MAC from ARP cache
	cmd := exec.Command("arp", "-n", ip.String())
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	// Parse ARP output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, ip.String()) {
			fields := strings.Fields(line)
			for _, field := range fields {
				if strings.Count(field, ":") == 5 {
					return field
				}
			}
		}
	}

	return "unknown"
}

// CreateLease creates a new lease in the database
func (a *Allocator) CreateLease(macAddr string, ip net.IP, hostname string, isStatic bool) error {
	leaseTime := time.Duration(a.config.Global.LeaseTime) * time.Second
	now := time.Now()

	lease := &db.Lease{
		MACAddress: macAddr,
		IPAddress:  ip.String(),
		Hostname:   hostname,
		LeaseStart: now,
		LeaseEnd:   now.Add(leaseTime),
		State:      "active",
		IsStatic:   isStatic,
	}

	// Check if lease already exists
	existingLease, err := a.db.GetLeaseByIP(ip.String())
	if err != nil {
		return err
	}

	if existingLease != nil {
		// Update existing lease
		lease.ID = existingLease.ID
		return a.db.UpdateLease(lease)
	}

	// Create new lease
	return a.db.AddLease(lease)
}

// ReleaseLease releases a lease
func (a *Allocator) ReleaseLease(macAddr string) error {
	lease, err := a.db.GetLeaseByMAC(macAddr)
	if err != nil {
		return err
	}

	if lease == nil {
		return fmt.Errorf("no lease found for MAC %s", macAddr)
	}

	// Mark as expired
	lease.State = "expired"
	lease.LeaseEnd = time.Now()
	return a.db.UpdateLease(lease)
}

// Helper functions

// nextIP returns the next IP address
func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)
	for j := len(next) - 1; j >= 0; j-- {
		next[j]++
		if next[j] > 0 {
			break
		}
	}
	return next
}

// broadcast returns the broadcast address for a network
func broadcast(n *net.IPNet) net.IP {
	ip := make(net.IP, len(n.IP))
	for i := range n.IP {
		ip[i] = n.IP[i] | ^n.Mask[i]
	}
	return ip
}

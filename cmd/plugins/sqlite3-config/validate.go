package main

import (
	"fmt"
	"net"

	"github.com/greeneg/go-dhcpd/internal/config"
)

// validateSubnet performs basic sanity checks before a subnet is persisted,
// so obviously malformed data can't reach the database.
func validateSubnet(s *config.SubnetConfig) error {
	if s.Network == "" {
		return fmt.Errorf("subnet network is required")
	}
	if net.ParseIP(s.Network) == nil {
		return fmt.Errorf("invalid network address: %q", s.Network)
	}
	if s.Netmask == "" {
		return fmt.Errorf("subnet netmask is required")
	}
	if net.ParseIP(s.Netmask) == nil {
		return fmt.Errorf("invalid netmask: %q", s.Netmask)
	}
	for _, r := range s.DynamicRanges {
		if net.ParseIP(r.StartAddress) == nil {
			return fmt.Errorf("invalid dynamic range start address: %q", r.StartAddress)
		}
		if net.ParseIP(r.EndAddress) == nil {
			return fmt.Errorf("invalid dynamic range end address: %q", r.EndAddress)
		}
	}
	return nil
}

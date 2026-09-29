package main

import (
	"fmt"
	"net"

	"github.com/greeneg/go-dhcpd/internal/config"
)

// isIPv4 reports whether s parses as an IPv4 address. IPv6 is rejected: the
// daemon's subnet matching (server.go's netmask handling) and DHCP option
// encoding (packet.go) both convert addresses via net.IP.To4(), which
// silently drops IPv6 values instead of erroring. Without this check a
// plugin could persist a subnet that validates successfully here but is
// then silently unusable (or serves malformed options) at runtime.
func isIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

// validateIPv4List reports an error naming label if any entry in values is
// not a valid IPv4 address.
func validateIPv4List(label string, values []string) error {
	for _, v := range values {
		if !isIPv4(v) {
			return fmt.Errorf("invalid %s address: %q (only IPv4 is supported)", label, v)
		}
	}
	return nil
}

// validateSubnet performs basic sanity checks before a subnet is persisted,
// so obviously malformed data can't reach the database.
func validateSubnet(s *config.SubnetConfig) error {
	if s.Network == "" {
		return fmt.Errorf("subnet network is required")
	}
	if !isIPv4(s.Network) {
		return fmt.Errorf("invalid network address: %q (only IPv4 is supported)", s.Network)
	}
	if s.Netmask == "" {
		return fmt.Errorf("subnet netmask is required")
	}
	if !isIPv4(s.Netmask) {
		return fmt.Errorf("invalid netmask: %q (only IPv4 is supported)", s.Netmask)
	}
	for _, r := range s.DynamicRanges {
		if !isIPv4(r.StartAddress) {
			return fmt.Errorf("invalid dynamic range start address: %q (only IPv4 is supported)", r.StartAddress)
		}
		if !isIPv4(r.EndAddress) {
			return fmt.Errorf("invalid dynamic range end address: %q (only IPv4 is supported)", r.EndAddress)
		}
	}

	for _, list := range []struct {
		label  string
		values []string
	}{
		{"domain_name_servers", s.DomainNameServers},
		{"ntp_servers", s.NTPServers},
		{"time_servers", s.TimeServers},
		{"smtp_servers", s.SMTPServers},
		{"lpr_servers", s.LPRServers},
		{"netbios_name_servers", s.NetBIOSNameServers},
		{"netbios_dd_servers", s.NetBIOSDDServers},
		{"routers", s.Routers},
	} {
		if err := validateIPv4List(list.label, list.values); err != nil {
			return err
		}
	}

	return nil
}

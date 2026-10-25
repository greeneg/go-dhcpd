package sqlite3store

import (
	"fmt"
	"net"

	"github.com/greeneg/go-dhcpd/internal/config"
)

// isIPv4 reports whether s parses as an IPv4 address. IPv6 is rejected: the
// daemon's subnet matching (server.go's netmask handling) and DHCP option
// encoding (packet.go) both convert addresses via net.IP.To4(), which
// silently drops IPv6 values instead of erroring. Without this check a
// caller could persist a subnet that validates successfully here but is
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

// ValidateSubnet performs basic sanity checks before a subnet is persisted,
// so obviously malformed data can't reach the database.
func ValidateSubnet(s *config.SubnetConfig) error {
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
	if s.NextServer != "" && !isIPv4(s.NextServer) {
		return fmt.Errorf("invalid next_server address: %q (only IPv4 is supported)", s.NextServer)
	}
	// BootP's sname/file packet fields are fixed-size, null-terminated
	// byte arrays (RFC 951 §3), so values must fit with room for the
	// terminator.
	if len(s.ServerHostname) > 63 {
		return fmt.Errorf("server_hostname must be 63 bytes or fewer, got %d", len(s.ServerHostname))
	}
	if len(s.BootFilename) > 127 {
		return fmt.Errorf("boot_filename must be 127 bytes or fewer, got %d", len(s.BootFilename))
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

// ValidateStaticHost performs basic sanity checks before a static host is
// persisted. ip_address intentionally isn't restricted to IPv4 literals
// here: config.StaticHost documents it as "Can be IP or FQDN", and the
// allocator resolves hostnames via DNS at allocation time.
func ValidateStaticHost(h *config.StaticHost) error {
	if h.MACAddress == "" {
		return fmt.Errorf("static host mac_address is required")
	}
	// net.ParseMAC also accepts 8-byte (EUI-64) and 20-byte (InfiniBand)
	// hardware addresses; reject those since this daemon only ever
	// matches six-byte Ethernet client addresses (see StaticHost's doc
	// comment), and canonicalize the accepted value's formatting.
	hw, err := net.ParseMAC(h.MACAddress)
	if err != nil || len(hw) != 6 {
		return fmt.Errorf("invalid mac_address: %q (must be a 6-byte Ethernet address, e.g. aa:bb:cc:dd:ee:ff)", h.MACAddress)
	}
	h.MACAddress = hw.String()
	if h.IPAddress == "" {
		return fmt.Errorf("static host ip_address is required")
	}
	return nil
}

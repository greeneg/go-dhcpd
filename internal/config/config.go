package config

import (
	"encoding/json"
	"os"

	"github.com/yosuke-furukawa/json5/encoding/json5"
)

// Config represents the complete DHCP server configuration
type Config struct {
	Global  GlobalConfig   `json:"global"`
	Subnets []SubnetConfig `json:"subnets"`
	Static  []StaticHost   `json:"static"`
}

// GlobalConfig represents global DHCP settings
type GlobalConfig struct {
	LeaseTime       int    `json:"lease_time"`        // in seconds
	NetBIOSNodeType int    `json:"netbios_node_type"` // 1=B-node, 2=P-node, 4=M-node, 8=H-node
	PingTimeout     int    `json:"ping_timeout"`      // in milliseconds
	PingRetries     int    `json:"ping_retries"`
	DatabasePath    string `json:"database_path"`
	ListenAddress   string `json:"listen_address"`   // IP address to bind to (deprecated, use ListenInterface)
	ListenInterface string `json:"listen_interface"` // Network interface to bind to (e.g., eth0, ens33)
	APIPort         int    `json:"api_port"`         // default 18467
}

// DynamicRange represents a dynamic IP allocation range
type DynamicRange struct {
	StartAddress string `json:"start_address"`
	EndAddress   string `json:"end_address"`
}

// SubnetConfig represents a subnet configuration
type SubnetConfig struct {
	Network            string         `json:"network"`
	Netmask            string         `json:"netmask"`
	DynamicRanges      []DynamicRange `json:"dynamic_ranges"`
	EnableBootP        bool           `json:"enable_bootp"`
	NetBIOSNodeType    *int           `json:"netbios_node_type"` // Override global if set
	DomainNameServers  []string       `json:"domain_name_servers"`
	DomainName         string         `json:"domain_name"`
	DomainSearch       []string       `json:"domain_search"`
	InterfaceMTU       *int           `json:"interface_mtu"`
	NTPServers         []string       `json:"ntp_servers"`
	TimeServers        []string       `json:"time_servers"`
	SMTPServers        []string       `json:"smtp_servers"`
	LPRServers         []string       `json:"lpr_servers"`
	NetBIOSNameServers []string       `json:"netbios_name_servers"`
	NetBIOSDDServers   []string       `json:"netbios_dd_servers"`
	Routers            []string       `json:"routers"`
}

// StaticHost represents a static DHCP assignment
type StaticHost struct {
	MACAddress string `json:"mac_address"` // Format: aa:bb:cc:dd:ee:ff
	IPAddress  string `json:"ip_address"`  // Can be IP or FQDN
	Hostname   string `json:"hostname"`
}

// LoadConfig loads configuration from a JSON5 file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json5.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Set defaults
	if config.Global.LeaseTime == 0 {
		config.Global.LeaseTime = 86400 // 24 hours
	}
	if config.Global.NetBIOSNodeType == 0 {
		config.Global.NetBIOSNodeType = 8 // H-node
	}
	if config.Global.PingTimeout == 0 {
		config.Global.PingTimeout = 1000 // 1 second
	}
	if config.Global.PingRetries == 0 {
		config.Global.PingRetries = 2
	}
	if config.Global.DatabasePath == "" {
		config.Global.DatabasePath = "/var/lib/go-dhcpd/dhcpd.db"
	}
	if config.Global.ListenAddress == "" {
		config.Global.ListenAddress = "0.0.0.0"
	}
	if config.Global.APIPort == 0 {
		config.Global.APIPort = 18467
	}

	return &config, nil
}

// SaveConfig saves configuration to a JSON file (pretty-printed)
func SaveConfig(path string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

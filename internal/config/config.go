package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/yosuke-furukawa/json5/encoding/json5"
)

// DefaultPluginDir is the directory searched for the built-in config-provider
// plugin when plugins.config_provider.path is not set in config.json5. It is
// set at build time via -ldflags (see Makefile's INSTALL_PREFIX) so it always
// matches the prefix the binary was actually installed under; the literal
// below is only a fallback for builds that don't pass -ldflags (e.g. `go run`).
var DefaultPluginDir = "/usr/local/lib/go-dhcpd/plugins"

// Config represents the complete DHCP server configuration
type Config struct {
	Global  GlobalConfig   `json:"global"`
	Auth    AuthConfig     `json:"auth"`
	Plugins PluginsConfig  `json:"plugins"`
	Subnets []SubnetConfig `json:"subnets"`
	Static  []StaticHost   `json:"static"`
}

// AuthConfig controls who may access authenticated API endpoints. Requests
// are authenticated via HTTP Basic Auth against the host's local PAM stack;
// this only controls authorization (who is allowed through) once a request's
// credentials have been verified. root is always authorized.
type AuthConfig struct {
	AllowedUsers  []string `json:"allowed_users"`
	AllowedGroups []string `json:"allowed_groups"`
}

// PluginConfig describes a single external plugin binary and the settings
// passed to it on every invocation.
type PluginConfig struct {
	Name     string         `json:"name"`
	Path     string         `json:"path"`
	Settings map[string]any `json:"settings"`
}

// PluginsConfig lists the plugins the daemon loads at startup.
type PluginsConfig struct {
	// ConfigProvider is the plugin responsible for supplying (and, if it
	// supports write operations, managing) subnets and static leases.
	ConfigProvider PluginConfig `json:"config_provider"`
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

	if config.Plugins.ConfigProvider.Name == "" {
		config.Plugins.ConfigProvider.Name = "file"
	}
	if config.Plugins.ConfigProvider.Path == "" {
		config.Plugins.ConfigProvider.Path = filepath.Join(DefaultPluginDir, "file-config.plugin")
	}
	if config.Plugins.ConfigProvider.Settings == nil {
		config.Plugins.ConfigProvider.Settings = map[string]any{}
	}
	if _, ok := config.Plugins.ConfigProvider.Settings["config_path"]; !ok {
		absPath, err := filepath.Abs(path)
		if err != nil {
			absPath = path
		}
		config.Plugins.ConfigProvider.Settings["config_path"] = absPath
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

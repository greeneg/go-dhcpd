// Command file-config is the built-in go-dhcpd configuration-provider
// plugin. It reads subnet and static-host definitions from the daemon's
// config.json5 file (or another file specified via the "config_path"
// setting) and reports them on stdout. It is intentionally read-only: it
// exists to preserve today's behavior of managing subnets/static leases in
// config.json5 while the plugin architecture is adopted, and it rejects any
// write action so callers must use a plugin that supports persistence (e.g.
// a future database-backed plugin) to mutate this data.
package main

import (
	"fmt"
	"os"

	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/pluginapi"
)

// pluginName must match this plugin's installed binary name (without the
// ".plugin" suffix), since config.LoadConfig derives the default plugin
// path from plugins.config_provider.name when "path" is not set.
const pluginName = "file-config"
const pluginVersion = "1.0.0"

func main() {
	req, err := pluginapi.ReadRequest(os.Stdin)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("failed to read request: %v", err))
		os.Exit(1)
	}

	switch req.Action {
	case pluginapi.ActionCapabilities:
		handleCapabilities()
	case pluginapi.ActionGetSubnets:
		handleGetSubnets(req)
	case pluginapi.ActionGetStaticHosts:
		handleGetStaticHosts(req)
	case pluginapi.ActionAddSubnet,
		pluginapi.ActionUpdateSubnet,
		pluginapi.ActionDeleteSubnet,
		pluginapi.ActionAddStaticHost,
		pluginapi.ActionUpdateStaticHost,
		pluginapi.ActionDeleteStaticHost:
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("the %q plugin is read-only and does not support action %q", pluginName, req.Action))
	default:
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("unknown action: %q", req.Action))
	}
}

func handleCapabilities() {
	caps := pluginapi.Capabilities{
		Name:     pluginName,
		Version:  pluginVersion,
		Writable: false,
	}
	_ = pluginapi.WriteSuccess(os.Stdout, caps)
}

func handleGetSubnets(req *pluginapi.Request) {
	cfg, err := loadConfig(req)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, err.Error())
		return
	}
	_ = pluginapi.WriteSuccess(os.Stdout, cfg.Subnets)
}

func handleGetStaticHosts(req *pluginapi.Request) {
	cfg, err := loadConfig(req)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, err.Error())
		return
	}
	_ = pluginapi.WriteSuccess(os.Stdout, cfg.Static)
}

// loadConfig resolves the "config_path" setting and parses it as a go-dhcpd config.json5 file.
func loadConfig(req *pluginapi.Request) (*config.Config, error) {
	path, ok := req.Settings["config_path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("the %q plugin requires a non-empty \"config_path\" setting", pluginName)
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %q: %w", path, err)
	}
	return cfg, nil
}

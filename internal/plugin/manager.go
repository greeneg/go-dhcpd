// Package plugin manages the external configuration-provider plugin used to
// supply and (optionally) mutate subnet and static lease data.
//
// Plugins are independent executables invoked as subprocesses. A single
// JSON request is written to the plugin's stdin, and a single JSON response
// is read from its stdout, per the internal/pluginapi protocol.
package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/pluginapi"
)

// callTimeout bounds how long a single plugin invocation may run.
const callTimeout = 15 * time.Second

// Manager executes the configured config-provider plugin and exposes typed
// helpers for the subnet and static-host operations the daemon needs.
type Manager struct {
	name     string
	path     string
	settings map[string]any
	caps     pluginapi.Capabilities
}

// NewManager validates and loads the config-provider plugin described by cfg,
// then queries its capabilities.
func NewManager(cfg config.PluginConfig) (*Manager, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("config_provider plugin name is required")
	}
	if err := validatePluginPath(cfg.Path); err != nil {
		return nil, fmt.Errorf("config_provider plugin %q: %w", cfg.Name, err)
	}

	m := &Manager{
		name:     cfg.Name,
		path:     cfg.Path,
		settings: cfg.Settings,
	}

	caps, err := m.fetchCapabilities()
	if err != nil {
		return nil, fmt.Errorf("failed to query capabilities of plugin %q: %w", cfg.Name, err)
	}
	m.caps = caps

	return m, nil
}

// validatePluginPath ensures the plugin binary is an absolute path to an
// existing, executable, non-world-writable regular file before it is ever
// invoked as a subprocess.
func validatePluginPath(path string) error {
	if path == "" {
		return fmt.Errorf("plugin path is required")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("plugin path %q must be absolute", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("plugin path %q is not accessible: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("plugin path %q is not a regular file", path)
	}
	if info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("plugin path %q is not executable", path)
	}
	if info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("plugin path %q must not be group- or world-writable", path)
	}

	return nil
}

// Capabilities returns the previously-fetched capabilities of the loaded plugin.
func (m *Manager) Capabilities() pluginapi.Capabilities {
	return m.caps
}

// Name returns the configured plugin name.
func (m *Manager) Name() string {
	return m.name
}

func (m *Manager) fetchCapabilities() (pluginapi.Capabilities, error) {
	resp, err := m.call(pluginapi.ActionCapabilities, nil)
	if err != nil {
		// Fail safe: if we can't confirm the plugin supports writes, treat it as read-only.
		return pluginapi.Capabilities{Name: m.name, Writable: false}, err
	}

	var caps pluginapi.Capabilities
	if err := json.Unmarshal(resp.Data, &caps); err != nil {
		// Fail safe: an unparsable response must not be treated as an implicit grant to write.
		return pluginapi.Capabilities{Name: m.name, Writable: false}, fmt.Errorf("invalid capabilities response: %w", err)
	}
	return caps, nil
}

// GetSubnets asks the plugin for the current list of subnets.
func (m *Manager) GetSubnets() ([]config.SubnetConfig, error) {
	resp, err := m.call(pluginapi.ActionGetSubnets, nil)
	if err != nil {
		return nil, err
	}
	var subnets []config.SubnetConfig
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &subnets); err != nil {
			return nil, fmt.Errorf("invalid subnets response: %w", err)
		}
	}
	return subnets, nil
}

// GetStaticHosts asks the plugin for the current list of static hosts.
func (m *Manager) GetStaticHosts() ([]config.StaticHost, error) {
	resp, err := m.call(pluginapi.ActionGetStaticHosts, nil)
	if err != nil {
		return nil, err
	}
	var hosts []config.StaticHost
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &hosts); err != nil {
			return nil, fmt.Errorf("invalid static hosts response: %w", err)
		}
	}
	return hosts, nil
}

// AddSubnet asks the plugin to add a new subnet. Returns an error if the
// plugin is read-only.
func (m *Manager) AddSubnet(subnet config.SubnetConfig) error {
	return m.writeSubnet(pluginapi.ActionAddSubnet, subnet)
}

// UpdateSubnet asks the plugin to replace an existing subnet.
func (m *Manager) UpdateSubnet(subnet config.SubnetConfig) error {
	return m.writeSubnet(pluginapi.ActionUpdateSubnet, subnet)
}

// DeleteSubnet asks the plugin to remove the subnet identified by network.
func (m *Manager) DeleteSubnet(network string) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]string{"network": network})
	if err != nil {
		return err
	}
	_, err = m.call(pluginapi.ActionDeleteSubnet, data)
	return err
}

func (m *Manager) writeSubnet(action pluginapi.Action, subnet config.SubnetConfig) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	data, err := json.Marshal(subnet)
	if err != nil {
		return err
	}
	_, err = m.call(action, data)
	return err
}

// AddStaticHost asks the plugin to add a new static host.
func (m *Manager) AddStaticHost(host config.StaticHost) error {
	return m.writeStaticHost(pluginapi.ActionAddStaticHost, host)
}

// UpdateStaticHost asks the plugin to replace an existing static host.
func (m *Manager) UpdateStaticHost(host config.StaticHost) error {
	return m.writeStaticHost(pluginapi.ActionUpdateStaticHost, host)
}

// DeleteStaticHost asks the plugin to remove the static host identified by macAddress.
func (m *Manager) DeleteStaticHost(macAddress string) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]string{"mac_address": macAddress})
	if err != nil {
		return err
	}
	_, err = m.call(pluginapi.ActionDeleteStaticHost, data)
	return err
}

func (m *Manager) writeStaticHost(action pluginapi.Action, host config.StaticHost) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	data, err := json.Marshal(host)
	if err != nil {
		return err
	}
	_, err = m.call(action, data)
	return err
}

// requireWritable rejects write operations client-side unless the plugin has
// explicitly advertised write support; the zero value (Writable == false)
// denies writes, so a plugin must opt in rather than opt out. The plugin
// itself is still expected to enforce this independently.
func (m *Manager) requireWritable() error {
	if !m.caps.Writable {
		return fmt.Errorf("plugin %q is read-only and does not support write operations", m.name)
	}
	return nil
}

// call invokes the plugin binary once with the given action/data and returns
// its decoded response, translating a plugin-reported error into a Go error.
func (m *Manager) call(action pluginapi.Action, data json.RawMessage) (*pluginapi.Response, error) {
	req := &pluginapi.Request{
		Version:  pluginapi.ProtocolVersion,
		Action:   action,
		Settings: m.settings,
		Data:     data,
	}

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, m.path)

	var stdin bytes.Buffer
	if err := pluginapi.WriteRequest(&stdin, req); err != nil {
		return nil, fmt.Errorf("failed to encode plugin request: %w", err)
	}
	cmd.Stdin = &stdin

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("plugin %q execution failed: %w (stderr: %s)", m.name, err, stderr.String())
	}

	resp, err := pluginapi.ReadResponse(&stdout)
	if err != nil {
		return nil, fmt.Errorf("plugin %q returned invalid response: %w", m.name, err)
	}

	if resp.Status != pluginapi.StatusSuccess {
		return nil, fmt.Errorf("plugin %q reported an error: %s", m.name, resp.Message)
	}

	return resp, nil
}

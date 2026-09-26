// Package pluginapi defines the stdin/stdout JSON protocol shared between
// the go-dhcpd daemon and its external configuration-provider plugins.
//
// A plugin is an independent executable that is invoked as a subprocess for
// each request. The daemon writes a single JSON-encoded Request to the
// plugin's stdin and closes it, then reads a single JSON-encoded Response
// from the plugin's stdout.
package pluginapi

import (
	"encoding/json"
	"fmt"
	"io"
)

// ProtocolVersion is the current version of the plugin request/response protocol.
const ProtocolVersion = 1

// Action identifies the operation a plugin request is asking for.
type Action string

const (
	// ActionCapabilities asks the plugin to describe itself, including
	// whether it supports write operations.
	ActionCapabilities Action = "capabilities"

	// ActionGetSubnets asks the plugin for the current list of subnets.
	ActionGetSubnets Action = "get_subnets"
	// ActionGetStaticHosts asks the plugin for the current list of static hosts.
	ActionGetStaticHosts Action = "get_static_hosts"

	// ActionAddSubnet asks the plugin to add a new subnet.
	ActionAddSubnet Action = "add_subnet"
	// ActionUpdateSubnet asks the plugin to replace an existing subnet.
	ActionUpdateSubnet Action = "update_subnet"
	// ActionDeleteSubnet asks the plugin to remove a subnet.
	ActionDeleteSubnet Action = "delete_subnet"

	// ActionAddStaticHost asks the plugin to add a new static host.
	ActionAddStaticHost Action = "add_static_host"
	// ActionUpdateStaticHost asks the plugin to replace an existing static host.
	ActionUpdateStaticHost Action = "update_static_host"
	// ActionDeleteStaticHost asks the plugin to remove a static host.
	ActionDeleteStaticHost Action = "delete_static_host"
)

// StatusSuccess and StatusError are the two values a Response.Status may hold.
const (
	StatusSuccess = "success"
	StatusError   = "error"
)

// Request is sent by the daemon to a plugin on stdin.
type Request struct {
	Version  int             `json:"version"`
	Action   Action          `json:"action"`
	Settings map[string]any  `json:"settings,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// Response is returned by a plugin on stdout.
type Response struct {
	Status  string          `json:"status"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Capabilities describes what a plugin supports. Plugins must answer
// ActionCapabilities with this structure. Writable defaults to false (the
// safe, read-only choice) for any plugin that omits it, so a plugin must
// explicitly opt in to receiving write actions.
type Capabilities struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Writable bool   `json:"writable"`
}

// ReadRequest decodes a single Request from r.
func ReadRequest(r io.Reader) (*Request, error) {
	var req Request
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return nil, fmt.Errorf("failed to decode plugin request: %w", err)
	}
	return &req, nil
}

// WriteRequest encodes req to w.
func WriteRequest(w io.Writer, req *Request) error {
	return json.NewEncoder(w).Encode(req)
}

// WriteSuccess encodes a successful Response wrapping data to w.
func WriteSuccess(w io.Writer, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal plugin response data: %w", err)
	}
	return json.NewEncoder(w).Encode(Response{Status: StatusSuccess, Data: raw})
}

// WriteError encodes a failed Response carrying message to w.
func WriteError(w io.Writer, message string) error {
	return json.NewEncoder(w).Encode(Response{Status: StatusError, Message: message})
}

// ReadResponse decodes a single Response from r.
func ReadResponse(r io.Reader) (*Response, error) {
	var resp Response
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode plugin response: %w", err)
	}
	return &resp, nil
}

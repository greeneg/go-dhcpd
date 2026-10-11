// Command sqlite3-config is a config-provider plugin that stores go-dhcpd
// subnet definitions in a SQLite3 database instead of config.json5. Static
// lease storage is not implemented yet: get_static_hosts reports an empty
// list so the daemon can still start, and static-host write actions are
// rejected until that support is added.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/pluginapi"
)

const pluginName = "sqlite3-config"
const pluginVersion = "0.1.0"

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
	case pluginapi.ActionAddSubnet:
		handleAddSubnet(req)
	case pluginapi.ActionUpdateSubnet:
		handleUpdateSubnet(req)
	case pluginapi.ActionDeleteSubnet:
		handleDeleteSubnet(req)
	case pluginapi.ActionGetStaticHosts:
		// Not implemented yet; report none so the daemon (which requires
		// both subnets and static hosts from the active plugin) can start.
		_ = pluginapi.WriteSuccess(os.Stdout, []config.StaticHost{})
	case pluginapi.ActionAddStaticHost, pluginapi.ActionUpdateStaticHost, pluginapi.ActionDeleteStaticHost:
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("the %q plugin does not support static host storage yet", pluginName))
	default:
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("unknown action: %q", req.Action))
	}
}

func handleCapabilities() {
	caps := pluginapi.Capabilities{
		Name:     pluginName,
		Version:  pluginVersion,
		Writable: true,
	}
	_ = pluginapi.WriteSuccess(os.Stdout, caps)
}

func handleGetSubnets(req *pluginapi.Request) {
	db, err := openDB(req)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, err.Error())
		return
	}
	defer db.Close()

	subnets, err := getSubnets(db)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("failed to read subnets: %v", err))
		return
	}
	_ = pluginapi.WriteSuccess(os.Stdout, subnets)
}

func handleAddSubnet(req *pluginapi.Request) {
	subnet, err := decodeSubnet(req)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, err.Error())
		return
	}

	db, err := openDB(req)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, err.Error())
		return
	}
	defer db.Close()

	if err := insertSubnet(db, subnet); err != nil {
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("failed to add subnet: %v", err))
		return
	}
	_ = pluginapi.WriteSuccess(os.Stdout, subnet)
}

func handleUpdateSubnet(req *pluginapi.Request) {
	subnet, err := decodeSubnet(req)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, err.Error())
		return
	}

	db, err := openDB(req)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, err.Error())
		return
	}
	defer db.Close()

	if err := updateSubnet(db, subnet); err != nil {
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("failed to update subnet: %v", err))
		return
	}
	_ = pluginapi.WriteSuccess(os.Stdout, subnet)
}

func handleDeleteSubnet(req *pluginapi.Request) {
	var payload struct {
		Network string `json:"network"`
	}
	if err := json.Unmarshal(req.Data, &payload); err != nil || payload.Network == "" {
		_ = pluginapi.WriteError(os.Stdout, "delete_subnet requires a non-empty \"network\" field")
		return
	}

	db, err := openDB(req)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, err.Error())
		return
	}
	defer db.Close()

	if err := deleteSubnet(db, payload.Network); err != nil {
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("failed to delete subnet: %v", err))
		return
	}
	_ = pluginapi.WriteSuccess(os.Stdout, map[string]string{"network": payload.Network})
}

// decodeSubnet unmarshals and validates a SubnetConfig from the request payload.
func decodeSubnet(req *pluginapi.Request) (*config.SubnetConfig, error) {
	var subnet config.SubnetConfig
	if err := json.Unmarshal(req.Data, &subnet); err != nil {
		return nil, fmt.Errorf("invalid subnet payload: %w", err)
	}
	if err := validateSubnet(&subnet); err != nil {
		return nil, err
	}
	return &subnet, nil
}

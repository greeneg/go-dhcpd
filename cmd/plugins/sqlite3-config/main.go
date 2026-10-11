// Command sqlite3-config is a config-provider plugin that stores go-dhcpd
// subnet and static-lease definitions in a SQLite3 database instead of
// config.json5.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/pluginapi"
)

const pluginName = "sqlite3-config"
const pluginVersion = "0.9.0"

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
		handleGetStaticHosts(req)
	case pluginapi.ActionAddStaticHost:
		handleAddStaticHost(req)
	case pluginapi.ActionUpdateStaticHost:
		handleUpdateStaticHost(req)
	case pluginapi.ActionDeleteStaticHost:
		handleDeleteStaticHost(req)
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

func handleGetStaticHosts(req *pluginapi.Request) {
	db, err := openDB(req)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, err.Error())
		return
	}
	defer db.Close()

	hosts, err := getStaticHosts(db)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("failed to read static hosts: %v", err))
		return
	}
	_ = pluginapi.WriteSuccess(os.Stdout, hosts)
}

func handleAddStaticHost(req *pluginapi.Request) {
	host, err := decodeStaticHost(req)
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

	if err := insertStaticHost(db, host); err != nil {
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("failed to add static host: %v", err))
		return
	}
	_ = pluginapi.WriteSuccess(os.Stdout, host)
}

func handleUpdateStaticHost(req *pluginapi.Request) {
	host, err := decodeStaticHost(req)
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

	if err := updateStaticHost(db, host); err != nil {
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("failed to update static host: %v", err))
		return
	}
	_ = pluginapi.WriteSuccess(os.Stdout, host)
}

func handleDeleteStaticHost(req *pluginapi.Request) {
	var payload struct {
		MACAddress string `json:"mac_address"`
	}
	if err := json.Unmarshal(req.Data, &payload); err != nil || payload.MACAddress == "" {
		_ = pluginapi.WriteError(os.Stdout, "delete_static_host requires a non-empty \"mac_address\" field")
		return
	}
	payload.MACAddress = config.NormalizeMAC(payload.MACAddress)

	db, err := openDB(req)
	if err != nil {
		_ = pluginapi.WriteError(os.Stdout, err.Error())
		return
	}
	defer db.Close()

	if err := deleteStaticHost(db, payload.MACAddress); err != nil {
		_ = pluginapi.WriteError(os.Stdout, fmt.Sprintf("failed to delete static host: %v", err))
		return
	}
	_ = pluginapi.WriteSuccess(os.Stdout, map[string]string{"mac_address": payload.MACAddress})
}

// decodeStaticHost unmarshals and validates a StaticHost from the request payload.
func decodeStaticHost(req *pluginapi.Request) (*config.StaticHost, error) {
	var host config.StaticHost
	if err := json.Unmarshal(req.Data, &host); err != nil {
		return nil, fmt.Errorf("invalid static host payload: %w", err)
	}
	host.MACAddress = config.NormalizeMAC(host.MACAddress)
	if err := validateStaticHost(&host); err != nil {
		return nil, err
	}
	return &host, nil
}

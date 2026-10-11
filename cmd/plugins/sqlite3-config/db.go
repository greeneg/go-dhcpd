package main

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/pluginapi"
)

// stringListBinding associates one of SubnetConfig's []string attributes
// with the table that stores it. SQLite has no native array column type, so
// each ordered string-list attribute (domain_name_servers, routers, etc.)
// gets its own table of (subnet_id, position, value) rows.
type stringListBinding struct {
	table string
	get   func(*config.SubnetConfig) []string
	set   func(*config.SubnetConfig, []string)
}

var stringListBindings = []stringListBinding{
	{"subnet_domain_name_servers",
		func(s *config.SubnetConfig) []string { return s.DomainNameServers },
		func(s *config.SubnetConfig, v []string) { s.DomainNameServers = v }},
	{"subnet_domain_search",
		func(s *config.SubnetConfig) []string { return s.DomainSearch },
		func(s *config.SubnetConfig, v []string) { s.DomainSearch = v }},
	{"subnet_ntp_servers",
		func(s *config.SubnetConfig) []string { return s.NTPServers },
		func(s *config.SubnetConfig, v []string) { s.NTPServers = v }},
	{"subnet_time_servers",
		func(s *config.SubnetConfig) []string { return s.TimeServers },
		func(s *config.SubnetConfig, v []string) { s.TimeServers = v }},
	{"subnet_smtp_servers",
		func(s *config.SubnetConfig) []string { return s.SMTPServers },
		func(s *config.SubnetConfig, v []string) { s.SMTPServers = v }},
	{"subnet_lpr_servers",
		func(s *config.SubnetConfig) []string { return s.LPRServers },
		func(s *config.SubnetConfig, v []string) { s.LPRServers = v }},
	{"subnet_netbios_name_servers",
		func(s *config.SubnetConfig) []string { return s.NetBIOSNameServers },
		func(s *config.SubnetConfig, v []string) { s.NetBIOSNameServers = v }},
	{"subnet_netbios_dd_servers",
		func(s *config.SubnetConfig) []string { return s.NetBIOSDDServers },
		func(s *config.SubnetConfig, v []string) { s.NetBIOSDDServers = v }},
	{"subnet_routers",
		func(s *config.SubnetConfig) []string { return s.Routers },
		func(s *config.SubnetConfig, v []string) { s.Routers = v }},
}

// openDB opens (creating if needed) the SQLite database named by the
// request's "db_path" setting and ensures its schema exists.
func openDB(req *pluginapi.Request) (*sql.DB, error) {
	path, ok := req.Settings["db_path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("the %q plugin requires a non-empty \"db_path\" setting", pluginName)
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database %q: %w", path, err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign key support: %w", err)
	}
	if err := ensureSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return db, nil
}

// ensureSchema creates the subnets table, its dynamic-range table, and one
// table per string-list attribute, if they don't already exist.
func ensureSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS subnets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		network TEXT NOT NULL UNIQUE,
		netmask TEXT NOT NULL,
		enable_bootp INTEGER NOT NULL DEFAULT 0,
		netbios_node_type INTEGER,
		domain_name TEXT,
		interface_mtu INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS subnet_dynamic_ranges (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subnet_id INTEGER NOT NULL REFERENCES subnets(id) ON DELETE CASCADE,
		position INTEGER NOT NULL,
		start_address TEXT NOT NULL,
		end_address TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_subnet_dynamic_ranges_subnet ON subnet_dynamic_ranges(subnet_id);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	for _, b := range stringListBindings {
		stmt := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %[1]s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			subnet_id INTEGER NOT NULL REFERENCES subnets(id) ON DELETE CASCADE,
			position INTEGER NOT NULL,
			value TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_%[1]s_subnet ON %[1]s(subnet_id);
		`, b.table)
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("table %s: %w", b.table, err)
		}
	}

	return nil
}

// getSubnets loads every subnet row along with its dynamic ranges and
// string-list attributes.
func getSubnets(db *sql.DB) ([]config.SubnetConfig, error) {
	rows, err := db.Query(`
		SELECT id, network, netmask, enable_bootp, netbios_node_type, domain_name, interface_mtu
		FROM subnets
		ORDER BY network
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subnets []config.SubnetConfig
	var ids []int64
	for rows.Next() {
		var id int64
		var s config.SubnetConfig
		var enableBootP int
		var nodeType sql.NullInt64
		var domainName sql.NullString
		var mtu sql.NullInt64

		if err := rows.Scan(&id, &s.Network, &s.Netmask, &enableBootP, &nodeType, &domainName, &mtu); err != nil {
			return nil, err
		}
		s.EnableBootP = enableBootP != 0
		if nodeType.Valid {
			v := int(nodeType.Int64)
			s.NetBIOSNodeType = &v
		}
		if domainName.Valid {
			s.DomainName = domainName.String
		}
		if mtu.Valid {
			v := int(mtu.Int64)
			s.InterfaceMTU = &v
		}

		subnets = append(subnets, s)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if subnets == nil {
		subnets = []config.SubnetConfig{}
	}

	for i, id := range ids {
		ranges, err := getDynamicRanges(db, id)
		if err != nil {
			return nil, err
		}
		subnets[i].DynamicRanges = ranges

		for _, b := range stringListBindings {
			values, err := getStringList(db, b.table, id)
			if err != nil {
				return nil, err
			}
			b.set(&subnets[i], values)
		}
	}

	return subnets, nil
}

func getDynamicRanges(db *sql.DB, subnetID int64) ([]config.DynamicRange, error) {
	rows, err := db.Query(`
		SELECT start_address, end_address FROM subnet_dynamic_ranges
		WHERE subnet_id = ? ORDER BY position
	`, subnetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ranges []config.DynamicRange
	for rows.Next() {
		var r config.DynamicRange
		if err := rows.Scan(&r.StartAddress, &r.EndAddress); err != nil {
			return nil, err
		}
		ranges = append(ranges, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if ranges == nil {
		ranges = []config.DynamicRange{}
	}
	return ranges, nil
}

func getStringList(db *sql.DB, table string, subnetID int64) ([]string, error) {
	query := fmt.Sprintf(`SELECT value FROM %s WHERE subnet_id = ? ORDER BY position`, table)
	rows, err := db.Query(query, subnetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if values == nil {
		values = []string{}
	}
	return values, nil
}

// insertSubnet adds a new subnet and all of its child rows in one transaction.
func insertSubnet(db *sql.DB, subnet *config.SubnetConfig) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO subnets (network, netmask, enable_bootp, netbios_node_type, domain_name, interface_mtu)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		subnet.Network, subnet.Netmask, boolToInt(subnet.EnableBootP),
		nullableInt(subnet.NetBIOSNodeType), nullableString(subnet.DomainName), nullableInt(subnet.InterfaceMTU),
	)
	if err != nil {
		return fmt.Errorf("subnet %q may already exist: %w", subnet.Network, err)
	}

	subnetID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	if err := writeChildren(tx, subnetID, subnet); err != nil {
		return err
	}

	return tx.Commit()
}

// updateSubnet replaces an existing subnet's fields and child rows. The
// subnet is looked up by subnet.Network, which the API layer treats as the
// immutable identifier (the URL path parameter, not renameable via update).
func updateSubnet(db *sql.DB, subnet *config.SubnetConfig) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var subnetID int64
	err = tx.QueryRow(`SELECT id FROM subnets WHERE network = ?`, subnet.Network).Scan(&subnetID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("no subnet found for network %q", subnet.Network)
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`UPDATE subnets SET netmask = ?, enable_bootp = ?, netbios_node_type = ?, domain_name = ?, interface_mtu = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		subnet.Netmask, boolToInt(subnet.EnableBootP),
		nullableInt(subnet.NetBIOSNodeType), nullableString(subnet.DomainName), nullableInt(subnet.InterfaceMTU),
		subnetID,
	)
	if err != nil {
		return err
	}

	if err := clearChildren(tx, subnetID); err != nil {
		return err
	}
	if err := writeChildren(tx, subnetID, subnet); err != nil {
		return err
	}

	return tx.Commit()
}

// deleteSubnet removes a subnet by network address; its child rows are
// removed automatically via ON DELETE CASCADE.
func deleteSubnet(db *sql.DB, network string) error {
	result, err := db.Exec(`DELETE FROM subnets WHERE network = ?`, network)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no subnet found for network %q", network)
	}
	return nil
}

func clearChildren(tx *sql.Tx, subnetID int64) error {
	if _, err := tx.Exec(`DELETE FROM subnet_dynamic_ranges WHERE subnet_id = ?`, subnetID); err != nil {
		return err
	}
	for _, b := range stringListBindings {
		query := fmt.Sprintf(`DELETE FROM %s WHERE subnet_id = ?`, b.table)
		if _, err := tx.Exec(query, subnetID); err != nil {
			return err
		}
	}
	return nil
}

func writeChildren(tx *sql.Tx, subnetID int64, subnet *config.SubnetConfig) error {
	for i, r := range subnet.DynamicRanges {
		if _, err := tx.Exec(
			`INSERT INTO subnet_dynamic_ranges (subnet_id, position, start_address, end_address) VALUES (?, ?, ?, ?)`,
			subnetID, i, r.StartAddress, r.EndAddress,
		); err != nil {
			return err
		}
	}

	for _, b := range stringListBindings {
		query := fmt.Sprintf(`INSERT INTO %s (subnet_id, position, value) VALUES (?, ?, ?)`, b.table)
		for i, v := range b.get(subnet) {
			if _, err := tx.Exec(query, subnetID, i, v); err != nil {
				return err
			}
		}
	}

	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

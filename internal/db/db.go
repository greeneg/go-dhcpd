package db

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database represents the DHCP database
type Database struct {
	conn *sql.DB
}

// Lease represents a DHCP lease
type Lease struct {
	ID         int64
	MACAddress string
	IPAddress  string
	Hostname   string
	LeaseStart time.Time
	LeaseEnd   time.Time
	State      string // "active", "expired", "reserved"
	IsStatic   bool
}

// DenyAddress represents a martian address
type DenyAddress struct {
	ID         int64
	IPAddress  string
	MACAddress string
	Reason     string
	DetectedAt time.Time
}

// NewDatabase creates a new database connection
func NewDatabase(path string) (*Database, error) {
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	db := &Database{conn: conn}
	if err := db.initialize(); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

// initialize creates the database schema
func (db *Database) initialize() error {
	schema := `
	CREATE TABLE IF NOT EXISTS leases (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mac_address TEXT NOT NULL,
		ip_address TEXT NOT NULL UNIQUE,
		hostname TEXT,
		lease_start DATETIME NOT NULL,
		lease_end DATETIME NOT NULL,
		state TEXT NOT NULL,
		is_static BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_leases_mac ON leases(mac_address);
	CREATE INDEX IF NOT EXISTS idx_leases_ip ON leases(ip_address);
	CREATE INDEX IF NOT EXISTS idx_leases_state ON leases(state);

	CREATE TABLE IF NOT EXISTS deny_addresses (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ip_address TEXT NOT NULL UNIQUE,
		mac_address TEXT,
		reason TEXT NOT NULL,
		detected_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_deny_ip ON deny_addresses(ip_address);
	`

	_, err := db.conn.Exec(schema)
	return err
}

// AddLease adds a new lease to the database
func (db *Database) AddLease(lease *Lease) error {
	query := `
	INSERT INTO leases (mac_address, ip_address, hostname, lease_start, lease_end, state, is_static)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := db.conn.Exec(query,
		lease.MACAddress,
		lease.IPAddress,
		lease.Hostname,
		lease.LeaseStart,
		lease.LeaseEnd,
		lease.State,
		lease.IsStatic,
	)
	if err != nil {
		return err
	}

	lease.ID, _ = result.LastInsertId()
	return nil
}

// UpdateLease updates an existing lease
func (db *Database) UpdateLease(lease *Lease) error {
	query := `
	UPDATE leases
	SET mac_address = ?, hostname = ?, lease_start = ?, lease_end = ?, state = ?, updated_at = CURRENT_TIMESTAMP
	WHERE ip_address = ?
	`
	_, err := db.conn.Exec(query,
		lease.MACAddress,
		lease.Hostname,
		lease.LeaseStart,
		lease.LeaseEnd,
		lease.State,
		lease.IPAddress,
	)
	return err
}

// GetLeaseByMAC retrieves a lease by MAC address
func (db *Database) GetLeaseByMAC(mac string) (*Lease, error) {
	query := `
	SELECT id, mac_address, ip_address, hostname, lease_start, lease_end, state, is_static
	FROM leases
	WHERE mac_address = ? AND state = 'active'
	ORDER BY lease_end DESC
	LIMIT 1
	`
	var lease Lease
	err := db.conn.QueryRow(query, mac).Scan(
		&lease.ID,
		&lease.MACAddress,
		&lease.IPAddress,
		&lease.Hostname,
		&lease.LeaseStart,
		&lease.LeaseEnd,
		&lease.State,
		&lease.IsStatic,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lease, nil
}

// GetLeaseByIP retrieves a lease by IP address
func (db *Database) GetLeaseByIP(ip string) (*Lease, error) {
	query := `
	SELECT id, mac_address, ip_address, hostname, lease_start, lease_end, state, is_static
	FROM leases
	WHERE ip_address = ?
	ORDER BY lease_end DESC
	LIMIT 1
	`
	var lease Lease
	err := db.conn.QueryRow(query, ip).Scan(
		&lease.ID,
		&lease.MACAddress,
		&lease.IPAddress,
		&lease.Hostname,
		&lease.LeaseStart,
		&lease.LeaseEnd,
		&lease.State,
		&lease.IsStatic,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lease, nil
}

// GetAllLeases retrieves all active leases
func (db *Database) GetAllLeases() ([]Lease, error) {
	query := `
	SELECT id, mac_address, ip_address, hostname, lease_start, lease_end, state, is_static
	FROM leases
	WHERE state = 'active'
	ORDER BY ip_address
	`
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leases []Lease
	for rows.Next() {
		var lease Lease
		err := rows.Scan(
			&lease.ID,
			&lease.MACAddress,
			&lease.IPAddress,
			&lease.Hostname,
			&lease.LeaseStart,
			&lease.LeaseEnd,
			&lease.State,
			&lease.IsStatic,
		)
		if err != nil {
			return nil, err
		}
		leases = append(leases, lease)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return leases, nil
}

// AddDenyAddress adds a martian address to the deny list
func (db *Database) AddDenyAddress(deny *DenyAddress) error {
	query := `
	INSERT INTO deny_addresses (ip_address, mac_address, reason, detected_at)
	VALUES (?, ?, ?, ?)
	ON CONFLICT(ip_address) DO UPDATE SET
		mac_address = excluded.mac_address,
		reason = excluded.reason,
		detected_at = excluded.detected_at
	`
	result, err := db.conn.Exec(query,
		deny.IPAddress,
		deny.MACAddress,
		deny.Reason,
		deny.DetectedAt,
	)
	if err != nil {
		return err
	}

	deny.ID, _ = result.LastInsertId()
	return nil
}

// IsDenyAddress checks if an IP is in the deny list
func (db *Database) IsDenyAddress(ip string) (bool, error) {
	query := `SELECT COUNT(*) FROM deny_addresses WHERE ip_address = ?`
	var count int
	err := db.conn.QueryRow(query, ip).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetDenyAddresses retrieves all denied addresses
func (db *Database) GetDenyAddresses() ([]DenyAddress, error) {
	query := `
	SELECT id, ip_address, mac_address, reason, detected_at
	FROM deny_addresses
	ORDER BY detected_at DESC
	`
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var denies []DenyAddress
	for rows.Next() {
		var deny DenyAddress
		var macAddr sql.NullString
		err := rows.Scan(
			&deny.ID,
			&deny.IPAddress,
			&macAddr,
			&deny.Reason,
			&deny.DetectedAt,
		)
		if err != nil {
			return nil, err
		}
		if macAddr.Valid {
			deny.MACAddress = macAddr.String
		}
		denies = append(denies, deny)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return denies, nil
}

// ExpireLeases marks expired leases as expired
func (db *Database) ExpireLeases() error {
	query := `
	UPDATE leases
	SET state = 'expired', updated_at = CURRENT_TIMESTAMP
	WHERE state = 'active' AND lease_end < ? AND is_static = 0
	`
	_, err := db.conn.Exec(query, time.Now())
	return err
}

// GetStats returns database statistics
func (db *Database) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Active leases
	var activeCount int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM leases WHERE state = 'active'").Scan(&activeCount)
	if err != nil {
		return nil, err
	}
	stats["active_leases"] = activeCount

	// Static leases
	var staticCount int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM leases WHERE state = 'active' AND is_static = 1").Scan(&staticCount)
	if err != nil {
		return nil, err
	}
	stats["static_leases"] = staticCount

	// Expired leases
	var expiredCount int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM leases WHERE state = 'expired'").Scan(&expiredCount)
	if err != nil {
		return nil, err
	}
	stats["expired_leases"] = expiredCount

	// Denied addresses
	var deniedCount int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM deny_addresses").Scan(&deniedCount)
	if err != nil {
		return nil, err
	}
	stats["denied_addresses"] = deniedCount

	return stats, nil
}

// Close closes the database connection
func (db *Database) Close() error {
	return db.conn.Close()
}

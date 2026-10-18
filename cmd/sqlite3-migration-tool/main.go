// Command sqlite3-migration-tool is an offline, one-shot utility that
// migrates go-dhcpd subnet definitions from a JSON5 configuration file into
// a sqlite3-config plugin database. It works directly against the SQLite
// database file via internal/sqlite3store; it does not talk to the go-dhcpd
// daemon, its API, or any plugin process.
//
// Static-lease migration is not yet implemented; only subnet definitions
// are migrated by this tool.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/sqlite3store"
	"github.com/greeneg/go-dhcpd/internal/version"
)

const toolName = "sqlite3-migration-tool"

func main() {
	var configPath, dbPath string
	var dryRun, showHelp, showVersion bool

	// GNU-style flags: both the long and short form of each option write
	// to the same variable, so either spelling works on the command line.
	flag.StringVar(&configPath, "config", "", "Path to the go-dhcpd config.json5 file to read (required)")
	flag.StringVar(&configPath, "c", "", "Shorthand for --config")
	flag.StringVar(&dbPath, "database", "", "Path to the sqlite3 database file to create/update (required)")
	flag.StringVar(&dbPath, "d", "", "Shorthand for --database")
	flag.BoolVar(&dryRun, "dry-run", false, "Validate the configuration without writing to the database")
	flag.BoolVar(&dryRun, "n", false, "Shorthand for --dry-run")
	flag.BoolVar(&showHelp, "help", false, "Show this help message and exit")
	flag.BoolVar(&showHelp, "h", false, "Shorthand for --help")
	flag.BoolVar(&showVersion, "version", false, "Show version information and exit")
	flag.BoolVar(&showVersion, "v", false, "Shorthand for --version")
	flag.Usage = printUsage
	flag.Parse()

	if showHelp {
		printUsage()
		os.Exit(0)
	}
	if showVersion {
		fmt.Printf("%s (go-dhcpd %s)\n", toolName, version.Version)
		os.Exit(0)
	}
	if configPath == "" || dbPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config and --database are both required")
		printUsage()
		os.Exit(2)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to load configuration from %q: %v\n", configPath, err)
		os.Exit(1)
	}
	fmt.Printf("Loaded %d subnet(s) from %s\n", len(cfg.Subnets), configPath)

	if ok := migrateSubnets(cfg.Subnets, dbPath, dryRun); !ok {
		os.Exit(1)
	}
}

// migrateSubnets validates every subnet in subnets and, unless dryRun is
// set, writes each valid one into the sqlite3 database at dbPath (creating
// the file and schema first). It reports progress for each subnet and
// returns false if any subnet failed validation or insertion.
func migrateSubnets(subnets []config.SubnetConfig, dbPath string, dryRun bool) bool {
	if len(subnets) == 0 {
		fmt.Println("No subnets found in configuration; nothing to migrate.")
		return true
	}

	insert := func(s *config.SubnetConfig) error { return nil }
	if !dryRun {
		sqliteDB, err := sqlite3store.OpenDB(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: failed to open/create database %q: %v\n", dbPath, err)
			return false
		}
		defer sqliteDB.Close()
		insert = func(s *config.SubnetConfig) error { return sqlite3store.InsertSubnet(sqliteDB, s) }
		fmt.Printf("Database %s opened and schema ensured\n", dbPath)
	}

	succeeded, failed := 0, 0
	for i := range subnets {
		subnet := subnets[i]
		if err := sqlite3store.ValidateSubnet(&subnet); err != nil {
			fmt.Fprintf(os.Stderr, "  [INVALID] %s: %v\n", subnet.Network, err)
			failed++
			continue
		}
		if dryRun {
			fmt.Printf("  [OK]      %s (netmask %s) would be migrated\n", subnet.Network, subnet.Netmask)
			succeeded++
			continue
		}
		if err := insert(&subnet); err != nil {
			fmt.Fprintf(os.Stderr, "  [FAILED]  %s: %v\n", subnet.Network, err)
			failed++
			continue
		}
		fmt.Printf("  [MIGRATED] %s (netmask %s)\n", subnet.Network, subnet.Netmask)
		succeeded++
	}

	fmt.Printf("\n%d of %d subnet(s) migrated successfully (%d failed)\n", succeeded, len(subnets), failed)
	return failed == 0
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: %s --config FILE --database FILE [OPTIONS]

Migrates subnet definitions from a go-dhcpd JSON5 configuration file into a
sqlite3-config plugin database, creating the database file and schema if
needed. Operates directly on the SQLite database file; the go-dhcpd daemon,
its API, and the sqlite3-config plugin process are not involved and do not
need to be running.

Options:
  -c, --config FILE     Path to the go-dhcpd config.json5 file to read (required)
  -d, --database FILE   Path to the sqlite3 database file to create/update (required)
  -n, --dry-run         Validate the configuration without writing to the database
  -h, --help            Show this help message and exit
  -v, --version         Show version information and exit

Note: static-lease migration is not yet implemented; only subnet
definitions are migrated by this tool.
`, toolName)
}

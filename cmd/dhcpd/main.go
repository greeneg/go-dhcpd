package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/greeneg/go-dhcpd/internal/allocator"
	"github.com/greeneg/go-dhcpd/internal/api"
	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/db"
	"github.com/greeneg/go-dhcpd/internal/dhcp"
	"github.com/greeneg/go-dhcpd/internal/logger"
	"github.com/greeneg/go-dhcpd/internal/plugin"
	"github.com/greeneg/go-dhcpd/internal/version"
)

func main() {
	var configFile string
	var useStdout, showVersion, showHelp bool

	// GNU-style flags: both the long and short form of each option write
	// to the same variable, so either spelling works on the command line.
	flag.StringVar(&configFile, "config", "/etc/go-dhcpd/config.json5", "Path to configuration file")
	flag.StringVar(&configFile, "c", "/etc/go-dhcpd/config.json5", "Shorthand for --config")
	flag.BoolVar(&useStdout, "stdout", false, "Log to stdout instead of syslog")
	flag.BoolVar(&useStdout, "o", false, "Shorthand for --stdout")
	flag.BoolVar(&showVersion, "version", false, "Show version and exit")
	flag.BoolVar(&showVersion, "v", false, "Shorthand for --version")
	flag.BoolVar(&showHelp, "help", false, "Show help and exit")
	flag.BoolVar(&showHelp, "h", false, "Shorthand for --help")
	flag.Parse()

	if showVersion {
		fmt.Printf("go-dhcpd version %s\n", version.Version)
		fmt.Printf("Author: %s\n", version.Author)
		fmt.Printf("Copyright: %s\n", version.Copyright)
		fmt.Printf("Description: %s\n", version.Description)
		fmt.Printf("Repository: %s\n", version.Repository)
		fmt.Println("\nThis application is open-source software distributed under the terms linked below:")
		fmt.Printf("License: %s\n", version.License)
		os.Exit(0)
	}
	if showHelp {
		flag.Usage()
		os.Exit(0)
	}

	// Initialize logger
	if err := logger.InitLogger("go-dhcpd", useStdout); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	logger.Info(fmt.Sprintf("Starting go-dhcpd version %s", version.Version))

	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Critical(fmt.Sprintf("Failed to load configuration from %s: %v", configFile, err))
		os.Exit(1)
	}
	logger.Info(fmt.Sprintf("Configuration loaded from %s", configFile))

	// Initialize database
	database, err := db.NewDatabase(cfg.Global.DatabasePath)
	if err != nil {
		logger.Critical(fmt.Sprintf("Failed to initialize database: %v", err))
		os.Exit(1)
	}
	defer database.Close()
	logger.Info(fmt.Sprintf("Database initialized at %s", cfg.Global.DatabasePath))

	// Load the config-provider plugin (supplies subnets and static leases)
	pluginManager, err := plugin.NewManager(cfg.Plugins.ConfigProvider)
	if err != nil {
		logger.Critical(fmt.Sprintf("Failed to load config-provider plugin: %v", err))
		os.Exit(1)
	}
	caps := pluginManager.Capabilities()
	logger.Info(fmt.Sprintf("Config-provider plugin loaded: name=%s version=%s writable=%t",
		caps.Name, caps.Version, caps.Writable))

	// Create allocator
	alloc, err := allocator.NewAllocator(cfg, database, pluginManager)
	if err != nil {
		logger.Critical(fmt.Sprintf("Failed to create IP allocator: %v", err))
		os.Exit(1)
	}
	logger.Info("IP allocator initialized")

	// Seed the database with static leases so they're immediately reflected
	// in lease queries/metrics without waiting for a client to request one.
	if err := seedStaticLeases(alloc); err != nil {
		logger.Warning(fmt.Sprintf("Failed to seed static leases: %v", err))
	}

	// Create DHCP server
	dhcpServer, err := dhcp.NewServer(cfg, alloc)
	if err != nil {
		logger.Critical(fmt.Sprintf("Failed to create DHCP server: %v", err))
		os.Exit(1)
	}
	logger.Info("DHCP server created")

	// Create API server
	apiServer := api.NewAPI(cfg, database, dhcpServer, pluginManager, alloc)
	logger.Info(fmt.Sprintf("API server initialized on port %d", cfg.Global.APIPort))

	// Start API server in goroutine
	go func() {
		if err := apiServer.Start(); err != nil {
			logger.Error(fmt.Sprintf("API server error: %v", err))
		}
	}()

	// Start DHCP server in goroutine
	go func() {
		if err := dhcpServer.Start(); err != nil {
			logger.Critical(fmt.Sprintf("DHCP server error: %v", err))
			os.Exit(1)
		}
	}()

	// Wait for signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info(fmt.Sprintf("Received signal %v, shutting down...", sig))

	// Cleanup
	dhcpServer.Close()
	logger.Info("Shutdown complete")
}

// seedStaticLeases pre-loads static host assignments into the database using
// the allocator's shared reconciliation logic, so startup seeding and
// API-driven static-host writes stay consistent.
func seedStaticLeases(alloc *allocator.Allocator) error {
	for _, static := range alloc.StaticHosts() {
		if err := alloc.SeedStaticLease(static); err != nil {
			logger.Warning(fmt.Sprintf("Failed to seed static lease for %s: %v", static.MACAddress, err))
			continue
		}
		logger.Info(fmt.Sprintf("Seeded static lease: %s -> %s (%s)",
			static.MACAddress, static.IPAddress, static.Hostname))
	}
	return nil
}

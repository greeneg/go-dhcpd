package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/greeneg/go-dhcpd/internal/allocator"
	"github.com/greeneg/go-dhcpd/internal/api"
	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/db"
	"github.com/greeneg/go-dhcpd/internal/dhcp"
	"github.com/greeneg/go-dhcpd/internal/logger"
)

const version = "0.1.0"

func main() {
	// Parse command line flags
	configFile := flag.String("config", "/etc/go-dhcpd/config.json5", "Path to configuration file")
	useStdout := flag.Bool("stdout", false, "Log to stdout instead of syslog")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("go-dhcpd version %s\n", version)
		os.Exit(0)
	}

	// Initialize logger
	if err := logger.InitLogger("go-dhcpd", *useStdout); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	logger.Info(fmt.Sprintf("Starting go-dhcpd version %s", version))

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Critical(fmt.Sprintf("Failed to load configuration from %s: %v", *configFile, err))
		os.Exit(1)
	}
	logger.Info(fmt.Sprintf("Configuration loaded from %s", *configFile))

	// Initialize database
	database, err := db.NewDatabase(cfg.Global.DatabasePath)
	if err != nil {
		logger.Critical(fmt.Sprintf("Failed to initialize database: %v", err))
		os.Exit(1)
	}
	defer database.Close()
	logger.Info(fmt.Sprintf("Database initialized at %s", cfg.Global.DatabasePath))

	// Initialize static leases
	if err := initializeStaticLeases(cfg, database); err != nil {
		logger.Warning(fmt.Sprintf("Failed to initialize static leases: %v", err))
	}

	// Create allocator
	alloc := allocator.NewAllocator(cfg, database)
	logger.Info("IP allocator initialized")

	// Create DHCP server
	dhcpServer, err := dhcp.NewServer(cfg, alloc)
	if err != nil {
		logger.Critical(fmt.Sprintf("Failed to create DHCP server: %v", err))
		os.Exit(1)
	}
	logger.Info("DHCP server created")

	// Create API server
	apiServer := api.NewAPI(cfg, database, dhcpServer)
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

// initializeStaticLeases pre-loads static host assignments into the database
func initializeStaticLeases(cfg *config.Config, database *db.Database) error {
	for _, static := range cfg.Static {
		// Parse IP address (or resolve hostname)
		var ip string
		if parsed := parseIP(static.IPAddress); parsed != "" {
			ip = parsed
		} else {
			// Try to resolve as hostname
			resolved, err := resolveHostname(static.IPAddress)
			if err != nil {
				logger.Warning(fmt.Sprintf("Failed to resolve hostname %s: %v", static.IPAddress, err))
				continue
			}
			ip = resolved
		}

		// Check if lease already exists
		existingLease, err := database.GetLeaseByIP(ip)
		if err != nil {
			return err
		}

		if existingLease != nil {
			// Update if needed
			if existingLease.MACAddress != static.MACAddress {
				logger.Info(fmt.Sprintf("Updating static lease for %s: %s -> %s",
					ip, existingLease.MACAddress, static.MACAddress))
			}
			continue
		}

		// Create static lease
		lease := &db.Lease{
			MACAddress: static.MACAddress,
			IPAddress:  ip,
			Hostname:   static.Hostname,
			LeaseStart: time.Now(),
			LeaseEnd:   time.Now().AddDate(100, 0, 0), // 100 years (effectively permanent)
			State:      "active",
			IsStatic:   true,
		}

		if err := database.AddLease(lease); err != nil {
			logger.Warning(fmt.Sprintf("Failed to add static lease for %s: %v", static.MACAddress, err))
			continue
		}

		logger.Info(fmt.Sprintf("Added static lease: %s -> %s (%s)",
			static.MACAddress, ip, static.Hostname))
	}

	return nil
}

// parseIP tries to parse a string as an IP address
func parseIP(s string) string {
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	return ""
}

// resolveHostname resolves a hostname to an IP address
func resolveHostname(hostname string) (string, error) {
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no IP addresses found for %s", hostname)
	}
	// Return first IPv4 address
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("no IPv4 address found for %s", hostname)
}

package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/greeneg/go-dhcpd/internal/allocator"
	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/db"
	"github.com/greeneg/go-dhcpd/internal/dhcp"
	"github.com/greeneg/go-dhcpd/internal/logger"
	"github.com/greeneg/go-dhcpd/internal/plugin"
	"github.com/greeneg/go-dhcpd/internal/version"
)

// API represents the web API server
type API struct {
	config  *config.Config
	db      *db.Database
	dhcp    *dhcp.Server
	plugins *plugin.Manager
	alloc   *allocator.Allocator
	engine  *gin.Engine
}

// NewAPI creates a new API server
func NewAPI(cfg *config.Config, database *db.Database, dhcpServer *dhcp.Server, pluginManager *plugin.Manager, alloc *allocator.Allocator) *API {
	// Set Gin to release mode
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(loggerMiddleware())

	api := &API{
		config:  cfg,
		db:      database,
		dhcp:    dhcpServer,
		plugins: pluginManager,
		alloc:   alloc,
		engine:  engine,
	}

	api.setupRoutes()
	return api
}

// setupRoutes configures API routes
func (a *API) setupRoutes() {
	// Health endpoint
	a.engine.GET("/health", a.healthHandler)

	// Stats/metrics endpoints
	a.engine.GET("/metrics", a.metricsHandler)
	a.engine.GET("/metrics/dhcp", a.dhcpStatsHandler)
	a.engine.GET("/metrics/database", a.databaseStatsHandler)

	// Configuration endpoint
	a.engine.GET("/config", a.configHandler)

	// Subnet endpoints (backed by the config-provider plugin)
	a.engine.GET("/subnets", a.subnetsHandler)
	a.engine.POST("/subnets", a.addSubnetHandler)
	a.engine.PUT("/subnets/:network", a.updateSubnetHandler)
	a.engine.DELETE("/subnets/:network", a.deleteSubnetHandler)

	// Static host endpoints (backed by the config-provider plugin)
	a.engine.GET("/static", a.staticHostsHandler)
	a.engine.POST("/static", a.addStaticHostHandler)
	a.engine.PUT("/static/:mac", a.updateStaticHostHandler)
	a.engine.DELETE("/static/:mac", a.deleteStaticHostHandler)

	// Leases endpoints
	a.engine.GET("/leases", a.leasesHandler)
	a.engine.GET("/leases/active", a.activeLeasesHandler)
	a.engine.GET("/leases/:mac", a.leaseByMACHandler)

	// Deny addresses endpoint
	a.engine.GET("/deny", a.denyAddressesHandler)

	// Version endpoint
	a.engine.GET("/version", a.versionHandler)
}

// Start starts the API server
func (a *API) Start() error {
	addr := fmt.Sprintf(":%d", a.config.Global.APIPort)
	logger.Info(fmt.Sprintf("API server listening on %s", addr))
	return a.engine.Run(addr)
}

// Health check handler
func (a *API) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// Metrics handler - overall statistics
func (a *API) metricsHandler(c *gin.Context) {
	stats := a.dhcp.GetStats()
	dbStats, err := a.db.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	uptime := time.Since(stats.StartTime)

	c.JSON(http.StatusOK, gin.H{
		"uptime_seconds":   uptime.Seconds(),
		"uptime_string":    uptime.String(),
		"packets_received": stats.PacketsReceived,
		"packets_sent":     stats.PacketsSent,
		"errors":           stats.Errors,
		"active_leases":    dbStats["active_leases"],
		"static_leases":    dbStats["static_leases"],
		"expired_leases":   dbStats["expired_leases"],
		"denied_addresses": dbStats["denied_addresses"],
	})
}

// DHCP statistics handler
func (a *API) dhcpStatsHandler(c *gin.Context) {
	stats := a.dhcp.GetStats()

	c.JSON(http.StatusOK, gin.H{
		"start_time":       stats.StartTime.Format(time.RFC3339),
		"uptime_seconds":   time.Since(stats.StartTime).Seconds(),
		"discover_count":   stats.DiscoverCount,
		"offer_count":      stats.OfferCount,
		"request_count":    stats.RequestCount,
		"ack_count":        stats.AckCount,
		"nak_count":        stats.NakCount,
		"release_count":    stats.ReleaseCount,
		"inform_count":     stats.InformCount,
		"decline_count":    stats.DeclineCount,
		"packets_received": stats.PacketsReceived,
		"packets_sent":     stats.PacketsSent,
		"errors":           stats.Errors,
	})
}

// Database statistics handler
func (a *API) databaseStatsHandler(c *gin.Context) {
	stats, err := a.db.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// Configuration handler
func (a *API) configHandler(c *gin.Context) {
	subnets, err := a.plugins.GetSubnets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	staticHosts, err := a.plugins.GetStaticHosts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"global":  a.config.Global,
		"plugins": a.config.Plugins,
		"subnets": subnets,
		"static":  staticHosts,
	})
}

// subnetsHandler returns the current subnets as reported by the config-provider plugin
func (a *API) subnetsHandler(c *gin.Context) {
	subnets, err := a.plugins.GetSubnets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"count":   len(subnets),
		"subnets": subnets,
	})
}

// addSubnetHandler adds a new subnet via the config-provider plugin
func (a *API) addSubnetHandler(c *gin.Context) {
	var subnet config.SubnetConfig
	if err := c.ShouldBindJSON(&subnet); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := a.plugins.AddSubnet(subnet); err != nil {
		a.writePluginError(c, err)
		return
	}
	a.refreshAllocator(c)
	c.JSON(http.StatusCreated, subnet)
}

// updateSubnetHandler replaces an existing subnet via the config-provider plugin
func (a *API) updateSubnetHandler(c *gin.Context) {
	var subnet config.SubnetConfig
	if err := c.ShouldBindJSON(&subnet); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	subnet.Network = c.Param("network")
	if err := a.plugins.UpdateSubnet(subnet); err != nil {
		a.writePluginError(c, err)
		return
	}
	a.refreshAllocator(c)
	c.JSON(http.StatusOK, subnet)
}

// deleteSubnetHandler removes a subnet via the config-provider plugin
func (a *API) deleteSubnetHandler(c *gin.Context) {
	network := c.Param("network")
	if err := a.plugins.DeleteSubnet(network); err != nil {
		a.writePluginError(c, err)
		return
	}
	a.refreshAllocator(c)
	c.JSON(http.StatusOK, gin.H{"message": "subnet deleted", "network": network})
}

// staticHostsHandler returns the current static hosts as reported by the config-provider plugin
func (a *API) staticHostsHandler(c *gin.Context) {
	hosts, err := a.plugins.GetStaticHosts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"count":  len(hosts),
		"static": hosts,
	})
}

// addStaticHostHandler adds a new static host via the config-provider plugin
func (a *API) addStaticHostHandler(c *gin.Context) {
	var host config.StaticHost
	if err := c.ShouldBindJSON(&host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := a.plugins.AddStaticHost(host); err != nil {
		a.writePluginError(c, err)
		return
	}
	a.refreshAllocator(c)
	c.JSON(http.StatusCreated, host)
}

// updateStaticHostHandler replaces an existing static host via the config-provider plugin
func (a *API) updateStaticHostHandler(c *gin.Context) {
	var host config.StaticHost
	if err := c.ShouldBindJSON(&host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	host.MACAddress = c.Param("mac")
	if err := a.plugins.UpdateStaticHost(host); err != nil {
		a.writePluginError(c, err)
		return
	}
	a.refreshAllocator(c)
	c.JSON(http.StatusOK, host)
}

// deleteStaticHostHandler removes a static host via the config-provider plugin
func (a *API) deleteStaticHostHandler(c *gin.Context) {
	mac := c.Param("mac")
	if err := a.plugins.DeleteStaticHost(mac); err != nil {
		a.writePluginError(c, err)
		return
	}
	a.refreshAllocator(c)
	c.JSON(http.StatusOK, gin.H{"message": "static host deleted", "mac_address": mac})
}

// writePluginError maps a plugin-reported error to an HTTP response, using
// 403 Forbidden when the plugin rejected the request because it is read-only.
func (a *API) writePluginError(c *gin.Context, err error) {
	if !a.plugins.Capabilities().Writable {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// refreshAllocator reloads the allocator's cached subnets/static hosts after
// a successful write so subsequent allocation decisions see the new data.
func (a *API) refreshAllocator(c *gin.Context) {
	if err := a.alloc.Refresh(); err != nil {
		logger.Error(fmt.Sprintf("Failed to refresh allocator after plugin write: %v", err))
	}
}

// All leases handler
func (a *API) leasesHandler(c *gin.Context) {
	leases, err := a.db.GetAllLeases()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count":  len(leases),
		"leases": leases,
	})
}

// Active leases handler
func (a *API) activeLeasesHandler(c *gin.Context) {
	leases, err := a.db.GetAllLeases()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Filter active leases
	activeLeases := make([]db.Lease, 0)
	for _, lease := range leases {
		if time.Now().Before(lease.LeaseEnd) {
			activeLeases = append(activeLeases, lease)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"count":  len(activeLeases),
		"leases": activeLeases,
	})
}

// Lease by MAC handler
func (a *API) leaseByMACHandler(c *gin.Context) {
	mac := c.Param("mac")
	lease, err := a.db.GetLeaseByMAC(mac)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if lease == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "lease not found"})
		return
	}

	c.JSON(http.StatusOK, lease)
}

// Deny addresses handler
func (a *API) denyAddressesHandler(c *gin.Context) {
	denies, err := a.db.GetDenyAddresses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count":     len(denies),
		"addresses": denies,
	})
}

// Version handler
func (a *API) versionHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":        "go-dhcpd",
		"version":     version.Version,
		"author":      version.Author,
		"repository":  version.Repository,
		"license":     version.License,
		"description": version.Description,
	})
}

// Logger middleware
func loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		clientIP := c.ClientIP()

		if query != "" {
			path = path + "?" + query
		}

		logger.Info(fmt.Sprintf("API: %s %s %d %v %s",
			method, path, statusCode, latency, clientIP))
	}
}

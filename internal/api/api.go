package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/db"
	"github.com/greeneg/go-dhcpd/internal/dhcp"
	"github.com/greeneg/go-dhcpd/internal/logger"
)

// API represents the web API server
type API struct {
	config *config.Config
	db     *db.Database
	dhcp   *dhcp.Server
	engine *gin.Engine
}

// NewAPI creates a new API server
func NewAPI(cfg *config.Config, database *db.Database, dhcpServer *dhcp.Server) *API {
	// Set Gin to release mode
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(loggerMiddleware())

	api := &API{
		config: cfg,
		db:     database,
		dhcp:   dhcpServer,
		engine: engine,
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
	c.JSON(http.StatusOK, a.config)
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
		"name":       "go-dhcpd",
		"version":    "1.0.0",
		"author":     "Gary L. Greene Jr.",
		"repository": "https://github.com/greeneg/go-dhcpd",
		"license":    "Apache-2.0 <https://www.apache.org/licenses/LICENSE-2.0>",
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

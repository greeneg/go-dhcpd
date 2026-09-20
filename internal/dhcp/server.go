package dhcp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/greeneg/go-dhcpd/internal/allocator"
	"github.com/greeneg/go-dhcpd/internal/config"
	"github.com/greeneg/go-dhcpd/internal/logger"
)

// Server represents a DHCP server
type Server struct {
	config      *config.Config
	allocator   *allocator.Allocator
	rawSocket   *RawSocket   // Raw socket for broadcast DHCP packets
	unicastConn *net.UDPConn // Listens on specific IP:67 for relay/unicast packets
	serverIP    net.IP
	stats       *Stats
}

// Stats holds server statistics
type Stats struct {
	StartTime       time.Time
	DiscoverCount   uint64
	OfferCount      uint64
	RequestCount    uint64
	AckCount        uint64
	NakCount        uint64
	ReleaseCount    uint64
	InformCount     uint64
	DeclineCount    uint64
	PacketsReceived uint64
	PacketsSent     uint64
	Errors          uint64
}

// NewServer creates a new DHCP server
func NewServer(cfg *config.Config, alloc *allocator.Allocator) (*Server, error) {
	// Get server IP
	serverIP, err := getServerIP(cfg.Global.ListenAddress)
	if err != nil {
		return nil, err
	}

	return &Server{
		config:    cfg,
		allocator: alloc,
		serverIP:  serverIP,
		stats: &Stats{
			StartTime: time.Now(),
		},
	}, nil
}

// Start starts the DHCP server
func (s *Server) Start() error {
	// Create raw socket for broadcast DHCP packets
	var rawSocket *RawSocket
	var err error

	if s.config.Global.ListenInterface != "" {
		rawSocket, err = NewRawSocket(s.config.Global.ListenInterface)
	} else {
		rawSocket, err = NewRawSocket("")
	}

	if err != nil {
		return fmt.Errorf("failed to create raw socket for broadcasts: %v", err)
	}
	s.rawSocket = rawSocket

	// Log raw socket info
	if s.config.Global.ListenInterface != "" {
		logger.Info(fmt.Sprintf("Raw socket listening on interface %s (broadcast DHCP packets)", s.config.Global.ListenInterface))
	} else {
		logger.Info("Raw socket listening on all interfaces (broadcast DHCP packets)")
	}

	// Create unicast listener on port 67 (specific IP or 0.0.0.0:67)
	unicastIP := s.serverIP
	if s.config.Global.ListenAddress != "" && s.config.Global.ListenAddress != "0.0.0.0" {
		unicastIP = net.ParseIP(s.config.Global.ListenAddress)
	}

	unicastAddr := &net.UDPAddr{
		IP:   unicastIP,
		Port: 67, // Standard DHCP port for unicast/relay
	}

	// Create a ListenConfig for unicast
	unicastLC := &net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err := c.Control(func(fd uintptr) {
				// Set SO_REUSEADDR
				if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
					opErr = fmt.Errorf("failed to set SO_REUSEADDR: %v", err)
					return
				}
				// Set SO_BROADCAST to allow sending broadcast packets
				if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1); err != nil {
					opErr = fmt.Errorf("failed to set SO_BROADCAST: %v", err)
					return
				}
				// Bind to specific interface if configured
				if s.config.Global.ListenInterface != "" {
					if err := syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, s.config.Global.ListenInterface); err != nil {
						opErr = fmt.Errorf("failed to bind to interface %s: %v", s.config.Global.ListenInterface, err)
						return
					}
				}
			})
			if err != nil {
				return err
			}
			return opErr
		},
	}

	// Listen for unicast/relay packets on port 67
	unicastConn, err := unicastLC.ListenPacket(context.Background(), "udp4", unicastAddr.String())
	if err != nil {
		s.rawSocket.Close()
		return fmt.Errorf("failed to listen on UDP port 67 for unicast: %v", err)
	}

	unicastUDP, ok := unicastConn.(*net.UDPConn)
	if !ok {
		s.rawSocket.Close()
		unicastConn.Close()
		return fmt.Errorf("failed to convert unicast connection to UDPConn")
	}
	s.unicastConn = unicastUDP

	// Log unicast listener info
	unicastInfo := fmt.Sprintf("%s:67 (unicast/relay packets)", unicastIP)
	if s.config.Global.ListenInterface != "" {
		unicastInfo = fmt.Sprintf("%s:67 on interface %s (unicast/relay packets)", unicastIP, s.config.Global.ListenInterface)
	}
	logger.Info(fmt.Sprintf("DHCP server listening on %s", unicastInfo))

	// Start lease expiration goroutine
	go s.leaseExpirationLoop()

	// Start goroutine to handle broadcast packets from raw socket
	go s.handleRawSocket()

	// Handle unicast packets in main goroutine
	s.handleConnection(s.unicastConn, "unicast")

	return nil
}

// handleRawSocket handles DHCP packets from the raw socket
func (s *Server) handleRawSocket() {
	for {
		dhcpData, srcMAC, err := s.rawSocket.ReadDHCPPacket()
		if err != nil {
			logger.Error(fmt.Sprintf("Error reading from raw socket: %v", err))
			s.stats.Errors++
			continue
		}

		s.stats.PacketsReceived++

		// Parse DHCP packet
		packet, err := ParsePacket(dhcpData)
		if err != nil {
			logger.Error(fmt.Sprintf("Error parsing DHCP packet from raw socket: %v", err))
			s.stats.Errors++
			continue
		}

		// Create a dummy remote address for broadcast packets
		// The packet came from broadcast, so we use 0.0.0.0:68 as source
		remoteAddr := &net.UDPAddr{
			IP:   net.IPv4zero,
			Port: 68,
		}

		logger.Debug(fmt.Sprintf("Received broadcast DHCP packet from MAC %s", srcMAC))

		// Handle packet in goroutine
		go s.handlePacket(packet, remoteAddr)
	}
}

// handleConnection handles packets from a specific connection
func (s *Server) handleConnection(conn *net.UDPConn, connType string) {
	buffer := make([]byte, 1500)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			logger.Error(fmt.Sprintf("Error reading UDP packet from %s listener: %v", connType, err))
			s.stats.Errors++
			continue
		}

		s.stats.PacketsReceived++

		// Parse packet
		packet, err := ParsePacket(buffer[:n])
		if err != nil {
			logger.Error(fmt.Sprintf("Error parsing DHCP packet from %s listener: %v", connType, err))
			s.stats.Errors++
			continue
		}

		// Handle packet in goroutine
		go s.handlePacket(packet, remoteAddr)
	}
}

// handlePacket handles a DHCP packet
func (s *Server) handlePacket(packet *Packet, remoteAddr *net.UDPAddr) {
	msgType, err := packet.GetMessageType()
	if err != nil {
		// Might be a BootP request
		s.handleBootP()
		return
	}

	macAddr := packet.MACAddressString()
	logger.Info(fmt.Sprintf("Received %s from %s (MAC: %s, XID: %08x)",
		MessageTypeName(msgType), remoteAddr.IP, macAddr, packet.Xid))

	switch msgType {
	case DHCPDiscover:
		s.stats.DiscoverCount++
		s.handleDiscover(packet)
	case DHCPRequest:
		s.stats.RequestCount++
		s.handleRequest(packet)
	case DHCPRelease:
		s.stats.ReleaseCount++
		s.handleRelease(packet)
	case DHCPDecline:
		s.stats.DeclineCount++
		s.handleDecline(packet)
	case DHCPInform:
		s.stats.InformCount++
		s.handleInform(packet)
	default:
		logger.Warning(fmt.Sprintf("Unhandled DHCP message type: %s", MessageTypeName(msgType)))
	}
}

// handleDiscover handles DHCP DISCOVER messages
func (s *Server) handleDiscover(packet *Packet) {
	macAddr := packet.MACAddressString()
	requestedIP := packet.GetRequestedIP()

	// Allocate IP
	ip, err := s.allocator.AllocateIP(macAddr, requestedIP)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to allocate IP for %s: %v", macAddr, err))
		return
	}

	// Find subnet for this IP
	subnet := s.findSubnetForIP(ip)
	if subnet == nil {
		logger.Error(fmt.Sprintf("No subnet found for IP %s", ip))
		return
	}

	// Create DHCP OFFER
	offer := s.createOffer(packet, ip, subnet)

	// Send offer
	s.sendPacket(offer)
	s.stats.OfferCount++

	logger.Info(fmt.Sprintf("Sent DHCPOFFER to %s for IP %s", macAddr, ip))
}

// handleRequest handles DHCP REQUEST messages
func (s *Server) handleRequest(packet *Packet) {
	macAddr := packet.MACAddressString()
	requestedIP := packet.GetRequestedIP()
	serverID := packet.GetServerIdentifier()

	// Check if this request is for us
	if serverID != nil && !serverID.Equal(s.serverIP) {
		logger.Debug(fmt.Sprintf("REQUEST not for us (server ID: %s)", serverID))
		return
	}

	// Get requested IP (from option or ciaddr)
	var ip net.IP
	if requestedIP != nil && !requestedIP.IsUnspecified() {
		ip = requestedIP
	} else if !packet.CIAddr.IsUnspecified() {
		ip = packet.CIAddr
	} else {
		logger.Error(fmt.Sprintf("No IP requested in REQUEST from %s", macAddr))
		s.sendNak(packet)
		return
	}

	// Verify IP is available for this MAC
	allocated, err := s.allocator.AllocateIP(macAddr, ip)
	if err != nil || !allocated.Equal(ip) {
		logger.Warning(fmt.Sprintf("Cannot allocate requested IP %s for %s", ip, macAddr))
		s.sendNak(packet)
		return
	}

	// Find subnet
	subnet := s.findSubnetForIP(ip)
	if subnet == nil {
		logger.Error(fmt.Sprintf("No subnet found for IP %s", ip))
		s.sendNak(packet)
		return
	}

	// Create lease
	hostname := s.getHostname(packet)
	isStatic := s.isStaticLease(macAddr)
	if err := s.allocator.CreateLease(macAddr, ip, hostname, isStatic); err != nil {
		logger.Error(fmt.Sprintf("Failed to create lease: %v", err))
		s.sendNak(packet)
		return
	}

	// Send ACK
	ack := s.createAck(packet, ip, subnet)
	s.sendPacket(ack)
	s.stats.AckCount++

	logger.Info(fmt.Sprintf("Sent DHCPACK to %s for IP %s", macAddr, ip))
}

// handleRelease handles DHCP RELEASE messages
func (s *Server) handleRelease(packet *Packet) {
	macAddr := packet.MACAddressString()
	if err := s.allocator.ReleaseLease(macAddr); err != nil {
		logger.Error(fmt.Sprintf("Failed to release lease for %s: %v", macAddr, err))
		return
	}
	logger.Info(fmt.Sprintf("Released lease for %s", macAddr))
}

// handleDecline handles DHCP DECLINE messages
func (s *Server) handleDecline(packet *Packet) {
	macAddr := packet.MACAddressString()
	requestedIP := packet.GetRequestedIP()
	logger.Notice(fmt.Sprintf("Client %s declined IP %s", macAddr, requestedIP))
	// IP is already marked as martian by the client's duplicate address detection
}

// handleInform handles DHCP INFORM messages
func (s *Server) handleInform(packet *Packet) {
	// Client already has IP, just needs configuration
	subnet := s.findSubnetForIP(packet.CIAddr)
	if subnet == nil {
		return
	}

	ack := s.createAck(packet, packet.CIAddr, subnet)
	ack.YIAddr = net.IPv4zero // Don't assign IP for INFORM
	s.sendPacket(ack)
}

// handleBootP handles BootP requests
func (s *Server) handleBootP() {
	// Check if BootP is enabled
	// For simplicity, we'll treat BootP like DHCP for now
	logger.Debug("Received BootP request")
	// BootP handling would be similar to DHCP but without lease times
}

// createOffer creates a DHCP OFFER packet
func (s *Server) createOffer(request *Packet, ip net.IP, subnet *config.SubnetConfig) *Packet {
	offer := &Packet{
		OpCode:  BootReply,
		HwType:  request.HwType,
		HwLen:   request.HwLen,
		Hops:    0,
		Xid:     request.Xid,
		Secs:    0,
		Flags:   request.Flags,
		CIAddr:  net.IPv4zero,
		YIAddr:  ip.To4(),
		SIAddr:  s.serverIP.To4(),
		GIAddr:  request.GIAddr,
		CHAddr:  request.CHAddr,
		Options: make(map[byte][]byte),
	}

	offer.SetMessageType(DHCPOffer)
	offer.SetServerIdentifier(s.serverIP)
	offer.SetLeaseTime(uint32(s.config.Global.LeaseTime))
	s.addSubnetOptions(offer, subnet)

	return offer
}

// createAck creates a DHCP ACK packet
func (s *Server) createAck(request *Packet, ip net.IP, subnet *config.SubnetConfig) *Packet {
	ack := &Packet{
		OpCode:  BootReply,
		HwType:  request.HwType,
		HwLen:   request.HwLen,
		Hops:    0,
		Xid:     request.Xid,
		Secs:    0,
		Flags:   request.Flags,
		CIAddr:  net.IPv4zero,
		YIAddr:  ip.To4(),
		SIAddr:  s.serverIP.To4(),
		GIAddr:  request.GIAddr,
		CHAddr:  request.CHAddr,
		Options: make(map[byte][]byte),
	}

	ack.SetMessageType(DHCPAck)
	ack.SetServerIdentifier(s.serverIP)
	ack.SetLeaseTime(uint32(s.config.Global.LeaseTime))
	s.addSubnetOptions(ack, subnet)

	return ack
}

// sendNak sends a DHCP NAK
func (s *Server) sendNak(request *Packet) {
	nak := &Packet{
		OpCode:  BootReply,
		HwType:  request.HwType,
		HwLen:   request.HwLen,
		Hops:    0,
		Xid:     request.Xid,
		Secs:    0,
		Flags:   request.Flags,
		CIAddr:  net.IPv4zero,
		YIAddr:  net.IPv4zero,
		SIAddr:  s.serverIP.To4(),
		GIAddr:  request.GIAddr,
		CHAddr:  request.CHAddr,
		Options: make(map[byte][]byte),
	}

	nak.SetMessageType(DHCPNak)
	nak.SetServerIdentifier(s.serverIP)

	s.sendPacket(nak)
	s.stats.NakCount++
}

// addSubnetOptions adds subnet-specific options to a packet
func (s *Server) addSubnetOptions(packet *Packet, subnet *config.SubnetConfig) {
	// Subnet mask - parse from dotted decimal notation
	netmask := net.ParseIP(subnet.Netmask)
	if netmask != nil {
		mask := net.IPMask(netmask.To4())
		if mask != nil {
			packet.SetSubnetMask(mask)
		}
	}

	// Routers
	if len(subnet.Routers) > 0 {
		routers := parseIPs(subnet.Routers)
		packet.SetRouter(routers)
	}

	// DNS servers
	if len(subnet.DomainNameServers) > 0 {
		dns := parseIPs(subnet.DomainNameServers)
		packet.SetDNS(dns)
	}

	// Domain name
	if subnet.DomainName != "" {
		packet.SetDomainName(subnet.DomainName)
	}

	// NTP servers
	if len(subnet.NTPServers) > 0 {
		ntp := parseIPs(subnet.NTPServers)
		packet.SetNTPServers(ntp)
	}

	// NetBIOS name servers
	if len(subnet.NetBIOSNameServers) > 0 {
		netbios := parseIPs(subnet.NetBIOSNameServers)
		packet.SetNetBIOSNameServers(netbios)
	}

	// NetBIOS Datagram Distribution servers
	if len(subnet.NetBIOSDDServers) > 0 {
		netbiosDD := parseIPs(subnet.NetBIOSDDServers)
		packet.SetNetBIOSDDServers(netbiosDD)
	}

	// NetBIOS node type
	nodeType := s.config.Global.NetBIOSNodeType
	if subnet.NetBIOSNodeType != nil {
		nodeType = *subnet.NetBIOSNodeType
	}
	packet.SetNetBIOSNodeType(uint8(nodeType))

	// Interface MTU
	if subnet.InterfaceMTU != nil {
		packet.SetInterfaceMTU(uint16(*subnet.InterfaceMTU))
	}
}

// sendPacket sends a DHCP packet
func (s *Server) sendPacket(packet *Packet) error {
	data := packet.ToBytes()

	// Determine destination address based on RFC 2131 section 4.1
	// Default to broadcast (for clients without IP addresses)
	destAddr := &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: 68, // DHCP client port
	}

	// If GIADDR is set, send to relay agent
	if !packet.GIAddr.IsUnspecified() {
		destAddr.IP = packet.GIAddr
		destAddr.Port = 67
		logger.Debug(fmt.Sprintf("Sending packet via relay agent to %s:%d", destAddr.IP, destAddr.Port))
	} else if (packet.Flags&0x8000) == 0 && !packet.CIAddr.IsUnspecified() {
		// Broadcast flag not set and client has IP - send unicast
		destAddr.IP = packet.CIAddr
		logger.Debug(fmt.Sprintf("Sending packet via unicast to %s:%d", destAddr.IP, destAddr.Port))
	} else {
		// Send broadcast (client has no IP or broadcast flag is set)
		logger.Debug(fmt.Sprintf("Sending packet via broadcast to %s:%d", destAddr.IP, destAddr.Port))
	}

	// Send responses from the unicast connection (port 67)
	// This socket has SO_BROADCAST enabled to allow broadcast sends
	_, err := s.unicastConn.WriteToUDP(data, destAddr)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to send packet to %s:%d: %v", destAddr.IP, destAddr.Port, err))
		s.stats.Errors++
		return err
	}

	s.stats.PacketsSent++
	return nil
}

// findSubnetForIP finds the subnet configuration for an IP
func (s *Server) findSubnetForIP(ip net.IP) *config.SubnetConfig {
	for _, subnet := range s.config.Subnets {
		// Parse network address
		networkIP := net.ParseIP(subnet.Network)
		if networkIP == nil {
			logger.Error(fmt.Sprintf("Invalid network address: %s", subnet.Network))
			continue
		}

		// Parse netmask
		netmask := net.ParseIP(subnet.Netmask)
		if netmask == nil {
			logger.Error(fmt.Sprintf("Invalid netmask: %s", subnet.Netmask))
			continue
		}

		// Convert netmask to net.IPMask
		mask := net.IPMask(netmask.To4())
		if mask == nil {
			logger.Error(fmt.Sprintf("Failed to convert netmask to IPMask: %s", subnet.Netmask))
			continue
		}

		// Create IPNet
		ipNet := &net.IPNet{
			IP:   networkIP.Mask(mask),
			Mask: mask,
		}

		// Check if IP is in this subnet
		if ipNet.Contains(ip) {
			return &subnet
		}
	}
	return nil
}

// getHostname extracts hostname from packet options
func (s *Server) getHostname(packet *Packet) string {
	if hostname, ok := packet.Options[OptionHostname]; ok {
		return string(hostname)
	}
	return ""
}

// leaseExpirationLoop periodically expires old leases
func (s *Server) leaseExpirationLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// This would call db.ExpireLeases()
		logger.Debug("Running lease expiration check")
	}
}

// GetStats returns server statistics
func (s *Server) GetStats() *Stats {
	return s.stats
}

// Close closes the server
func (s *Server) Close() error {
	var err error
	if s.rawSocket != nil {
		if closeErr := s.rawSocket.Close(); closeErr != nil {
			err = closeErr
		}
	}
	if s.unicastConn != nil {
		if closeErr := s.unicastConn.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
			}
		}
	}
	return err
}

// isStaticLease checks if a MAC address has a static assignment
func (s *Server) isStaticLease(macAddr string) bool {
	macAddr = toLowerMAC(macAddr)
	for _, static := range s.config.Static {
		if toLowerMAC(static.MACAddress) == macAddr {
			return true
		}
	}
	return false
}

// Helper functions

func parseIPs(ips []string) []net.IP {
	result := make([]net.IP, 0, len(ips))
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			result = append(result, ip)
		}
	}
	return result
}

func getServerIP(listenAddr string) (net.IP, error) {
	if listenAddr != "0.0.0.0" && listenAddr != "" {
		return net.ParseIP(listenAddr), nil
	}

	// Get primary interface IP
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP, nil
			}
		}
	}

	return nil, fmt.Errorf("no suitable IP address found")
}

func toLowerMAC(mac string) string {
	return strings.ToLower(mac)
}

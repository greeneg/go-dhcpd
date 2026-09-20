package dhcp

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"

	"github.com/greeneg/go-dhcpd/internal/logger"
)

// RawSocket represents a raw packet socket for receiving DHCP broadcasts
type RawSocket struct {
	fd        int
	ifaceIdx  int
	ifaceName string
}

// NewRawSocket creates a new raw socket bound to the specified interface
func NewRawSocket(ifaceName string) (*RawSocket, error) {
	// Create raw socket - AF_PACKET, SOCK_RAW, ETH_P_IP
	// SOCK_RAW = 3, with flags for state 7: SOCK_NONBLOCK | SOCK_CLOEXEC
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, int(htons(syscall.ETH_P_IP)))
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %v", err)
	}

	rs := &RawSocket{
		fd:        fd,
		ifaceName: ifaceName,
	}

	// If interface is specified, bind to it
	if ifaceName != "" {
		iface, err := net.InterfaceByName(ifaceName)
		if err != nil {
			syscall.Close(fd)
			return nil, fmt.Errorf("failed to get interface %s: %v", ifaceName, err)
		}
		rs.ifaceIdx = iface.Index

		// Bind socket to interface
		sll := &syscall.SockaddrLinklayer{
			Protocol: htons(syscall.ETH_P_IP),
			Ifindex:  iface.Index,
		}
		if err := syscall.Bind(fd, sll); err != nil {
			syscall.Close(fd)
			return nil, fmt.Errorf("failed to bind socket to interface %s: %v", ifaceName, err)
		}
		logger.Info(fmt.Sprintf("Raw socket bound to interface %s (index %d)", ifaceName, iface.Index))
	} else {
		// Listen on all interfaces
		logger.Info("Raw socket listening on all interfaces")
	}

	return rs, nil
}

// ReadDHCPPacket reads a DHCP packet from the raw socket
// Returns the DHCP packet data and source hardware address
func (rs *RawSocket) ReadDHCPPacket() ([]byte, net.HardwareAddr, error) {
	buffer := make([]byte, 1500)

	for {
		n, _, err := syscall.Recvfrom(rs.fd, buffer, 0)
		if err != nil {
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				// Non-blocking socket, no data available
				continue
			}
			return nil, nil, fmt.Errorf("error reading from raw socket: %v", err)
		}

		if n < 14 {
			// Too small for Ethernet header
			continue
		}

		// Parse Ethernet header (14 bytes)
		// Destination MAC (6) + Source MAC (6) + EtherType (2)
		srcMAC := net.HardwareAddr(buffer[6:12])
		etherType := binary.BigEndian.Uint16(buffer[12:14])

		// Check if it's an IP packet (0x0800)
		if etherType != 0x0800 {
			continue
		}

		// Parse IP header (starts at byte 14)
		if n < 34 {
			// Too small for IP + UDP headers
			continue
		}

		ipStart := 14
		ipHeader := buffer[ipStart:]

		// Check IP version (should be 4)
		version := ipHeader[0] >> 4
		if version != 4 {
			continue
		}

		// Get IP header length (in 32-bit words)
		ihl := int(ipHeader[0]&0x0F) * 4
		if n < ipStart+ihl+8 {
			// Too small for UDP header
			continue
		}

		// Check if it's UDP (protocol 17)
		protocol := ipHeader[9]
		if protocol != 17 {
			continue
		}

		// Parse UDP header (starts after IP header)
		udpStart := ipStart + ihl
		udpHeader := buffer[udpStart:]

		// Get destination port (bytes 2-3 of UDP header)
		dstPort := binary.BigEndian.Uint16(udpHeader[2:4])

		// Check if destination port is 67 (DHCP server)
		if dstPort != 67 {
			continue
		}

		// Get UDP length (bytes 4-5)
		udpLength := binary.BigEndian.Uint16(udpHeader[4:6])
		if udpLength < 8 {
			continue
		}

		// Extract DHCP payload (starts after UDP header, which is 8 bytes)
		dhcpStart := udpStart + 8
		dhcpLength := int(udpLength) - 8

		if n < dhcpStart+dhcpLength {
			// Packet truncated
			continue
		}

		dhcpData := make([]byte, dhcpLength)
		copy(dhcpData, buffer[dhcpStart:dhcpStart+dhcpLength])

		return dhcpData, srcMAC, nil
	}
}

// Close closes the raw socket
func (rs *RawSocket) Close() error {
	if rs.fd >= 0 {
		return syscall.Close(rs.fd)
	}
	return nil
}

// htons converts host byte order to network byte order (16-bit)
func htons(v uint16) uint16 {
	return (v << 8) | (v >> 8)
}

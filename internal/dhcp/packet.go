package dhcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

// DHCP Message Types
const (
	DHCPDiscover = 1
	DHCPOffer    = 2
	DHCPRequest  = 3
	DHCPDecline  = 4
	DHCPAck      = 5
	DHCPNak      = 6
	DHCPRelease  = 7
	DHCPInform   = 8
)

// DHCP Options
const (
	OptionSubnetMask           = 1
	OptionRouter               = 3
	OptionDNS                  = 6
	OptionHostname             = 12
	OptionDomainName           = 15
	OptionInterfaceMTU         = 26
	OptionBroadcastAddress     = 28
	OptionNTPServers           = 42
	OptionNetBIOSNameServers   = 44
	OptionNetBIOSDDServer      = 45
	OptionNetBIOSNodeType      = 46
	OptionRequestedIPAddress   = 50
	OptionLeaseTime            = 51
	OptionMessageType          = 53
	OptionServerIdentifier     = 54
	OptionParameterRequestList = 55
	OptionRenewalTime          = 58
	OptionRebindingTime        = 59
	OptionClientIdentifier     = 61
	OptionDomainSearch         = 119
	OptionEnd                  = 255
)

// DHCP BootP operation codes
const (
	BootRequest = 1
	BootReply   = 2
)

// Magic cookie for DHCP
var MagicCookie = []byte{99, 130, 83, 99}

// Packet represents a DHCP/BootP packet
type Packet struct {
	OpCode    uint8    // 1 = BOOTREQUEST, 2 = BOOTREPLY
	HwType    uint8    // Hardware address type (1 = Ethernet)
	HwLen     uint8    // Hardware address length
	Hops      uint8    // Client sets to 0
	Xid       uint32   // Transaction ID
	Secs      uint16   // Seconds elapsed
	Flags     uint16   // Flags
	CIAddr    net.IP   // Client IP address
	YIAddr    net.IP   // Your IP address
	SIAddr    net.IP   // Server IP address
	GIAddr    net.IP   // Gateway IP address
	CHAddr    net.HardwareAddr // Client hardware address
	SName     [64]byte // Server host name
	File      [128]byte // Boot file name
	Options   map[byte][]byte // DHCP options
}

// ParsePacket parses a DHCP packet from bytes
func ParsePacket(data []byte) (*Packet, error) {
	if len(data) < 236 {
		return nil, errors.New("packet too small")
	}

	p := &Packet{
		OpCode: data[0],
		HwType: data[1],
		HwLen:  data[2],
		Hops:   data[3],
		Xid:    binary.BigEndian.Uint32(data[4:8]),
		Secs:   binary.BigEndian.Uint16(data[8:10]),
		Flags:  binary.BigEndian.Uint16(data[10:12]),
		CIAddr: net.IP(data[12:16]),
		YIAddr: net.IP(data[16:20]),
		SIAddr: net.IP(data[20:24]),
		GIAddr: net.IP(data[24:28]),
		CHAddr: net.HardwareAddr(data[28:28+data[2]]),
		Options: make(map[byte][]byte),
	}

	copy(p.SName[:], data[44:108])
	copy(p.File[:], data[108:236])

	// Parse options
	if len(data) > 236 {
		// Check for magic cookie
		if len(data) >= 240 && bytes.Equal(data[236:240], MagicCookie) {
			p.parseOptions(data[240:])
		}
	}

	return p, nil
}

// parseOptions parses DHCP options
func (p *Packet) parseOptions(data []byte) {
	i := 0
	for i < len(data) {
		option := data[i]
		if option == OptionEnd {
			break
		}
		if option == 0 { // Pad option
			i++
			continue
		}
		if i+1 >= len(data) {
			break
		}
		length := int(data[i+1])
		if i+2+length > len(data) {
			break
		}
		p.Options[option] = data[i+2 : i+2+length]
		i += 2 + length
	}
}

// ToBytes converts the packet to bytes
func (p *Packet) ToBytes() []byte {
	buf := make([]byte, 236)
	
	buf[0] = p.OpCode
	buf[1] = p.HwType
	buf[2] = p.HwLen
	buf[3] = p.Hops
	binary.BigEndian.PutUint32(buf[4:8], p.Xid)
	binary.BigEndian.PutUint16(buf[8:10], p.Secs)
	binary.BigEndian.PutUint16(buf[10:12], p.Flags)
	
	copy(buf[12:16], p.CIAddr.To4())
	copy(buf[16:20], p.YIAddr.To4())
	copy(buf[20:24], p.SIAddr.To4())
	copy(buf[24:28], p.GIAddr.To4())
	copy(buf[28:44], p.CHAddr)
	copy(buf[44:108], p.SName[:])
	copy(buf[108:236], p.File[:])
	
	// Add magic cookie and options
	buf = append(buf, MagicCookie...)
	
	// Add options
	for opt, val := range p.Options {
		buf = append(buf, opt)
		buf = append(buf, byte(len(val)))
		buf = append(buf, val...)
	}
	
	// End option
	buf = append(buf, OptionEnd)
	
	// Pad to minimum 300 bytes
	for len(buf) < 300 {
		buf = append(buf, 0)
	}
	
	return buf
}

// GetMessageType returns the DHCP message type
func (p *Packet) GetMessageType() (uint8, error) {
	if msgType, ok := p.Options[OptionMessageType]; ok && len(msgType) == 1 {
		return msgType[0], nil
	}
	return 0, errors.New("no message type found")
}

// GetRequestedIP returns the requested IP address
func (p *Packet) GetRequestedIP() net.IP {
	if ip, ok := p.Options[OptionRequestedIPAddress]; ok && len(ip) == 4 {
		return net.IP(ip)
	}
	return nil
}

// GetServerIdentifier returns the server identifier
func (p *Packet) GetServerIdentifier() net.IP {
	if ip, ok := p.Options[OptionServerIdentifier]; ok && len(ip) == 4 {
		return net.IP(ip)
	}
	return nil
}

// SetMessageType sets the DHCP message type
func (p *Packet) SetMessageType(msgType uint8) {
	p.Options[OptionMessageType] = []byte{msgType}
}

// SetLeaseTime sets the lease time in seconds
func (p *Packet) SetLeaseTime(seconds uint32) {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, seconds)
	p.Options[OptionLeaseTime] = buf
}

// SetServerIdentifier sets the server identifier
func (p *Packet) SetServerIdentifier(ip net.IP) {
	p.Options[OptionServerIdentifier] = ip.To4()
}

// SetSubnetMask sets the subnet mask
func (p *Packet) SetSubnetMask(mask net.IPMask) {
	p.Options[OptionSubnetMask] = []byte(mask)
}

// SetRouter sets the router (gateway)
func (p *Packet) SetRouter(routers []net.IP) {
	var buf []byte
	for _, r := range routers {
		buf = append(buf, r.To4()...)
	}
	p.Options[OptionRouter] = buf
}

// SetDNS sets the DNS servers
func (p *Packet) SetDNS(servers []net.IP) {
	var buf []byte
	for _, s := range servers {
		buf = append(buf, s.To4()...)
	}
	p.Options[OptionDNS] = buf
}

// SetDomainName sets the domain name
func (p *Packet) SetDomainName(domain string) {
	p.Options[OptionDomainName] = []byte(domain)
}

// SetNTPServers sets NTP servers
func (p *Packet) SetNTPServers(servers []net.IP) {
	var buf []byte
	for _, s := range servers {
		buf = append(buf, s.To4()...)
	}
	p.Options[OptionNTPServers] = buf
}

// SetNetBIOSNameServers sets NetBIOS name servers
func (p *Packet) SetNetBIOSNameServers(servers []net.IP) {
	var buf []byte
	for _, s := range servers {
		buf = append(buf, s.To4()...)
	}
	p.Options[OptionNetBIOSNameServers] = buf
}

// SetNetBIOSDDServers sets NetBIOS Datagram Distribution servers
func (p *Packet) SetNetBIOSDDServers(servers []net.IP) {
	var buf []byte
	for _, s := range servers {
		buf = append(buf, s.To4()...)
	}
	p.Options[OptionNetBIOSDDServer] = buf
}

// SetNetBIOSNodeType sets the NetBIOS node type
func (p *Packet) SetNetBIOSNodeType(nodeType uint8) {
	p.Options[OptionNetBIOSNodeType] = []byte{nodeType}
}

// SetInterfaceMTU sets the interface MTU
func (p *Packet) SetInterfaceMTU(mtu uint16) {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, mtu)
	p.Options[OptionInterfaceMTU] = buf
}

// MACAddressString returns the MAC address as a string
func (p *Packet) MACAddressString() string {
	return p.CHAddr.String()
}

// MessageTypeName returns the name of the message type
func MessageTypeName(msgType uint8) string {
	switch msgType {
	case DHCPDiscover:
		return "DHCPDISCOVER"
	case DHCPOffer:
		return "DHCPOFFER"
	case DHCPRequest:
		return "DHCPREQUEST"
	case DHCPDecline:
		return "DHCPDECLINE"
	case DHCPAck:
		return "DHCPACK"
	case DHCPNak:
		return "DHCPNAK"
	case DHCPRelease:
		return "DHCPRELEASE"
	case DHCPInform:
		return "DHCPINFORM"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", msgType)
	}
}

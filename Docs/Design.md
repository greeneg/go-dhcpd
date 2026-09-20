# Go-DHCPd - A Modern DHCP / BootP Daemon Written in Golang

## Introduction

The `go-dhcpd` project intends to write a DHCP / BootP daemon in Golang. This daemon will have the following features:

- IPv4 dynamic address allocation for unregistered systems
- IPv4 static address assignment and allocation for registered systems
- Adherance to the RFCs for DHCP and BootP
- Subnet definitions using network and netmask definitions
  - The configuration for subnets must allow defining the dynamic allocation ranges using CIDR syntax
  - Allow enabling or disabling BootP on the subnet range
- Support for the following DHCP protocol options:
  - Setting the global and per-subnet netbios-node-type
  - Setting the per-subnet domain-name-servers
  - Setitng the per-subnet domain-name
  - Setting the per-subnet domain-search list
  - Setting the per-subnet interface-mtu
  - Setting the per-subnet ntp-server
  - Setting the per-subnet time-server
  - Setting the per-subnet smtp-server
  - Setting the per-subnet lpr-server
  - Setting the per-subnet netbios-name-servers
  - Setting the per-subnet netbios-dd-server
  - Setting the per-subnet routers
- Registered "static" addresses using the device's hardware MAC address and either direct IP address listing or the fully-qualified hostname associated with an IP address
- BootP support adhering to the RFCs for BootP
- JSON5 configuration files
- Address allocations are stored in a SQLite3 database
  - Two main tables:
    - Leases table: Allocations and lease times
    - DenyAddresses table: All martian addresses that are disallowed for allocation
- During address allocation a ping check is attempted to ensure that the address is not already in use on the network.
  - If already in use, the address is marked as a martian and disallowed from use in the database. If possible, attempt to retrieve the MAC address of the host that is squatting on a network address and issue a syslog record in the NOTICE category to notify system operators

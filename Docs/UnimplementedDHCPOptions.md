# Unimplemented DHCP Options

This document tracks which DHCP options (RFC 2132 and later RFCs) go-dhcpd
does **not** implement, so operators and contributors can tell at a glance
whether a given client/network requirement is supported. It is derived
directly from `internal/dhcp/packet.go`/`server.go`, not from the feature
list in the README, so it should stay accurate even if marketing copy
drifts.

See the [Supported DHCP Options](../README.md#supported-dhcp-options)
list in the README for what *is* sent today, and
[go-dhcpd-config.json5(5)](../man/man5/go-dhcpd-config.json5.5) for how to
configure each of those.

## Options defined in code but not fully functional

These have a named constant in `internal/dhcp/packet.go`, but the server
never acts on them the way RFC 2131/2132 describes. Listed here because
"grep finds the option number" is not the same as "the option works":

| # | Option | Current behavior | Gap |
|---|--------|-------------------|-----|
| 12 | Host Name | Read from inbound requests (used to seed the lease's stored hostname) | Never sent back to the client in OFFER/ACK; a server-assigned or client-overridden hostname is never communicated to the client itself |
| 28 | Broadcast Address | Constant defined | Never set on any outgoing packet; no `SetBroadcastAddress` exists |
| 55 | Parameter Request List | Constant defined | Never read; the server always sends every option it has configured for the subnet regardless of what the client actually asked for (harmless per RFC, but means PRL-based minimal responses aren't possible) |
| 58 | Renewal Time (T1) | Constant defined | Never sent; clients fall back to their own default (typically 50% of the lease time per RFC 2131 §4.4.5), which may not match an administrator's intended renewal schedule |
| 59 | Rebinding Time (T2) | Constant defined | Never sent; same fallback caveat as T1 above (typically 87.5% of lease time) |
| 61 | Client Identifier | Constant defined | Never read anywhere; all lease/static-host matching is done purely by `chaddr` (MAC address). Clients that send a Client Identifier different from their MAC (uncommon, but legal) will not be matched against a static reservation keyed to that identifier |

## Options not implemented at all

No constant, no parsing, no encoding. Grouped by why they'd matter if you
needed them.

### High-value / commonly requested

- **66 (TFTP Server Name) / 67 (Bootfile Name)** — the *DHCP* equivalents
  of BOOTP's `sname`/`file` fields. go-dhcpd's `next_server`/
  `server_hostname`/`boot_filename` subnet settings only populate the
  legacy fixed-size BOOTP packet fields (used by the BootP code path, see
  [README#bootp-support](../README.md#bootp-support)); a PXE client
  negotiating over plain DHCP (not BootP) and expecting options 66/67
  instead of the fixed fields will not receive this information today.
- **82 (Relay Agent Information)** — commonly inserted by relay agents
  (circuit-id/remote-id sub-options) for policy decisions and is normally
  echoed back by the server. Relevant to the planned `dhcp-relayd` work in
  [TODO.md](TODO.md); not implemented on either side yet.
- **121 (Classless Static Routes, RFC 3442)** — the modern replacement for
  option 33 (Static Route); widely relied upon for multi-route subnets.
  Not implemented; only a single default gateway list via option 3
  (Router) is supported.
- **93 (Client System Architecture) / 97 (UUID/GUID-based Client
  Identifier)** — used by PXE clients to identify firmware architecture;
  relevant if you want architecture-specific `boot_filename` values (e.g.
  different bootloaders for BIOS vs. UEFI clients). Not implemented.
- **43 (Vendor-Specific Information) / 60 (Vendor Class Identifier)** —
  used heavily by VoIP phones, PXE firmware, and other vendor-specific
  provisioning workflows. Not implemented.
- **81 (Client FQDN, RFC 4702)** — modern alternative to option 12 for
  clients that want to negotiate DNS updates with the server. Not
  implemented (go-dhcpd does not perform dynamic DNS updates at all).
- **80 (Rapid Commit, RFC 4039)** — two-message DISCOVER/ACK exchange
  instead of the full four-message DORA handshake. Not implemented.

### Less commonly needed, listed for completeness

Time/offset and legacy host-table options (2 Time Offset, 19 IP
Forwarding, 20 Non-Local Source Routing, 21 Policy Filter, 22 Max Datagram
Reassembly, 23 Default IP TTL, 24 Path MTU Aging Timeout, 25 Path MTU
Plateau Table, 27 All Subnets Local, 29-32 mask/router discovery, 34
Trailer Encapsulation, 35 ARP Cache Timeout, 36 Ethernet Encapsulation,
37-39 TCP keepalive/TTL options), legacy name/directory services (5 IEN
116 Name Server, 7 Log Server, 8 Cookie Server, 10 Impress Server, 11
Resource Location Server, 40 NIS Domain, 41 NIS Servers, 47 NetBIOS Scope,
48-49 X Window options, 64-65 NIS+, 70-76 application servers such as
POP3/NNTP/WWW/Finger/IRC/StreetTalk, 78-79 SLP, 85-87 NDS, 95 LDAP, 100-101
timezone codes, 112-114 NetInfo/URL, 120 SIP Servers), and newer/niche
options (13-14, 16-18 boot/swap/root-path, 33 legacy Static Route, 52
Option Overload, 56 Message, 57 Max DHCP Message Size, 62-63 NetWare/IP,
68 Mobile IP Home Agent, 77 User Class, 83 iSNS, 88-91 authentication/last-
transaction options, 98 User-Auth, 116-118 auto-config/name-service-search/
subnet-selection, 125 Vendor-Identifying Vendor-Specific, 145 Forcenew
Nonce, 150 TFTP Server Address, and PXE-specific 208-213) are not
implemented. Consult the [IANA BOOTP/DHCP Parameters
registry](https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml)
for the authoritative option list if you need one not mentioned above.

## Non-option protocol gaps

Not DHCP *options* per se, but related limitations worth noting alongside
this list:

- **DHCPINFORM** is handled, but the ACK it sends goes through the same
  `addSubnetOptions` path as a normal lease ACK; it does not specifically
  trim lease-related options (51/58/59) the way RFC 2131 §4.3.5 expects
  for INFORM replies, since those aren't sent at all today (see the table
  above).
- **Option overload (option 52)** and using the `sname`/`file` fixed
  fields to carry additional DHCP options when the options area is full
  are both unimplemented; large option sets could theoretically exceed
  the packet's practical size without using this mechanism.
- IPv6 (DHCPv6) has an entirely separate, non-overlapping option registry;
  see the "DHCP for IPv6" entry in [TODO.md](TODO.md) rather than this
  document for that gap.

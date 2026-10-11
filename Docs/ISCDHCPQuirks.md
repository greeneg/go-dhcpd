# ISC DHCP Client-Specific Quirks and Workarounds

ISC dhcpd includes several configuration statements that exist specifically to
work around clients that misbehave or deviate from RFC 2131/RFC 1048. These
are documented in `server/dhcpd.conf.5` and implemented across `server/dhcp.c`,
`server/confpars.c`, and `server/stables.c` in the ISC DHCP source
(dhcp-4.4.3-P1).

| Statement | Quirk it works around |
|---|---|
| `always-broadcast` | Some DHCP/BOOTP clients don't set the broadcast flag bit in the packet header, so they never see a unicast reply before they have an IP. The man page explicitly notes "the Microsoft DHCP client is known not to have this problem, as are the OpenTransport and ISC DHCP clients" — implying the flag targets other, typically older/embedded, clients. |
| `always-reply-rfc1048` | Legacy BOOTP clients send non-RFC1048 requests but still expect RFC1048-style vendor-options replies. The server logs `(non-rfc1048)` when it detects this; the flag forces RFC1048-style responses anyway. |
| `check-secs-byte-order` | Some clients get the `secs` field's byte order backwards (should be network byte order). When enabled, the server detects the malformed value (high byte non-zero, low byte zero) and swaps it — mainly relevant to failover load-balancing, which relies on `secs` being correct. |
| `echo-client-id` | RFC 6842 compliance: some clients expect option 61 (client identifier) echoed back in ACK/NAK. Off by default specifically for backward compatibility with clients/environments that predate RFC 6842. |
| `ignore-client-uids` | Explicitly violates the spec to support dual-boot machines — e.g. a PC that dual-boots Windows and NetBSD/Linux on the same NIC/MAC but sends a different `dhcp-client-identifier` per OS. Without this, the server would treat each OS as a different "client." |
| `allow duplicates` / `deny duplicates` | Related to the above: controls whether a lease matching a host declaration by MAC address should evict *other* leases under different client-identifiers for that same MAC (again, for multi-OS-per-NIC machines). |
| `min-secs` | Exploits the client-reported `secs` field to make a secondary server intentionally ignore a client's first DHCPDISCOVER/REQUEST, letting a primary server answer first. Relies on (and works around variability in) how faithfully clients report elapsed time. |
| `use-lease-addr-for-default-route` | Explicitly documented as a Windows 95 workaround: "This supposedly causes Win95 machines to ARP for all IP addresses, which can be helpful if your router is configured for proxy ARP." The man page itself calls it not recommended since it breaks other clients. |
| `stash-agent-options` | Works around relay-agent-info options not being resent by clients during unicast RENEWING-state REQUESTs (since those bypass the relay agent that originally inserted the option). |
| `dynamic-bootp-lease-cutoff` / `dynamic-bootp-lease-length` | BOOTP (not DHCP) clients have no renewal concept and don't know their "lease" can expire, so ISC dhcpd normally gives them infinite leases — these let an admin impose an artificial expiration anyway. |
| `declines` (`allow`/`deny`/`ignore`) | Defends against buggy *or malicious* clients that spam DHCPDECLINE messages to exhaust the address pool (since a DECLINE normally makes the server abandon that address). |
| `one-lease-per-client` | Assumes (optimistically, per the man page's own caveat) that a client only remembers the lease in its most recent DHCPREQUEST and has "forgotten" any others — frees all other leases held by that client when enabled. |

A few of these (`ignore-client-uids`, `allow duplicates`,
`use-lease-addr-for-default-route`) are explicitly flagged in the man page
itself as protocol violations or "not recommended" — they exist purely
because real-world client behavior (particularly older Microsoft/Windows
clients and dual-boot PCs) didn't match the spec closely enough otherwise.

## go-dhcpd comparison

go-dhcpd currently implements none of these quirk-specific accommodations —
its packet handling in `internal/dhcp/server.go` follows the RFC 2131 happy
path without special-casing broadcast-bit bugs, non-RFC1048 BOOTP, `secs`
byte-order, or client-identifier edge cases.

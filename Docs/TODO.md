# TODOs

## Insecure output that may leak credentials in the future

- `a.config.Plugins` includes the arbitrary settings map supplied to the
provider, and this unauthenticated `/config` endpoint now serializes it
verbatim. A database-backed provider can reasonably place credentials or
tokens in these settings, exposing them to any API caller. Return only
non-sensitive plugin metadata or redact provider settings before including
them in this response.

## DHCP Relay Agent (2.z beta feature)

Goal: a standalone `dhcp-relayd` binary implementing an RFC 1542 BOOTP/DHCP
relay agent, so `dhcpd` can serve clients on subnets that aren't on the
same broadcast domain as the server.

Relevant findings from the current codebase:

- `internal/dhcp/server.go`'s request handling already assumes a relay may
  be involved: it accepts requests with `GIAddr` set, copies `GIAddr` into
  every reply (`createOffer`/`createAck`/`sendNak`), and `sendPacket`
  already unicasts replies to `packet.GIAddr:67` when it's non-zero (RFC
  2131 §4.1). No server-side changes are needed for a single relay hop to
  work — only the relay agent binary itself is missing.
- `internal/dhcp/rawsocket.go`'s `RawSocket` is receive-only
  (`ReadDHCPPacket` via `Recvfrom`); there is no raw-socket send path. This
  is fine for a relay agent too: like the daemon's own `sendPacket`, it
  only ever needs to deliver replies via a UDP socket with `SO_BROADCAST`
  set (broadcast when the client has no IP yet or the broadcast flag is
  set, unicast to `ciaddr` otherwise) — it never needs to send a raw
  link-layer frame to a specific MAC address.
- `RawSocket`/`Packet`/`ParsePacket`/`ToBytes` live in `internal/dhcp` but
  are reusable as-is by a new `cmd/dhcp-relayd` binary without
  exporting/relocating anything, since internal-package visibility only
  restricts cross-module imports, not other packages within this module.

What `dhcp-relayd` needs to add:

- A raw-socket listener per client-facing interface (reusing
  `internal/dhcp.RawSocket`) that captures broadcast BOOTREQUEST/DHCP
  packets from clients.
- Forwarding logic (RFC 1542 §4.1): for a request with `giaddr == 0`,
  increment `hops`, set `giaddr` to the relay's own address on the
  receiving interface, and unicast it to each configured upstream server
  on port 67. Discard (and log) requests whose `hops` already exceeds a
  small threshold (RFC 1542 suggests 4-16) to prevent forwarding loops.
- A UDP/67 listener for the server's unicast reply, and reply delivery
  back toward the client on the originating interface, using the same
  broadcast-vs-unicast decision `internal/dhcp/server.go`'s `sendPacket`
  already makes — that logic should be factored out and shared rather
  than re-implemented.
- Configuration for one or more (client-interface -> upstream server
  list) mappings. Open design decision: a new top-level section in
  `config.json5` (documented in `go-dhcpd-config.json5(5)`) vs. repeatable
  CLI flags (ISC `dhcrelay`-style `-i`/`-s`). A config-file section fits
  this project's existing conventions better and more easily supports
  multiple interfaces/servers.
- Decide whether to support chained/multi-hop relays (`giaddr` already
  non-zero on receipt) — recommend not supporting this initially, and
  only relaying requests with `giaddr == 0`, to avoid RFC 1542's added
  multi-hop complexity.
- Packaging/docs to match existing conventions: `cmd/dhcp-relayd/main.go`,
  GNU-style flags matching `dhcpd`/`go-dhcpd-migration-tool`, a
  `dhcp-relayd.8` man page, a systemd unit, and `Makefile`/`go-dhcpd.spec`
  wiring.
- An integration test extending `tests/integration/` with a three-node
  topology (client / relay / server on separate networks); the existing
  harness only covers client+server on one shared network.

## DHCP for IPv6 (3.x feature)

This is not an incremental extension of the current server: DHCPv4 (RFC
2131) and DHCPv6 (RFC 8415) are different protocols on the wire, and the
codebase is IPv4-specific at nearly every layer, not just at the edges.

Where IPv4 is currently load-bearing (not just "the only thing tested"):

- `internal/dhcp/packet.go`'s `Packet` struct and its `ParsePacket`/
  `ToBytes` hard-code the DHCPv4/BOOTP fixed 236-byte header (op/htype/
  hlen/xid/secs/flags/ciaddr/yiaddr/siaddr/giaddr/chaddr/sname/file) and
  4-byte addresses (`.To4()` throughout). DHCPv6 has no equivalent
  fixed-field header at all — client/server messages are just a 1-byte
  message type + 3-byte transaction ID followed entirely by TLV options;
  relay messages (`RELAY-FORW`/`RELAY-REPL`) are a third, different format.
  None of this struct or its (de)serialization is reusable.
- `internal/dhcp/rawsocket.go` opens an `AF_PACKET`/`ETH_P_IP` (0x0800)
  socket specifically to capture broadcast traffic from address-less
  IPv4 clients, since a normal UDP socket can't receive from 0.0.0.0.
  DHCPv6 doesn't need this: clients already have a link-local address
  (via SLAAC) before ever sending DHCPv6 traffic, and the protocol uses
  UDP/547 multicast (`ff02::1:2`) instead of L2 broadcast — so the raw
  socket layer is simply not needed for v6, but a new multicast-socket
  listener is.
- Every DHCPv4 option constant and setter in `packet.go` (`OptionRouter`,
  `OptionDNS`, `OptionNTPServers`, etc.) uses the DHCPv4 option-number
  registry; DHCPv6 has its own, non-overlapping option numbers (e.g. DNS
  servers are option 23, not 6) carried as nested TLVs inside IA_NA/IA_TA/
  IA_PD options rather than flat top-level options.
- `internal/allocator/allocator.go`'s range allocation
  (`findAvailableIPInRange`) converts addresses to `uint32` to do range
  arithmetic; IPv6 addresses are 128 bits and need either `big.Int`-based
  arithmetic or prefix-based (not start/end-pair) range handling.
- `internal/sqlite3store/validate.go`'s `isIPv4` rejects anything that
  isn't IPv4 by design (see its doc comment), and every subnet/static-host
  field it validates assumes IPv4 literals; `config.SubnetConfig`'s shape
  (`network`/`netmask`/`dynamic_ranges`) doesn't fit IPv6 (prefixes use
  length, not a dotted netmask, and IPv6 allocation is more naturally
  IA_PD prefix delegation than a flat address range).
- Client identity: static reservations and lease lookups are keyed by
  MAC address end-to-end (`config.StaticHost.MACAddress`,
  `db.Lease.MACAddress`, `config.NormalizeMAC`). DHCPv6 identifies clients
  by DUID + IAID, not MAC; a DUID can be LL/LLT (link-layer-based, so a
  MAC is recoverable), EN (vendor-assigned), or UUID (not MAC-derived at
  all), so MAC can't remain the universal key.
- One partial exception: `internal/db/db.go`'s `leases`/`deny_addresses`
  tables store `ip_address`/`mac_address` as plain `TEXT`, so the lease
  storage layer itself isn't hard-coded to IPv4 formatting — the blocker
  there is the keying model (MAC-based), not the column types.

What adding IPv6 support actually requires:

- A new DHCPv6 wire-format package (message header + TLV option
  encode/decode, including nested IA_NA/IA_TA/IA_PD options) — not an
  extension of `internal/dhcp/packet.go`.
- A UDP/546+547 multicast listener (joining `ff02::1:2`), replacing the
  raw-socket broadcast-capture approach entirely for v6.
- A new or substantially generalized allocator: 128-bit address/prefix
  arithmetic, per-IA T1/T2 renewal timers (distinct from a single
  lease-time field), and IA_PD prefix-delegation pools alongside (or
  instead of) flat address ranges.
- A new config schema section (e.g. `subnets_v6`), new validation (can't
  reuse `isIPv4`), and new sqlite3store tables/plugin actions for v6
  subnets and DUID-keyed static reservations.
- A client-identity model built on DUID+IAID rather than MAC, threaded
  through the allocator, lease database, and static-host config/API.
- Decide the deployment model: a second listener inside `dhcpd` sharing
  the lease database, or a fully separate daemon (mirroring the
  `dhcp-relayd` split above) — given how little of the existing v4 code
  is reusable, a separate `internal/dhcpv6` package (and likely binary)
  is probably cleaner than threading v6 branches through `internal/dhcp`.
- New REST API endpoints (or extensions of existing ones) to expose v6
  leases/IA state, plus man-page and config-file documentation updates.

## Alternative Database Engine Support

There are two independent SQLite3 usages in this codebase, and they have
very different pluggability stories today:

1. **Config-provider storage** (subnets/static hosts) — already behind
   the plugin interface. `sqlite3-config` is just one implementation of
   `pluginapi`'s stdin/stdout protocol; a Postgres/MySQL-backed
   equivalent is architecturally just a new plugin binary, with no
   daemon-side changes needed (`internal/plugin/manager.go` and
   `internal/allocator` only ever talk to the `pluginapi.Request`/
   `Response` contract, never to SQL directly).
2. **Lease-tracking storage** (`internal/db`) — not pluggable at all.
   This is the real gap.

Findings specific to the lease-tracking database (`internal/db/db.go`):

- `db.Database` is a concrete struct wrapping `*sql.DB`, with no
  interface boundary. `cmd/dhcpd/main.go` constructs it directly via
  `db.NewDatabase(cfg.Global.DatabasePath)`, and both
  `internal/allocator.Allocator` and `internal/api.API` hold a
  `*db.Database` field and take one as a constructor argument — there is
  no `LeaseStore` interface for them to depend on instead.
- `NewDatabase`/`initialize` hard-code `sql.Open("sqlite3", path)` and a
  single `global.database_path` config field (a bare file path, not a
  driver-qualified DSN) — there's no "which engine" selector anywhere in
  `config.GlobalConfig`.
- The schema itself uses SQLite-specific SQL: `INTEGER PRIMARY KEY
  AUTOINCREMENT` (Postgres: `GENERATED ALWAYS AS IDENTITY`; MySQL:
  `AUTO_INCREMENT`), a `DATETIME` column type (Postgres has no such
  type; use `TIMESTAMP`), and `ON CONFLICT(ip_address) DO UPDATE SET`
  upsert syntax (MySQL instead uses `ON DUPLICATE KEY UPDATE`). None of
  the SQL in `db.go` is portable across engines as written.
- SQLite is an embedded, single-file, single-process-friendly engine;
  Postgres/MySQL are network services, so a new engine also needs
  connection-string/credential handling, connection pooling
  (`sql.DB.SetMaxOpenConns`/`SetConnMaxLifetime`), retry/backoff, and
  optionally TLS to the database — none of which today's code considers.
- `internal/sqlite3store` (used by both the `sqlite3-config` plugin and
  `go-dhcpd-migration-tool`) mixes together storage-agnostic validation
  (`ValidateSubnet`/`ValidateStaticHost`) with SQLite-specific
  schema/CRUD in one package. A new config-provider plugin for another
  engine would either have to duplicate that validation or import a
  package literally named "sqlite3store" just for it — this validation
  logic should be extracted into a storage-agnostic package (e.g.
  `internal/configvalidate`) so it isn't SQLite-coupled by name.

What adding another lease-storage engine requires:

- Define a `LeaseStore` interface from `db.Database`'s current method
  set (`AddLease`, `UpdateLease`, `GetLeaseByMAC`, `GetLeaseByIP`,
  `GetAllLeases`, `IsDenyAddress`, `AddDenyAddress`, etc.), and change
  `internal/allocator`/`internal/api` to depend on the interface instead
  of the concrete `*db.Database` type.
- Add an engine selector to `config.GlobalConfig` (e.g.
  `database_driver` alongside a driver-appropriate connection string),
  defaulting to today's `sqlite3`/bare-path behavior for backward
  compatibility.
- Per-engine schema definitions and a real migration/versioning strategy
  (today's ad hoc `CREATE TABLE IF NOT EXISTS` doesn't generalize once
  there's more than one dialect to keep in sync over time).
- New driver dependencies per engine (e.g. `jackc/pgx` for Postgres,
  `go-sql-driver/mysql` for MySQL), ideally gated behind Go build tags
  so a default build doesn't have to link every driver — the project
  already pays a cgo cost for `mattn/go-sqlite3` and `msteinert/pam/v2`
  and shouldn't grow that further by default.

## Dynamic DNS Updates

Goal: have the daemon create/update/remove forward (A) and reverse (PTR)
DNS records as leases are issued, renewed, and released, the way ISC
`dhcpd`'s `ddns-update-style` does.

Relevant findings from the current codebase (there is no DNS-update code
at all today, but these existing pieces are directly relevant):

- `internal/dhcp/server.go`'s `getHostname` reads option 12 (Host Name)
  verbatim into a Go string with **no validation at all** — no length
  limit, no check against RFC 1035 label syntax, no rejection of `.`/
  control characters. Feeding this straight into a DNS update (or even
  into a zone file) as-is would be a DNS-injection risk; it needs to be
  validated/sanitized before it can be used as part of an FQDN.
- `internal/dhcp/server.go`'s `leaseExpirationLoop` is a stub: it logs
  "Running lease expiration check" on a timer and does nothing else.
  `Database.ExpireLeases` already exists in `internal/db/db.go` but is
  never called here. Dynamic DNS needs to remove records when a lease
  expires or is released, so wiring expiration into the loop is a
  prerequisite bug fix, not just a DDNS nice-to-have.
- Per [Docs/UnimplementedDHCPOptions.md](UnimplementedDHCPOptions.md),
  option 81 (Client FQDN, RFC 4702) is not implemented. Without it, the
  server can't tell whether a client wants to do its own forward-DNS
  update, wants the server to do both forward and reverse, or is
  suggesting a name the server is free to override — DDNS would have to
  start in a "server always owns both records" mode rather than
  RFC 4702's full negotiation.
- `config.SubnetConfig.DomainName` and `config.StaticHost.Hostname`
  already exist and could supply the forward zone / node name, but
  there's no config for a DNS server address, update credentials, or
  which zone(s) to update.
- The Go standard library has no DNS UPDATE (RFC 2136) client; `net`'s
  resolver only does lookups. A new dependency (e.g. `miekg/dns`) would
  be needed for both constructing UPDATE messages and signing them.

What implementing this requires:

- An RFC 2136 DNS UPDATE client, authenticated via TSIG (RFC 2845) —
  per-zone (or per-server) key name/algorithm/secret configuration, most
  likely under a new `global.dynamic_dns` config section (server
  address, forward zone, reverse zone, TSIG key material, record TTL).
- Hostname sanitization for option 12 (and eventually validated/used
  for option 81 once that's implemented) before it's ever used to build
  an FQDN — reject or escape anything that isn't a valid RFC 1035 label.
- Record lifecycle hooks: create/update on `Allocator.CreateLease`,
  delete on `Allocator.ReleaseLease` and on actual lease expiration (see
  the `leaseExpirationLoop` gap above) — both the forward A record and
  the reverse PTR record (computed from the leased IP's `in-addr.arpa`
  name) need to move together.
- A conflict-resolution strategy for multiple clients claiming the same
  hostname — ISC `dhcpd` uses DHCID records (RFC 4701/4703) for this;
  without something equivalent, two clients requesting the same name
  would silently overwrite each other's DNS records.
- DNS updates must not block or fail lease issuance: do them
  asynchronously (e.g. a goroutine/queue) with logging and limited
  retry, similar in spirit to the existing ping-check pattern but
  without holding up the DHCPACK on DNS server reachability.
- Treat TSIG secrets as credentials from day one: the existing
  "Insecure output that may leak credentials" item above already flags
  `/config` serializing `plugins.settings` verbatim — any new DDNS
  config section needs the same redaction before it ships, not added as
  an afterthought.
- Man-page (`go-dhcpd-config.json5(5)`) and README documentation for the
  new config section once implemented.

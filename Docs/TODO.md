# TODOs

## Insecure output that may leak credentials in the future

- `a.config.Plugins` includes the arbitrary settings map supplied to the provider, and this unauthenticated `/config` endpoint now serializes it verbatim. A database-backed provider can reasonably place credentials or tokens in these settings, exposing them to any API caller. Return only non-sensitive plugin metadata or redact provider settings before including them in this response.

## BootP support (blocker for 2.x beta)

The daemon and its documentation (README, `Docs/Design.md`, `dhcpd.8`) claim BootP support, but it is not actually implemented. Found during code review against RFC 951 (BOOTP) and RFC 2131/1532 (DHCP-BOOTP interoperability):

- `handleBootP()` in `internal/dhcp/server.go` is a no-op stub: it logs and returns without building or sending any reply packet. No BootP client has ever received a response from this daemon.
- BootP request detection in `handlePacket()` (`internal/dhcp/server.go`) is just "DHCP message-type option (53) missing/unparseable", not an explicit check of `OpCode == BootRequest`. Any malformed/truncated DHCP packet lacking option 53 is currently misrouted into the BootP stub instead of being logged as an error.
- `config.SubnetConfig.EnableBootP` is defined, validated, and persisted (`internal/config/config.go`, `internal/sqlite3store/store.go`), but nothing in `internal/dhcp` ever reads it; there is no subnet-gating of BootP, and no code path even has subnet context to check it.
- No config schema exists for per-client/per-subnet boot server data (RFC 951 §3 requires a boot file name, and optionally a different boot server address/hostname, per client). `Packet.SName`/`Packet.File` (`internal/dhcp/packet.go`) are parsed from incoming packets but nothing ever populates them on a reply — no `filename`, `next_server`, or `server_hostname` fields exist on `config.SubnetConfig` or `config.StaticHost`.
- BootP must only ever serve statically pre-registered clients (RFC 951 §3); there's no static-only allocation path. `Allocator.AllocateIP()` (`internal/allocator/allocator.go`) falls back to the dynamic pool when no static entry matches, which is invalid for BootP once implemented.
- `createOffer`/`createAck` (`internal/dhcp/server.go`) unconditionally set DHCP-only options (message type 53, lease time 51) that must not appear in a BootP reply; a BootP-specific reply builder is needed rather than reusing these as-is.
- README's feature list, `Docs/Design.md`, and `dhcpd.8`'s NAME/DESCRIPTION all claim RFC-compliant BootP support; these should be corrected (or the above implemented) before advertising BootP support in a 2.x release.

## DHCP Relay Agent (2.z beta feature)

## DHCP for IPv6 (3.x feature)

## Alternative Database Engine Support

# Architecture

This document is the design source of truth for Zapret Mobile. Read it before
changing the engine or the VPN plumbing.

## Overview

```
engine/                    Go module -> app/libs/zapret.aar (gomobile bind)
  desync/                  the DPI-bypass core (pure, host-testable)
    tls.go, parser.go      minimal TLS ClientHello / SNI parser
    strategy.go            strategy string grammar + model
    segments.go            BuildSegments: turns the first payload into framed segments
    dialer.go              desync dialer + per-connection first-write rewrite + protect()
    relay.go               bidirectional copy with half-close
    socks5.go              SOCKS5 front-end (proxy-only mode; also unit-tested)
  tunnel/                  gVisor TUN <-> desync bridge
    tunnel.go              stack setup, TCP forwarder, lifecycle
    udp.go                 protected UDP forwarding
  mobile/                  gomobile-bound API (Engine, Protector, Logger, ValidateStrategy)

app/                       Android app (Kotlin + Compose)
  vpn/ZapretVpnService     VpnService: TUN + engine lifecycle + foreground notif
  vpn/ZapretTileService    Quick Settings tile
  vpn/BootReceiver         optional start-on-boot
  vpn/VpnState             process-wide StateFlow shared with the UI
  engine/EngineCallbacks   Protector/Logger impls (kept by ProGuard; called from native)
  data/                    DataStore settings + strategy presets
  ui/                      Compose screen + ViewModel + theme
```

## Why this shape

- **One toolchain, one artifact.** The whole engine (desync + a full TCP/IP
  stack) is Go, compiled to a single `.aar` with `gomobile bind`. No NDK/C glue
  in the app, no second build system.
- **The interesting code is host-testable.** `desync/` has no Android or gVisor
  dependency, so `go test ./desync` exercises the real fragmentation logic on the
  dev machine. `tunnel/` is validated with an in-memory two-stack integration
  test that drives a real TCP flow through the forwarder to a real echo server.

## The desync model (`desync/segments.go`)

The engine only ever rewrites the **first application write** of a connection
(the TLS ClientHello, or the HTTP request). `BuildSegments(data, strategy)`
returns an ordered list of `Segment{Data, OOB}`. Invariants:

- The concatenation of all **non-OOB** segment data equals a byte stream the peer
  reassembles to the *original* request. We never change what the server sees —
  only how it is framed. This is what makes every method **server-safe**.
- `tlsrec` rewrites one TLS handshake record into two valid records (correct
  2-byte lengths) split inside the SNI. TLS explicitly allows a handshake message
  to span records, so all servers accept it; a DPI that parses only the first
  record sees a truncated SNI.
- `split` adds a TCP-segment boundary (a separate `Write`, with `TCP_NODELAY`
  set). `oob` injects one *extra* urgent byte that the peer drops (default
  `SO_OOBINLINE` off), so the in-band stream stays intact.

### Why no TTL "fake" / "disorder"

On a connected stream socket you cannot rewind the send sequence number without
`TCP_REPAIR` (needs `CAP_NET_ADMIN` = root) or inject raw IP packets (needs root
and arbitrary egress, which Android does not allow an app). So zapret's
fake-packet-with-low-TTL and true segment reordering **cannot** be done from an
unprivileged app without corrupting the TCP stream. We implement only the
stream-level, server-safe subset. If this ever runs with elevated privileges,
those methods could be added behind a capability check — not before.

## The tunnel (`tunnel/tunnel.go`)

- A gVisor `stack.Stack` with IPv4/IPv6 + TCP/UDP/ICMP, NIC backed by an
  `fdbased` endpoint on a **dup** of the `VpnService` TUN fd (so our lifetime is
  independent of the caller's `ParcelFileDescriptor`).
- Promiscuous mode + spoofing + default routes for both families, so the stack
  accepts packets for any destination we routed into the TUN.
- **TCP**: a `tcp.Forwarder` intercepts each flow, wraps it as a `gonet.TCPConn`,
  and relays it to a `desync.Dialer.DialContext(originalDst)` — the dial applies
  the strategy on the first write and is `protect()`ed.
- **UDP**: a `udp.Forwarder` relays datagrams through a `protect()`ed socket, with
  an option to drop UDP/443 so QUIC falls back to TCP TLS (where desync applies).

### protect() and loop avoidance

The engine's own upstream sockets must not re-enter the TUN. Two defenses:
`VpnService.protect(fd)` on every dialed socket (via the dialer's `Control`), and
`addDisallowedApplication(ourPackage)` on the VPN so the whole app process is
excluded. The `Protector` is a gomobile interface implemented in Kotlin.

## Concurrency note (don't reintroduce the deadlock)

`mobile.Engine` uses **two** locks on purpose: `mu` guards the running components
(`tun`, `socksLn`); `cbMu` guards the callbacks (`logger`, `protector`). `logf`
and the protector are read under `cbMu` only, so the engine can hold `mu` across
`tunnel.Start` — which logs through `logf` — without a self-deadlock. An earlier
single-mutex version wedged the start goroutine *after* forwarding was already
running (traffic flowed but `StartTunnel` never returned). `mobile/api_test.go`
guards against the regression.

## Android lifecycle notes

- `VpnService` runs a foreground service (`specialUse` FGS type on Android 14+)
  with an ongoing notification; `POST_NOTIFICATIONS` is requested at first start.
- VPN consent (`VpnService.prepare`) is an Activity result; the tile routes
  through the Activity when consent is not yet granted.
- `VpnState` is a singleton `StateFlow` the service writes and the UI observes —
  simple and sufficient; no bound-service IPC.

## Emulator testing gotcha

In the emulator, the host alias `10.0.2.2` is on the VM's own `eth0` subnet, so
the OS sends it on-link and it **bypasses** `tun0`. `scripts/e2e.sh` therefore
targets the host's **LAN IP** (off-subnet), which routes through the tunnel like
any real destination. Real-world destinations are never on the phone's subnet, so
this is purely a test artifact.

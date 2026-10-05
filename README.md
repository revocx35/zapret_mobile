# Zapret Mobile

A **no-root DPI-bypass VPN for Android**. It runs entirely on your phone: a local
`VpnService` captures your traffic and re-frames the TLS ClientHello and HTTP
request so that censorship [Deep Packet Inspection](https://en.wikipedia.org/wiki/Deep_packet_inspection)
can no longer match the domain you are visiting. Your traffic still goes
**directly to each website** — there is no remote proxy, nothing is decrypted,
and no account is required.

It is a mobile relative of desktop [zapret](https://github.com/bol-van/zapret),
[GoodbyeDPI](https://github.com/ValdikSS/GoodbyeDPI) and
[ByeDPI](https://github.com/hufrea/byedpi), using the subset of their techniques
that works **without root** on a stock Android phone.

> [!IMPORTANT]
> This is a tool for bypassing network censorship and accessing information. Use
> it in accordance with your local laws. It does not hide your IP address and is
> not an anonymity tool — it is not a substitute for Tor or a trusted VPN.

## How it works

```
         ┌─────────────────────────── your phone ───────────────────────────┐
 apps ──▶│  Android VpnService (TUN)  ──▶  Go engine (one .aar)              │──▶ internet
         │                                 ├─ gVisor userspace TCP/IP stack  │    (direct,
         │                                 ├─ desync dialer (per connection) │     no proxy)
         │                                 └─ protect() keeps egress off-VPN │
         └───────────────────────────────────────────────────────────────────┘
```

1. The app asks for the system VPN permission and opens a **TUN** interface with
   `VpnService` (no root needed — this is a normal Android API).
2. A [gVisor](https://gvisor.dev/) userspace network stack reads IP packets from
   the TUN and turns each TCP flow into a connection the engine can handle.
3. For every outgoing connection, the **desync dialer** re-frames the first
   payload (the TLS ClientHello or the HTTP request) and dials the real
   destination through a `protect()`ed socket so it leaves the phone normally.

The result the server reassembles is **byte-for-byte identical** to your original
request. Only the *framing* changes — which is exactly what a DPI box that keys
on a contiguous SNI / `Host` field fails to follow.

### What works without root — and what doesn't

Desktop zapret's most famous trick is sending **fake packets with a low TTL** so
the censor sees them but the server never does. That (and true segment
reordering) needs raw sockets / `TCP_REPAIR`, i.e. **root**, and would corrupt
the stream if attempted from an unprivileged app. Zapret Mobile therefore
implements only the **stream-safe** methods, which need no special privilege and
are what tools like [SpoofDPI](https://github.com/xvzc/SpoofDPI),
[GreenTunnel](https://github.com/SadeghHayeri/GreenTunnel) and
[PowerTunnel](https://github.com/krlvm/PowerTunnel) rely on:

| Method | What it does | Notes |
|---|---|---|
| `tlsrec` | Splits the TLS ClientHello into two **TLS records** across the SNI | Core method, always server-safe |
| `split` | Splits the first write into two **TCP segments** | Composes with `tlsrec` |
| `hostsplit` | Splits a plaintext HTTP request inside the `Host` header | For port-80 traffic |
| `oob` | Injects a TCP urgent (out-of-band) byte at a boundary | Experimental; a few servers mishandle urgent data |

Methods requiring root (TTL fakes, disorder/retransmit) are **intentionally
omitted**; see [`ARCHITECTURE.md`](ARCHITECTURE.md).

## Strategy grammar

A strategy is a comma-separated list of steps applied to the first payload, with
an optional inter-segment delay:

```
strategy := step ("," step)*  ("@delay=" <ms>)?
step     := method (":" position)?
method   := tlsrec | split | hostsplit | oob | none
position := <n> | sni[±n] | host[±n]
```

Examples:

```
tlsrec:sni                 # split the ClientHello record at the SNI (default-ish)
tlsrec:sni,split:1         # also force a separate first TCP segment
tlsrec:sni-1,split:2 @delay=15
split:3                    # lightest touch: just split the first TCP segment
```

The in-app presets cover the common cases; pick **Custom** to type your own.
Validation is live (the Go parser is called directly through the bound engine).

## Features

- No root, no remote server, no sign-up.
- Start/stop from the app, a **Quick Settings tile**, or automatically **on boot**.
- Per-connection DPI desync with selectable presets or a custom strategy.
- Optional **Block QUIC (UDP/443)** so browsers fall back to TCP TLS, where the
  desync applies.
- Material 3 UI with a live log; dynamic color on Android 12+.

## Building

The engine is a Go module compiled to a single `.aar`; the app is a normal
Gradle/Kotlin project that depends on it.

```bash
# 1. Build the engine (needs Go, the Android NDK and gomobile)
scripts/build-engine.sh          # -> app/libs/zapret.aar

# 2. Build the app
./gradlew :app:assembleDebug     # app/build/outputs/apk/debug/app-debug.apk
```

Requirements: JDK 17+, Android SDK (`compileSdk 37`), NDK r27, Go 1.24+ and
`golang.org/x/mobile/cmd/gomobile`. `local.properties` must point `sdk.dir` at
your SDK. See [`CLAUDE.md`](CLAUDE.md) for the exact environment used here.

## Testing

```bash
# Engine unit + integration tests (desync correctness, SOCKS, full gVisor datapath)
cd engine && go test ./...

# Full on-device end-to-end on an emulator (needs /dev/kvm):
scripts/emulator.sh start
scripts/e2e.sh
```

`scripts/e2e.sh` installs the app on a headless emulator, starts the VPN, then
from inside the emulator (so traffic traverses the VPN) it (1) completes a real
TLS handshake to `1.1.1.1` through the tunnel and (2) connects to a host-side
capture server and asserts the ClientHello arrived split across two TLS records
with the SNI no longer contiguous — i.e. the desync really happens on the wire.

## Credits

Inspired by [zapret](https://github.com/bol-van/zapret),
[ByeDPI](https://github.com/hufrea/byedpi),
[SpoofDPI](https://github.com/xvzc/SpoofDPI) and
[PowerTunnel](https://github.com/krlvm/PowerTunnel). Built on
[gVisor](https://gvisor.dev/)'s `netstack`.

## License

MIT — see [`LICENSE`](LICENSE). Bundled third-party components keep their own
licenses (gVisor: Apache-2.0; `golang.org/x/sys`: BSD-3-Clause).

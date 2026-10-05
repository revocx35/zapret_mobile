# Zapret Mobile: notes for Claude sessions

No-root DPI-bypass VPN for Android. **Read `ARCHITECTURE.md` first** — it is the
design source of truth (desync model, the two-lock concurrency rule, the gVisor
tunnel, and the emulator routing gotcha).

## Layout
- `engine/` — Go module, the DPI-bypass engine, compiled to `app/libs/zapret.aar`
  via gomobile. `desync/` is the host-testable core; `tunnel/` is the gVisor
  TUN↔desync bridge; `mobile/` is the bound API (`Engine`, `Protector`, `Logger`,
  `ValidateStrategy`). Module path `github.com/revocx35/zapret_mobile/engine`.
- `app/` — Kotlin + Compose app (`VpnService`, QS tile, boot receiver, UI).

## Commands
- Engine tests (fast, no Android): `cd engine && go test ./...`. Covers desync
  correctness (server-safe fragmentation, SNI split), SOCKS end-to-end, the full
  gVisor datapath (`tunnel_test.go`), and the start-up deadlock regression
  (`mobile/api_test.go`). Go is at `/usr/local/go/bin` (not on PATH by default).
- Build the engine .aar: `scripts/build-engine.sh` (needs Go, NDK r27, gomobile;
  `gomobile`/`gobind` via `go install golang.org/x/mobile/cmd/...@latest` into
  `/root/go/bin`). Targets arm64 + amd64 + arm, `-javapkg io.github.revocx35`.
- Build the app: `./gradlew :app:assembleDebug` (JDK 21 here; SDK `/opt/android-sdk`
  via `local.properties`). The .aar must exist first — the app depends on
  `files("libs/zapret.aar")`.
- On-device e2e (needs `/dev/kvm`): `scripts/emulator.sh start` then
  `scripts/e2e.sh`. It rebuilds the APK, pre-authorises the VPN via
  `appops set <pkg> ACTIVATE_VPN allow`, auto-starts the tunnel, then proves
  (1) a real TLS handshake to 1.1.1.1 completes through the tunnel and (2) the
  ClientHello arrives split across 2 TLS records with the SNI non-contiguous.
  Takes a few minutes — run it in the background and poll the log (the Bash tool
  kills foreground commands at ~120 s).

## Rules / gotchas
- **Server-safe only.** Every desync method must leave the bytes the peer
  reassembles identical to the original request (OOB bytes are the one exception —
  they are extra and dropped by the peer). No TTL-fake / disorder: they need root
  and would corrupt the stream (see ARCHITECTURE.md).
- **Two locks in `mobile.Engine`** (`mu` for state, `cbMu` for callbacks). Never
  call `logf` or the protector while holding `mu` under a single lock — that
  deadlocked start-up once (tunnel forwarded traffic but `StartTunnel` never
  returned). `mobile/api_test.go` guards it.
- gomobile maps Go `int` → Java `long`: `Protector.protect(fd: Long)`,
  `startTunnel(fd: Long, mtu: Long, ...)`.
- ProGuard must keep `go.**`, `io.github.revocx35.zapret.**` and
  `io.github.revocx35.zapretmobile.engine.**` (called from native). Release uses
  R8 with `-dontobfuscate`.
- Emulator: `10.0.2.2` is on the VM's own subnet and bypasses `tun0`; the e2e
  targets the **host LAN IP** instead. Don't use `pkill -f <pattern>` where the
  pattern also appears in your own command line (self-kill); free the capture port
  with `fuser -k 18443/tcp`.
- `app/libs/*.aar` is a build artifact and is git-ignored; CI rebuilds it before
  assembling the app.
- Don't commit real home addresses: the e2e discovers the LAN IP at runtime.

## Releasing
Bump `versionCode`/`versionName` in `app/build.gradle.kts`, tag `vX.Y.Z`, push.
The `release` CI job builds the engine, signs with the `RELEASE_*` repo secrets,
and attaches the APK. The signing keystore lives outside the repo; its location is
in Claude's project memory. Never commit it or `keystore.properties`.

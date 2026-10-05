#!/usr/bin/env bash
# End-to-end test of the real Android VPN datapath on the emulator.
#
# It installs the debug app, pre-authorises the VPN (appops) so no dialog is
# needed, starts the tunnel, then from *inside* the emulator (traffic therefore
# traverses the VPN):
#   1. connects to a real site (1.1.1.1:443) and completes a full TLS handshake
#      + HTTP GET  -> proves normal traffic works through the engine, and
#   2. connects to a host-side capture "DPI" server and we verify the engine
#      fragmented the TLS ClientHello across TLS records at the SNI.
#
# Requires a booted emulator (scripts/emulator.sh start) and /dev/kvm.
set -euo pipefail
cd "$(dirname "$0")/.."

SDK="${ANDROID_HOME:-/opt/android-sdk}"
ADB="$SDK/platform-tools/adb"
GO="${GO:-/usr/local/go/bin/go}"
PKG=io.github.revocx35.zapretmobile.debug
ACT=io.github.revocx35.zapretmobile/.ui.MainActivity   # namespace activity alias
ACT_FULL=$PKG/io.github.revocx35.zapretmobile.ui.MainActivity
APK=app/build/outputs/apk/debug/app-debug.apk
PORT=18443
# The emulator reaches the host via its LAN IP (an off-subnet address, so the
# probe routes through tun0 -> engine). 10.0.2.2 is on the emulator's own subnet
# and would bypass the tunnel.
HOST_IP="${HOST_IP:-$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{print $7; exit}')}"
TMP="${TMPDIR:-/tmp}/zapret-e2e"
mkdir -p "$TMP"
VERDICT="$TMP/verdict.json"
rm -f "$VERDICT"

echo "== build debug APK =="
JAVA_HOME="${JAVA_HOME:-$(dirname "$(dirname "$(readlink -f "$(command -v java)")")")}" \
  ./gradlew :app:assembleDebug --console=plain -q

echo "== build static test client =="
( cd scripts/dpitest && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -o "$TMP/dpitest" . )

echo "== install app =="
"$ADB" wait-for-device
"$ADB" uninstall "$PKG" >/dev/null 2>&1 || true
"$ADB" install -r -g "$APK" >/dev/null

echo "== pre-authorise VPN + notifications =="
"$ADB" shell appops set "$PKG" ACTIVATE_VPN allow
"$ADB" shell pm grant "$PKG" android.permission.POST_NOTIFICATIONS 2>/dev/null || true

echo "== start host capture server on :$PORT =="
python3 scripts/dpi_capture.py "$PORT" "$VERDICT" blocked.example &
CAP=$!
trap 'kill $CAP 2>/dev/null || true' EXIT
sleep 1

echo "== launch app (auto-start the VPN) =="
"$ADB" logcat -c
"$ADB" shell am start -n "$ACT_FULL" --ez auto_start true >/dev/null

echo "== wait for tunnel to connect =="
connected=0
for _ in $(seq 1 30); do
  if "$ADB" logcat -d -s ZapretVpn 2>/dev/null | grep -q "connected strategy="; then
    connected=1
    break
  fi
  sleep 1
done
if [ "$connected" != 1 ]; then
  echo "FAIL: tunnel did not connect" >&2
  "$ADB" logcat -d -s ZapretVpn >&2 || true
  exit 1
fi
"$ADB" logcat -d -s ZapretVpn | grep "connected strategy=" | tail -1

echo "== push test client into emulator =="
"$ADB" push "$TMP/dpitest" /data/local/tmp/dpitest >/dev/null
"$ADB" shell chmod 755 /data/local/tmp/dpitest

echo "== [1] connectivity check: real TLS to 1.1.1.1 through the VPN =="
CONN_OUT="$("$ADB" shell /data/local/tmp/dpitest conn 1.1.1.1:443 one.one.one.one | tr -d '\r')"
echo "   $CONN_OUT"

echo "== [2] fragmentation check: ClientHello to host capture server ($HOST_IP:$PORT) =="
FRAG_OUT="$("$ADB" shell /data/local/tmp/dpitest frag "$HOST_IP:$PORT" blocked.example | tr -d '\r')"
echo "   $FRAG_OUT"

echo "== wait for capture verdict =="
wait $CAP || true
trap - EXIT
echo "-- verdict --"
cat "$VERDICT"; echo

echo "== stop VPN =="
"$ADB" shell am start -n "$ACT_FULL" >/dev/null 2>&1 || true
"$ADB" shell am force-stop "$PKG" || true

echo "== assertions =="
fail=0
echo "$CONN_OUT" | grep -q "handshake-ok" || { echo "FAIL: real site handshake did not complete through the VPN"; fail=1; }
python3 - "$VERDICT" <<'PY' || fail=1
import json, sys
v = json.load(open(sys.argv[1]))
assert v.get("ok"), f"capture failed: {v}"
assert v["handshake_record_count"] >= 2, f"ClientHello not fragmented into >=2 records: {v}"
assert not v["sni_whole_in_first_record"], f"SNI still whole in first record: {v}"
assert v["fragmented"], f"not fragmented: {v}"
print("  OK: ClientHello split into", v["handshake_record_count"],
      "records; SNI not contiguous in the first record")
PY

if [ "$fail" = 0 ]; then
  echo "E2E PASSED"
else
  echo "E2E FAILED" >&2
  exit 1
fi

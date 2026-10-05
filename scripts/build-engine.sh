#!/usr/bin/env bash
# Build the Go DPI-bypass engine into app/libs/zapret.aar via gomobile.
#
# Requirements: Go 1.24+, Android SDK + NDK, and gomobile/gobind on PATH
# (go install golang.org/x/mobile/cmd/gomobile@latest golang.org/x/mobile/cmd/gobind@latest).
# Set ANDROID_HOME (or ANDROID_SDK_ROOT) and ANDROID_NDK_HOME.
set -euo pipefail
cd "$(dirname "$0")/.."

: "${ANDROID_HOME:=${ANDROID_SDK_ROOT:-/opt/android-sdk}}"
export ANDROID_HOME
if [ -z "${ANDROID_NDK_HOME:-}" ]; then
  # Pick the highest installed NDK.
  ANDROID_NDK_HOME="$(ls -d "$ANDROID_HOME"/ndk/* 2>/dev/null | sort -V | tail -1)"
  export ANDROID_NDK_HOME
fi
[ -d "$ANDROID_NDK_HOME" ] || { echo "NDK not found; set ANDROID_NDK_HOME" >&2; exit 1; }

GOBIN="${GOBIN:-$(go env GOPATH)/bin}"
export PATH="$GOBIN:$PATH"
command -v gomobile >/dev/null || { echo "gomobile not on PATH (go install golang.org/x/mobile/cmd/gomobile@latest)" >&2; exit 1; }

ABIS="${ABIS:-android/arm64,android/amd64,android/arm}"
OUT=app/libs/zapret.aar
mkdir -p app/libs

echo "Building $OUT for $ABIS (NDK: $ANDROID_NDK_HOME)"
( cd engine && gomobile bind \
    -target="$ABIS" \
    -androidapi 21 \
    -javapkg io.github.revocx35 \
    -o "../$OUT" \
    ./mobile )

echo "Done: $OUT ($(du -h "$OUT" | cut -f1))"

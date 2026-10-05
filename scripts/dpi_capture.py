#!/usr/bin/env python3
"""Host-side TLS "DPI" capture server for the e2e test.

Listens on 127.0.0.1:<port> (reachable from the emulator as 10.0.2.2:<port>),
accepts one connection, reads the initial bytes of the stream, parses the TLS
records, and writes a JSON verdict describing how the ClientHello was framed.

A connection that went through the Zapret engine with `tlsrec:sni` should arrive
as >= 2 TLS handshake records, with the SNI host name split across the record
boundary (so a DPI parsing only the first record cannot see the full domain).
"""
import json
import socket
import sys
import time


def parse_records(buf: bytes):
    """Return a list of (content_type, fragment_bytes) for each TLS record."""
    recs = []
    i = 0
    while i + 5 <= len(buf):
        ctype = buf[i]
        length = (buf[i + 3] << 8) | buf[i + 4]
        frag = buf[i + 5:i + 5 + length]
        recs.append((ctype, frag))
        i += 5 + length
        if len(frag) < length:
            break  # truncated capture
    return recs


def main():
    port = int(sys.argv[1])
    out = sys.argv[2]
    sni = sys.argv[3].encode()

    srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    # Bind all interfaces: the emulator reaches us via the host's LAN IP (an
    # off-subnet address, so the probe routes through the tunnel).
    srv.bind(("0.0.0.0", port))
    srv.listen(1)
    srv.settimeout(60)
    print(f"[capture] listening on 0.0.0.0:{port}", flush=True)

    try:
        conn, addr = srv.accept()
    except socket.timeout:
        json.dump({"ok": False, "error": "no connection within 60s"}, open(out, "w"))
        return

    conn.settimeout(2.0)
    data = b""
    deadline = time.time() + 2.5
    while time.time() < deadline and len(data) < 8192:
        try:
            chunk = conn.recv(4096)
        except socket.timeout:
            break
        if not chunk:
            break
        data += chunk
    conn.close()
    srv.close()

    recs = parse_records(data)
    handshake_recs = [r for r in recs if r[0] == 0x16]
    first_frag = handshake_recs[0][1] if handshake_recs else b""

    verdict = {
        "ok": True,
        "total_bytes": len(data),
        "record_count": len(recs),
        "handshake_record_count": len(handshake_recs),
        "record_fragment_lengths": [len(f) for _, f in recs],
        "sni_present_in_stream": sni in data,
        "sni_whole_in_first_record": sni in first_frag,
        # The desync is demonstrated when the ClientHello spans >= 2 records and
        # the SNI is NOT wholly contained in the first record.
        "fragmented": len(handshake_recs) >= 2 and (sni not in first_frag),
    }
    json.dump(verdict, open(out, "w"))
    print("[capture] verdict:", json.dumps(verdict), flush=True)


if __name__ == "__main__":
    main()

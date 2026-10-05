package io.github.revocx35.zapretmobile.engine

import android.util.Log
import io.github.revocx35.zapret.Logger
import io.github.revocx35.zapret.Protector
import io.github.revocx35.zapretmobile.vpn.VpnState

/**
 * Bridges the Go engine's Protector callback to VpnService.protect(fd). Called
 * from native code, so this class is kept by ProGuard (see proguard-rules.pro).
 *
 * gomobile maps Go `int` to Java `long`, hence the Long parameter.
 */
class ProtectorImpl(private val onProtect: (Int) -> Boolean) : Protector {
    override fun protect(fd: Long): Boolean = onProtect(fd.toInt())
}

/** Bridges the Go engine's log lines to the shared [VpnState] log buffer (and logcat). */
class LoggerImpl : Logger {
    override fun log(line: String) {
        Log.d("ZapretEngine", line)
        VpnState.log(line)
    }
}

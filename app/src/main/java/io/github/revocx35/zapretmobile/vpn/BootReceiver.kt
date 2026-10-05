package io.github.revocx35.zapretmobile.vpn

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.net.VpnService
import io.github.revocx35.zapretmobile.data.SettingsRepository
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking

/** Re-establishes the VPN after reboot when auto-start is enabled and consent was already granted. */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED) return
        val settings = runBlocking { SettingsRepository(context.applicationContext).settings.first() }
        if (!settings.autoStart) return
        // Only start if the user has previously approved the VPN connection.
        if (VpnService.prepare(context) == null) {
            ZapretVpnService.startWithSavedSettings(context)
        }
    }
}

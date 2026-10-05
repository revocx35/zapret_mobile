package io.github.revocx35.zapretmobile.vpn

import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import io.github.revocx35.zapretmobile.ui.MainActivity
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/** Quick Settings tile to toggle the VPN without opening the app. */
class ZapretTileService : TileService() {

    private var scope: CoroutineScope? = null

    override fun onStartListening() {
        super.onStartListening()
        val s = CoroutineScope(Dispatchers.Main + Job())
        scope = s
        s.launch {
            VpnState.status.collect { render(it) }
        }
    }

    override fun onStopListening() {
        scope?.cancel()
        scope = null
        super.onStopListening()
    }

    @Suppress("StartActivityAndCollapseDeprecated")
    override fun onClick() {
        super.onClick()
        if (VpnState.isActive) {
            ZapretVpnService.stop(this)
            return
        }
        // Starting requires VPN consent. If already granted, prepare() returns null
        // and we can start directly; otherwise send the user to the app to approve.
        if (VpnService.prepare(this) == null) {
            ZapretVpnService.startWithSavedSettings(this)
            return
        }
        val i = Intent(this, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            .putExtra(MainActivity.EXTRA_AUTO_START, true)
        if (Build.VERSION.SDK_INT >= 34) {
            val pi = PendingIntent.getActivity(
                this, 0, i,
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
            startActivityAndCollapse(pi)
        } else {
            @Suppress("DEPRECATION")
            startActivityAndCollapse(i)
        }
    }

    private fun render(status: VpnStatus) {
        val tile = qsTile ?: return
        tile.state = if (status == VpnStatus.CONNECTED || status == VpnStatus.CONNECTING) {
            Tile.STATE_ACTIVE
        } else {
            Tile.STATE_INACTIVE
        }
        tile.updateTile()
    }
}

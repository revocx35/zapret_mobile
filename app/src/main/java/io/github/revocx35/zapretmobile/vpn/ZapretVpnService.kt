package io.github.revocx35.zapretmobile.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import io.github.revocx35.zapret.Engine
import io.github.revocx35.zapret.Zapret
import io.github.revocx35.zapretmobile.R
import io.github.revocx35.zapretmobile.data.Settings
import io.github.revocx35.zapretmobile.data.SettingsRepository
import io.github.revocx35.zapretmobile.data.Strategies
import io.github.revocx35.zapretmobile.engine.LoggerImpl
import io.github.revocx35.zapretmobile.engine.ProtectorImpl
import io.github.revocx35.zapretmobile.ui.MainActivity
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking

/**
 * The no-root DPI-bypass VPN. It establishes a local TUN interface, hands the fd
 * to the Go engine (gVisor userspace TCP/IP stack + desync dialer), and keeps a
 * foreground notification while running. No traffic leaves the device through any
 * remote server: the engine dials each destination directly, only re-framing the
 * TLS ClientHello / HTTP request so censorship DPI cannot match the SNI/Host.
 */
class ZapretVpnService : VpnService() {

    private var engine: Engine? = null
    private var tun: ParcelFileDescriptor? = null

    override fun onCreate() {
        super.onCreate()
        ensureChannel(this)
        go.Seq.setContext(applicationContext)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                stopVpn()
                return START_NOT_STICKY
            }
            else -> startVpn(intent)
        }
        return START_STICKY
    }

    private fun startVpn(intent: Intent?) {
        if (engine != null) return // already running
        VpnState.setStatus(VpnStatus.CONNECTING)
        startForegroundCompat()

        val settings = readSettings()
        val strategy = intent?.getStringExtra(EXTRA_STRATEGY)
            ?: Strategies.resolve(settings)
        val blockQuic = intent?.let {
            if (it.hasExtra(EXTRA_BLOCK_QUIC)) it.getBooleanExtra(EXTRA_BLOCK_QUIC, true) else null
        } ?: settings.blockQuic

        try {
            val pfd = buildInterface() ?: throw IllegalStateException("VPN permission not granted")
            tun = pfd

            val e = Zapret.newEngine()
            e.setLogger(LoggerImpl())
            e.setProtector(ProtectorImpl { fd -> protect(fd) })
            e.startTunnel(pfd.fd.toLong(), MTU.toLong(), strategy, blockQuic)
            engine = e

            VpnState.setActiveStrategy(strategy)
            VpnState.log("connected (strategy: $strategy, block QUIC: $blockQuic)")
            VpnState.setStatus(VpnStatus.CONNECTED)
            Log.i(TAG, "connected strategy=$strategy blockQuic=$blockQuic")
            updateNotification(getString(R.string.notif_connected))
        } catch (t: Throwable) {
            Log.e(TAG, "start failed", t)
            VpnState.setError(t.message ?: t.toString())
            cleanup()
            stopForegroundCompat()
            stopSelf()
        }
    }

    private fun buildInterface(): ParcelFileDescriptor? {
        val b = Builder()
            .setSession(getString(R.string.app_name))
            .setMtu(MTU)
            .addAddress(TUN_V4, 32)
            .addAddress(TUN_V6, 128)
            .addRoute("0.0.0.0", 0)
            .addRoute("::", 0)
            .addDnsServer("1.1.1.1")
            .addDnsServer("1.0.0.1")
            .addDnsServer("2606:4700:4700::1111")
        // Keep our own process (which hosts the engine) outside the tunnel so our
        // upstream sockets never loop back in. protect() also covers this, but
        // excluding the app is a clean belt-and-suspenders.
        try {
            b.addDisallowedApplication(packageName)
        } catch (_: PackageManager.NameNotFoundException) {
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            b.setMetered(false)
        }
        return b.establish()
    }

    private fun readSettings(): Settings = runBlocking {
        SettingsRepository(applicationContext).settings.first()
    }

    private fun stopVpn() {
        cleanup()
        VpnState.setStatus(VpnStatus.STOPPED)
        VpnState.log("disconnected")
        stopForegroundCompat()
        stopSelf()
    }

    private fun cleanup() {
        engine?.let {
            try {
                it.stop()
            } catch (_: Throwable) {
            }
        }
        engine = null
        tun?.let {
            try {
                it.close()
            } catch (_: Throwable) {
            }
        }
        tun = null
    }

    override fun onRevoke() {
        VpnState.log("VPN revoked by system")
        stopVpn()
        super.onRevoke()
    }

    override fun onDestroy() {
        cleanup()
        if (VpnState.status.value != VpnStatus.ERROR) {
            VpnState.setStatus(VpnStatus.STOPPED)
        }
        super.onDestroy()
    }

    // ---- notification ----

    private fun startForegroundCompat() {
        val n = buildNotification(getString(R.string.notif_connecting))
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTIF_ID, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIF_ID, n)
        }
    }

    private fun stopForegroundCompat() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            stopForeground(STOP_FOREGROUND_REMOVE)
        } else {
            @Suppress("DEPRECATION")
            stopForeground(true)
        }
    }

    private fun updateNotification(text: String) {
        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        nm.notify(NOTIF_ID, buildNotification(text))
    }

    private fun buildNotification(text: String): Notification {
        val open = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val stop = PendingIntent.getService(
            this, 1,
            Intent(this, ZapretVpnService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_shield)
            .setOngoing(true)
            .setContentIntent(open)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .addAction(0, getString(R.string.action_stop), stop)
            .build()
    }

    companion object {
        private const val TAG = "ZapretVpn"
        const val ACTION_START = "io.github.revocx35.zapretmobile.START"
        const val ACTION_STOP = "io.github.revocx35.zapretmobile.STOP"
        const val EXTRA_STRATEGY = "strategy"
        const val EXTRA_BLOCK_QUIC = "block_quic"

        private const val CHANNEL_ID = "vpn"
        private const val NOTIF_ID = 1
        private const val MTU = 1500
        private const val TUN_V4 = "10.111.222.1"
        private const val TUN_V6 = "fd00:0:0:beef::1"

        fun ensureChannel(context: Context) {
            val nm = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            if (nm.getNotificationChannel(CHANNEL_ID) == null) {
                val ch = NotificationChannel(
                    CHANNEL_ID,
                    context.getString(R.string.channel_vpn),
                    NotificationManager.IMPORTANCE_LOW,
                ).apply { setShowBadge(false) }
                nm.createNotificationChannel(ch)
            }
        }

        /** Start the VPN with an explicit strategy (from the UI). */
        fun start(context: Context, strategy: String, blockQuic: Boolean) {
            val i = Intent(context, ZapretVpnService::class.java)
                .setAction(ACTION_START)
                .putExtra(EXTRA_STRATEGY, strategy)
                .putExtra(EXTRA_BLOCK_QUIC, blockQuic)
            ContextCompat.startForegroundService(context, i)
        }

        /** Start the VPN using saved settings (from the tile / boot). */
        fun startWithSavedSettings(context: Context) {
            val i = Intent(context, ZapretVpnService::class.java).setAction(ACTION_START)
            ContextCompat.startForegroundService(context, i)
        }

        fun stop(context: Context) {
            val i = Intent(context, ZapretVpnService::class.java).setAction(ACTION_STOP)
            context.startService(i)
        }
    }
}

package io.github.revocx35.zapretmobile.ui

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.core.content.ContextCompat
import io.github.revocx35.zapretmobile.data.Strategies
import io.github.revocx35.zapretmobile.ui.theme.ZapretTheme
import io.github.revocx35.zapretmobile.vpn.ZapretVpnService

class MainActivity : ComponentActivity() {

    private val vm: MainViewModel by viewModels()

    private val vpnConsent = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result ->
        if (result.resultCode == RESULT_OK) {
            startVpn()
        }
    }

    private val notifPermission = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { /* proceed regardless; the notification is best-effort */ requestConsentThenStart() }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        ZapretVpnService.ensureChannel(this)
        setContent {
            ZapretTheme {
                MainScreen(vm = vm, onToggle = ::onToggle)
            }
        }
        // Only on a fresh launch: after rotation the same intent is redelivered.
        if (savedInstanceState == null && intent?.getBooleanExtra(EXTRA_AUTO_START, false) == true) {
            onToggle(true)
        }
    }

    private fun onToggle(start: Boolean) {
        if (!start) {
            ZapretVpnService.stop(this)
            return
        }
        // On Android 13+ ask for notification permission first (the FGS needs a
        // visible notification); then request VPN consent; then start.
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) !=
            PackageManager.PERMISSION_GRANTED
        ) {
            notifPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
        } else {
            requestConsentThenStart()
        }
    }

    private fun requestConsentThenStart() {
        val consent: Intent? = VpnService.prepare(this)
        if (consent != null) {
            vpnConsent.launch(consent)
        } else {
            startVpn()
        }
    }

    private fun startVpn() {
        val s = vm.settings.value
        ZapretVpnService.start(this, Strategies.resolve(s), s.blockQuic)
    }

    companion object {
        const val EXTRA_AUTO_START = "auto_start"
    }
}

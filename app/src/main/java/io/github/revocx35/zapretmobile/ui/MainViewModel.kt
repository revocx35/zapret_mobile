package io.github.revocx35.zapretmobile.ui

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import io.github.revocx35.zapret.Zapret
import io.github.revocx35.zapretmobile.data.Settings
import io.github.revocx35.zapretmobile.data.SettingsRepository
import io.github.revocx35.zapretmobile.data.Strategies
import io.github.revocx35.zapretmobile.vpn.VpnState
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

class MainViewModel(app: Application) : AndroidViewModel(app) {
    private val repo = SettingsRepository(app)

    val settings = repo.settings.stateIn(
        viewModelScope, SharingStarted.WhileSubscribed(5_000), Settings(),
    )

    val status = VpnState.status
    val logs = VpnState.logs
    val activeStrategy = VpnState.activeStrategy
    val lastError = VpnState.lastError

    fun selectPreset(id: String) = viewModelScope.launch { repo.setPreset(id) }
    fun setUseCustom(enabled: Boolean) = viewModelScope.launch { repo.setUseCustom(enabled) }
    fun setCustom(strategy: String) = viewModelScope.launch { repo.setCustom(strategy) }
    fun setBlockQuic(enabled: Boolean) = viewModelScope.launch { repo.setBlockQuic(enabled) }
    fun setAutoStart(enabled: Boolean) = viewModelScope.launch { repo.setAutoStart(enabled) }

    fun clearLogs() = VpnState.clearLogs()

    /** Returns a validation error for [strategy], or null if it is valid. */
    fun validate(strategy: String): String? =
        Zapret.validateStrategy(strategy).ifEmpty { null }

    /** The strategy string that would be used to start the VPN right now. */
    fun effectiveStrategy(): String = Strategies.resolve(settings.value)
}

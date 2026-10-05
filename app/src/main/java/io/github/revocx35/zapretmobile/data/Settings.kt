package io.github.revocx35.zapretmobile.data

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

/** User-configurable settings, persisted with DataStore. */
data class Settings(
    val presetId: String = Strategies.DEFAULT_ID,
    val useCustomStrategy: Boolean = false,
    val customStrategy: String = "tlsrec:sni",
    val blockQuic: Boolean = true,
    val autoStart: Boolean = false,
)

private val Context.dataStore by preferencesDataStore(name = "zapret_settings")

object SettingsKeys {
    val PRESET = stringPreferencesKey("preset_id")
    val USE_CUSTOM = booleanPreferencesKey("use_custom")
    val CUSTOM = stringPreferencesKey("custom_strategy")
    val BLOCK_QUIC = booleanPreferencesKey("block_quic")
    val AUTO_START = booleanPreferencesKey("auto_start")
}

class SettingsRepository(private val context: Context) {
    val settings: Flow<Settings> = context.dataStore.data.map { p ->
        Settings(
            presetId = p[SettingsKeys.PRESET] ?: Strategies.DEFAULT_ID,
            useCustomStrategy = p[SettingsKeys.USE_CUSTOM] ?: false,
            customStrategy = p[SettingsKeys.CUSTOM] ?: "tlsrec:sni",
            blockQuic = p[SettingsKeys.BLOCK_QUIC] ?: true,
            autoStart = p[SettingsKeys.AUTO_START] ?: false,
        )
    }

    suspend fun setPreset(id: String) = context.dataStore.edit {
        it[SettingsKeys.PRESET] = id
        it[SettingsKeys.USE_CUSTOM] = false
    }

    suspend fun setCustom(strategy: String) = context.dataStore.edit {
        it[SettingsKeys.CUSTOM] = strategy
        it[SettingsKeys.USE_CUSTOM] = true
    }

    suspend fun setUseCustom(enabled: Boolean) = context.dataStore.edit {
        it[SettingsKeys.USE_CUSTOM] = enabled
    }

    suspend fun setBlockQuic(enabled: Boolean) = context.dataStore.edit {
        it[SettingsKeys.BLOCK_QUIC] = enabled
    }

    suspend fun setAutoStart(enabled: Boolean) = context.dataStore.edit {
        it[SettingsKeys.AUTO_START] = enabled
    }
}

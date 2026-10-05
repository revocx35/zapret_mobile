package io.github.revocx35.zapretmobile.vpn

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

enum class VpnStatus { STOPPED, CONNECTING, CONNECTED, ERROR }

/**
 * Process-wide observable state shared between the VpnService and the UI. The
 * service is the single writer; the UI only reads.
 */
object VpnState {
    private const val MAX_LOG_LINES = 300

    private val _status = MutableStateFlow(VpnStatus.STOPPED)
    val status: StateFlow<VpnStatus> = _status.asStateFlow()

    private val _lastError = MutableStateFlow<String?>(null)
    val lastError: StateFlow<String?> = _lastError.asStateFlow()

    private val _logs = MutableStateFlow<List<String>>(emptyList())
    val logs: StateFlow<List<String>> = _logs.asStateFlow()

    private val _activeStrategy = MutableStateFlow("")
    val activeStrategy: StateFlow<String> = _activeStrategy.asStateFlow()

    private val timeFmt = SimpleDateFormat("HH:mm:ss", Locale.US)

    fun setStatus(status: VpnStatus) {
        _status.value = status
        if (status != VpnStatus.ERROR) _lastError.value = null
    }

    fun setError(message: String) {
        _lastError.value = message
        _status.value = VpnStatus.ERROR
        log("error: $message")
    }

    fun setActiveStrategy(strategy: String) {
        _activeStrategy.value = strategy
    }

    fun log(line: String) {
        val stamped = "${timeFmt.format(Date())}  $line"
        _logs.update { prev ->
            val next = prev + stamped
            if (next.size > MAX_LOG_LINES) next.takeLast(MAX_LOG_LINES) else next
        }
    }

    fun clearLogs() {
        _logs.value = emptyList()
    }

    val isActive: Boolean
        get() = _status.value == VpnStatus.CONNECTED || _status.value == VpnStatus.CONNECTING
}

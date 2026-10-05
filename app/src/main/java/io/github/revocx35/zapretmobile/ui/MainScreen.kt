package io.github.revocx35.zapretmobile.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.revocx35.zapretmobile.data.Strategies
import io.github.revocx35.zapretmobile.vpn.VpnStatus

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MainScreen(vm: MainViewModel, onToggle: (Boolean) -> Unit) {
    val status by vm.status.collectAsStateWithLifecycle()
    val settings by vm.settings.collectAsStateWithLifecycle()
    val logs by vm.logs.collectAsStateWithLifecycle()
    val activeStrategy by vm.activeStrategy.collectAsStateWithLifecycle()
    val lastError by vm.lastError.collectAsStateWithLifecycle()

    Scaffold(topBar = { TopAppBar(title = { Text("Zapret Mobile") }) }) { pad ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(pad)
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            StatusCard(status, activeStrategy, onToggle)
            lastError?.let { ErrorBanner(it) }
            StrategyCard(vm, settings.presetId, settings.useCustomStrategy, settings.customStrategy)
            OptionsCard(
                blockQuic = settings.blockQuic,
                autoStart = settings.autoStart,
                onBlockQuic = vm::setBlockQuic,
                onAutoStart = vm::setAutoStart,
            )
            LogsCard(logs = logs, onClear = vm::clearLogs)
            FooterInfo()
        }
    }
}

@Composable
private fun StatusCard(status: VpnStatus, activeStrategy: String, onToggle: (Boolean) -> Unit) {
    val active = status == VpnStatus.CONNECTED || status == VpnStatus.CONNECTING
    val label = when (status) {
        VpnStatus.STOPPED -> "Disconnected"
        VpnStatus.CONNECTING -> "Connecting…"
        VpnStatus.CONNECTED -> "Protected"
        VpnStatus.ERROR -> "Error"
    }
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(20.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                text = label,
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.SemiBold,
            )
            if (active && activeStrategy.isNotEmpty()) {
                Text(
                    text = "Strategy: $activeStrategy",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                )
            }
            Button(
                onClick = { onToggle(!active) },
                modifier = Modifier
                    .fillMaxWidth()
                    .height(52.dp),
                colors = if (active) {
                    ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.error)
                } else {
                    ButtonDefaults.buttonColors()
                },
            ) {
                Text(if (active) "Stop" else "Start")
            }
        }
    }
}

@Composable
private fun ErrorBanner(message: String) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.errorContainer),
    ) {
        Text(
            text = message,
            modifier = Modifier.padding(16.dp),
            color = MaterialTheme.colorScheme.onErrorContainer,
            style = MaterialTheme.typography.bodyMedium,
        )
    }
}

@Composable
private fun StrategyCard(
    vm: MainViewModel,
    selectedPreset: String,
    useCustom: Boolean,
    custom: String,
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp)) {
            Text("DPI-bypass strategy", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(8.dp))
            Strategies.presets.forEach { preset ->
                val selected = !useCustom && preset.id == selectedPreset
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .selectable(selected = selected, onClick = { vm.selectPreset(preset.id) })
                        .padding(vertical = 6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    RadioButton(selected = selected, onClick = { vm.selectPreset(preset.id) })
                    Spacer(Modifier.size(8.dp))
                    Column {
                        Text(preset.title, style = MaterialTheme.typography.bodyLarge)
                        Text(
                            preset.description,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }

            HorizontalDivider(Modifier.padding(vertical = 8.dp))

            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .selectable(selected = useCustom, onClick = { vm.setUseCustom(true) })
                    .padding(vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                RadioButton(selected = useCustom, onClick = { vm.setUseCustom(true) })
                Spacer(Modifier.size(8.dp))
                Text("Custom", style = MaterialTheme.typography.bodyLarge)
            }
            if (useCustom) {
                val err = vm.validate(custom)
                OutlinedTextField(
                    value = custom,
                    onValueChange = vm::setCustom,
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                    label = { Text("Strategy string") },
                    isError = err != null,
                    supportingText = {
                        Text(err ?: "e.g. tlsrec:sni,split:1 — see the README for the grammar")
                    },
                )
            }
        }
    }
}

@Composable
private fun OptionsCard(
    blockQuic: Boolean,
    autoStart: Boolean,
    onBlockQuic: (Boolean) -> Unit,
    onAutoStart: (Boolean) -> Unit,
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp)) {
            Text("Options", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(8.dp))
            SwitchRow(
                title = "Block QUIC (UDP/443)",
                subtitle = "Forces browsers to use TCP TLS, where the desync applies. Recommended.",
                checked = blockQuic,
                onChange = onBlockQuic,
            )
            SwitchRow(
                title = "Start on boot",
                subtitle = "Reconnect automatically after the device restarts.",
                checked = autoStart,
                onChange = onAutoStart,
            )
        }
    }
}

@Composable
private fun SwitchRow(
    title: String,
    subtitle: String,
    checked: Boolean,
    onChange: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            Text(
                subtitle,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Switch(checked = checked, onCheckedChange = onChange)
    }
}

@Composable
private fun LogsCard(logs: List<String>, onClear: () -> Unit) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Log", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                TextButton(onClick = onClear) { Text("Clear") }
            }
            val scroll = rememberScrollState()
            LaunchedEffect(logs.size) { scroll.animateScrollTo(scroll.maxValue) }
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 80.dp, max = 220.dp)
                    .verticalScroll(scroll),
            ) {
                if (logs.isEmpty()) {
                    Text(
                        "No activity yet.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    logs.forEach { line ->
                        Text(
                            line,
                            fontFamily = FontFamily.Monospace,
                            fontSize = 11.sp,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun FooterInfo() {
    Text(
        text = "No root required. Zapret Mobile runs a local VPN and rewrites only how the " +
            "TLS ClientHello and HTTP request are framed (TLS-record and TCP fragmentation), " +
            "so censorship DPI can't match the domain. Your traffic goes directly to each site — " +
            "there is no remote proxy server and nothing is decrypted.",
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 4.dp, vertical = 8.dp),
    )
}

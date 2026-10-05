package io.github.revocx35.zapretmobile.data

/**
 * A named desync strategy. The [value] is the engine strategy string parsed by
 * the Go engine (see engine/desync/strategy.go). Every preset here is
 * server-safe: it only changes how the request is framed into TLS records and
 * TCP segments, never the bytes the server reassembles.
 */
data class StrategyPreset(
    val id: String,
    val title: String,
    val description: String,
    val value: String,
)

object Strategies {
    /** Presets offered in the UI, best general-purpose first. */
    val presets: List<StrategyPreset> = listOf(
        StrategyPreset(
            id = "default",
            title = "Default",
            description = "Fragment the TLS ClientHello at the SNI; split HTTP at the Host header.",
            value = "tlsrec:sni,hostsplit:host",
        ),
        StrategyPreset(
            id = "tlsrec_split",
            title = "Record split + segment",
            description = "Fragment at the SNI and also force a separate first TCP segment. Beats DPI that reassembles TLS but not TCP.",
            value = "tlsrec:sni,split:1,hostsplit:host",
        ),
        StrategyPreset(
            id = "early_split",
            title = "Aggressive (early cut)",
            description = "Cut the record one byte before the SNI midpoint, with a small inter-segment delay.",
            value = "tlsrec:sni-1,split:2 @delay=15",
        ),
        StrategyPreset(
            id = "split_only",
            title = "TCP split only",
            description = "Just split the first TCP segment near the start. Lightest touch; works against simple DPI.",
            value = "split:3",
        ),
        StrategyPreset(
            id = "oob",
            title = "OOB (experimental)",
            description = "Fragment at the SNI and inject a TCP urgent byte. Effective against some DPI; a few servers mishandle urgent data.",
            value = "tlsrec:sni,oob:sni",
        ),
    )

    const val DEFAULT_ID = "default"

    fun byId(id: String): StrategyPreset? = presets.firstOrNull { it.id == id }

    /** The strategy string to feed the engine, given the saved settings. */
    fun resolve(settings: Settings): String =
        if (settings.useCustomStrategy) settings.customStrategy
        else byId(settings.presetId)?.value ?: presets.first().value
}

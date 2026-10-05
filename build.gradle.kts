// Top-level build file. The app module is the Android UI + VpnService; the DPI-bypass
// engine lives in a Go module under engine/ and is shipped as app/libs/zapret.aar
// (built by scripts/build-engine.sh).
plugins {
    alias(libs.plugins.android.application) apply false
    // Declared so the Kotlin Gradle plugin on the classpath matches the Compose compiler
    // plugin version (AGP 9 compiles Kotlin itself; the app module does not apply this).
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.compose) apply false
}

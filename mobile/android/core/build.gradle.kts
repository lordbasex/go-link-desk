// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Plain Kotlin: the protocol and session logic, with no Android types, so it
// can be tested on the JVM, the same logic as the iOS app's DeskCore.
plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
}

kotlin {
    jvmToolchain(17)
}

dependencies {
    api(libs.kotlinx.coroutines.core)
    api(libs.kotlinx.serialization.json)
    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
}

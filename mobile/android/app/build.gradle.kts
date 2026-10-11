// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

// Release signing comes from a gitignored keystore.properties next to this
// project (or the file DESK_SIGNING points to, kept outside the repository,
// like ~/.go-link-signing/desk-android.properties), or from environment
// variables (CI). Nothing secret is committed; without them the release is
// built unsigned.
val signing = Properties().apply {
    val file = System.getenv("DESK_SIGNING")?.takeIf { it.isNotBlank() }?.let { File(it) }
        ?: rootProject.file("keystore.properties")
    if (file.isFile) file.inputStream().use { load(it) }
}

fun signingValue(key: String, env: String): String? =
    signing.getProperty(key)?.takeIf { it.isNotBlank() } ?: System.getenv(env)?.takeIf { it.isNotBlank() }

val releaseStoreFile = signingValue("storeFile", "DESK_KEYSTORE")
val releaseStorePassword = signingValue("storePassword", "DESK_KEYSTORE_PASSWORD")
val releaseKeyAlias = signingValue("keyAlias", "DESK_KEY_ALIAS")
val releaseKeyPassword = signingValue("keyPassword", "DESK_KEY_PASSWORD")
val canSignRelease = listOf(releaseStoreFile, releaseStorePassword, releaseKeyAlias, releaseKeyPassword).all { it != null }

android {
    namespace = "org.golink.desk"
    compileSdk = 37

    defaultConfig {
        applicationId = "org.golink.desk"
        // Android 11: the keystore key that opens the linked computers asks for
        // the fingerprint, the face or the screen lock (BiometricPrompt).
        minSdk = 30
        targetSdk = 36
        versionCode = (findProperty("versionCode") as String?)?.toIntOrNull() ?: 1
        versionName = (findProperty("versionName") as String?) ?: "0.1.0"
        // Phones and tablets (ARM) and the x86_64 emulator; 32-bit x86 is gone.
        ndk { abiFilters += listOf("arm64-v8a", "armeabi-v7a", "x86_64") }
    }

    signingConfigs {
        if (canSignRelease) {
            create("release") {
                storeFile = file(releaseStoreFile!!)
                storePassword = releaseStorePassword
                keyAlias = releaseKeyAlias
                keyPassword = releaseKeyPassword
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            if (canSignRelease) signingConfig = signingConfigs.getByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    packaging {
        resources.excludes += setOf("META-INF/{AL2.0,LGPL2.1}", "META-INF/versions/9/OSGI-INF/MANIFEST.MF")
    }
}

kotlin {
    jvmToolchain(17)
}

dependencies {
    implementation(project(":core"))
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.okhttp)
    implementation(libs.stream.webrtc)
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.lifecycle.runtime.compose)
    implementation(libs.lifecycle.viewmodel.ktx)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.foundation)
    implementation(libs.compose.material3)
    implementation(libs.compose.icons)
}

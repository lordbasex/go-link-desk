// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.runtime.LaunchedEffect
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import org.golink.desk.ui.DeskTheme
import org.golink.desk.ui.HomeScreen
import org.golink.desk.ui.SessionScreen

/** go-link Desk for Android phones and tablets: connect to another computer with its code, and see and use it. */
class MainActivity : ComponentActivity() {
    private val app: AppModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        if (savedInstanceState == null) debugLaunch()
        setContent {
            DeskTheme {
                val s = app.session
                LaunchedEffect(s != null) { fullScreen(s != null) }
                if (s != null) SessionScreen(s, app.unlockName) else HomeScreen(app)
            }
        }
    }

    /** A session takes the whole screen: the system bars come back with a swipe. */
    private fun fullScreen(on: Boolean) {
        val c = WindowInsetsControllerCompat(window, window.decorView)
        if (on) {
            c.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            c.hide(WindowInsetsCompat.Type.systemBars())
        } else {
            c.show(WindowInsetsCompat.Type.systemBars())
        }
        WindowCompat.getInsetsController(window, window.decorView).isAppearanceLightStatusBars = !on && !isDark()
    }

    private fun isDark() = resources.configuration.uiMode and android.content.res.Configuration.UI_MODE_NIGHT_MASK == android.content.res.Configuration.UI_MODE_NIGHT_YES

    /**
     * Debug builds only: --es code 915355636 connects at once,
     * --es signal ws://10.0.2.2:8391/ws picks a local server.
     */
    private fun debugLaunch() {
        if (!BuildConfig.DEBUG) return
        intent?.getStringExtra("signal")?.takeIf { it.isNotEmpty() }?.let { app.updateSignal(it) }
        intent?.getStringExtra("code")?.let { app.connect(it) }
    }
}

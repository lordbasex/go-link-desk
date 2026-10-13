// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.util.Base64
import org.json.JSONObject
import java.net.Inet4Address

/**
 * Test my connection: the connection test (desk.go-link.org/network) with
 * what only the app sees, after "#d=" (docs/network.md): this device's
 * address on its network, its router (the default route's gateway) and
 * whether it is on Wi-Fi, mobile data (almost always the provider's CGNAT)
 * or Ethernet. The fragment is never sent to a server.
 */
object NetworkProbe {
    fun url(context: Context, web: String = "https://desk.go-link.org"): String {
        val info = JSONObject().put("src", "android")
        val cm = context.getSystemService(ConnectivityManager::class.java)
        val net = cm?.activeNetwork
        val caps = net?.let { cm.getNetworkCapabilities(it) }
        info.put(
            "net",
            when {
                caps == null -> "other"
                caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "wifi"
                caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "cellular"
                caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "ethernet"
                else -> "other"
            },
        )
        net?.let { cm.getLinkProperties(it) }?.let { lp ->
            lp.linkAddresses.map { it.address }.firstOrNull { it is Inet4Address && !it.isLoopbackAddress }
                ?.hostAddress?.let { info.put("lan", it) }
            lp.routes.firstOrNull { it.isDefaultRoute && it.gateway is Inet4Address }
                ?.gateway?.hostAddress?.let { info.put("gw", it) }
        }
        val b64 = Base64.encodeToString(info.toString().toByteArray(), Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP)
        return "$web/network#d=$b64"
    }
}

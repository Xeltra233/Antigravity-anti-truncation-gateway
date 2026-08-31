package org.antigravity.gateway.util

import java.net.Inet4Address
import java.net.NetworkInterface

object NetworkUtils {
    const val GATEWAY_PORT = 38472

    fun getLocalIpAddress(): String {
        try {
            val interfaces = NetworkInterface.getNetworkInterfaces() ?: return "127.0.0.1"
            while (interfaces.hasMoreElements()) {
                val intf = interfaces.nextElement()
                if (intf.isLoopback || !intf.isUp) continue
                val addrs = intf.inetAddresses
                while (addrs.hasMoreElements()) {
                    val addr = addrs.nextElement()
                    if (!addr.isLoopbackAddress && addr is Inet4Address) {
                        val hostAddress = addr.hostAddress
                        if (!hostAddress.isNullOrEmpty() && !hostAddress.startsWith("127.") && !hostAddress.startsWith("169.254.")) {
                            return hostAddress
                        }
                    }
                }
            }
        } catch (_: Exception) {}
        return "127.0.0.1"
    }

    fun buildAppLink(ip: String = getLocalIpAddress(), port: Int = GATEWAY_PORT): String {
        return "http://$ip:$port/v1"
    }
}

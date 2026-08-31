package org.antigravity.gateway

import org.antigravity.gateway.util.NetworkUtils
import org.junit.Assert.*
import org.junit.Test

class NetworkUtilsTest {

    @Test
    fun testPortConstant() {
        assertEquals(38472, NetworkUtils.GATEWAY_PORT)
    }

    @Test
    fun testBuildAppLink() {
        val linkLocal = NetworkUtils.buildAppLink("127.0.0.1", 38472)
        assertEquals("http://127.0.0.1:38472/v1", linkLocal)

        val linkLan = NetworkUtils.buildAppLink("192.168.1.100", 38472)
        assertEquals("http://192.168.1.100:38472/v1", linkLan)
    }

    @Test
    fun testGetLocalIpAddressNotNull() {
        val ip = NetworkUtils.getLocalIpAddress()
        assertNotNull(ip)
        assertTrue(ip.isNotEmpty())
    }
}

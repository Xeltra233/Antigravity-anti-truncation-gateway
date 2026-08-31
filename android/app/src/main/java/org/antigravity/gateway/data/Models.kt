package org.antigravity.gateway.data

import java.util.UUID

data class Provider(
    val id: String = UUID.randomUUID().toString(),
    val name: String,
    val upstreamUrl: String = "",
    val upstreamKey: String = ""
)

data class GatewayConfig(
    val schemaVersion: Int = 1,
    val currentProviderId: String,
    val downstreamKey: String,
    val providers: List<Provider>
) {
    fun getCurrentProvider(): Provider? {
        return providers.firstOrNull { it.id == currentProviderId } ?: providers.firstOrNull()
    }
}

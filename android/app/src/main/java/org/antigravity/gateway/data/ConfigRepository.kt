package org.antigravity.gateway.data

import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.security.SecureRandom
import java.util.UUID

class ConfigRepository(
    private val storageFile: File,
    private val cryptoProvider: CryptoProvider
) {
    private var inMemoryConfig: GatewayConfig? = null

    companion object {
        const val CURRENT_SCHEMA_VERSION = 1
        const val DEFAULT_CONFIG_FILE_NAME = "gateway_config.json"

        fun generateDownstreamKey(): String {
            val randomBytes = ByteArray(24)
            SecureRandom().nextBytes(randomBytes)
            val hex = randomBytes.joinToString("") { "%02x".format(it) }
            return "sk-agw-$hex"
        }
    }

    @Synchronized
    fun load(): GatewayConfig {
        inMemoryConfig?.let { return it }

        if (!storageFile.exists() || storageFile.length() == 0L) {
            val defaultConfig = createInitialConfig()
            save(defaultConfig)
            inMemoryConfig = defaultConfig
            return defaultConfig
        }

        try {
            val jsonStr = storageFile.readText(Charsets.UTF_8)
            val root = JSONObject(jsonStr)
            val version = root.optInt("schemaVersion", 1)
            val currentId = root.optString("currentProviderId", "")
            val downstreamCipher = root.optString("downstreamKeyCiphertext", "")
            val downstreamKey = if (downstreamCipher.isNotEmpty()) {
                cryptoProvider.decrypt(downstreamCipher)
            } else {
                generateDownstreamKey()
            }

            val providersArray = root.optJSONArray("providers") ?: JSONArray()
            val providersList = mutableListOf<Provider>()
            for (i in 0 until providersArray.length()) {
                val item = providersArray.getJSONObject(i)
                val id = item.optString("id", UUID.randomUUID().toString())
                val name = item.optString("name", "供应商 ${i + 1}")
                val url = item.optString("upstreamUrl", "")
                val keyCipher = item.optString("upstreamKeyCiphertext", "")
                val key = if (keyCipher.isNotEmpty()) cryptoProvider.decrypt(keyCipher) else ""
                providersList.add(Provider(id = id, name = name, upstreamUrl = url, upstreamKey = key))
            }

            if (providersList.isEmpty()) {
                providersList.add(Provider(name = "供应商 1"))
            }

            val finalCurrentId = if (providersList.any { it.id == currentId }) {
                currentId
            } else {
                providersList.first().id
            }

            val loadedConfig = GatewayConfig(
                schemaVersion = version,
                currentProviderId = finalCurrentId,
                downstreamKey = downstreamKey,
                providers = providersList
            )
            inMemoryConfig = loadedConfig
            return loadedConfig
        } catch (e: Exception) {
            // If corrupted, create fallback and backup corrupted file
            val backupFile = File(storageFile.parentFile, "gateway_config_corrupted_${System.currentTimeMillis()}.bak")
            try {
                storageFile.copyTo(backupFile, overwrite = true)
            } catch (_: Exception) {}

            val fallback = createInitialConfig()
            save(fallback)
            inMemoryConfig = fallback
            return fallback
        }
    }

    private fun createInitialConfig(): GatewayConfig {
        val initialProvider = Provider(
            id = UUID.randomUUID().toString(),
            name = "供应商 1",
            upstreamUrl = "",
            upstreamKey = ""
        )
        return GatewayConfig(
            schemaVersion = CURRENT_SCHEMA_VERSION,
            currentProviderId = initialProvider.id,
            downstreamKey = generateDownstreamKey(),
            providers = listOf(initialProvider)
        )
    }

    @Synchronized
    fun save(config: GatewayConfig) {
        val root = JSONObject()
        root.put("schemaVersion", config.schemaVersion)
        root.put("currentProviderId", config.currentProviderId)
        root.put("downstreamKeyCiphertext", cryptoProvider.encrypt(config.downstreamKey))

        val array = JSONArray()
        for (p in config.providers) {
            val pObj = JSONObject()
            pObj.put("id", p.id)
            pObj.put("name", p.name)
            pObj.put("upstreamUrl", p.upstreamUrl.trim())
            pObj.put("upstreamKeyCiphertext", cryptoProvider.encrypt(p.upstreamKey))
            array.put(pObj)
        }
        root.put("providers", array)

        val tempFile = File(storageFile.parentFile, "${storageFile.name}.tmp")
        tempFile.writeText(root.toString(2), Charsets.UTF_8)
        if (tempFile.exists()) {
            tempFile.renameTo(storageFile)
        }
        inMemoryConfig = config
    }

    @Synchronized
    fun updateCurrentProviderDraft(upstreamUrl: String, upstreamKey: String): GatewayConfig {
        val current = load()
        val updatedProviders = current.providers.map {
            if (it.id == current.currentProviderId) {
                it.copy(upstreamUrl = upstreamUrl.trim(), upstreamKey = upstreamKey)
            } else {
                it
            }
        }
        val updated = current.copy(providers = updatedProviders)
        save(updated)
        return updated
    }

    @Synchronized
    fun selectProvider(providerId: String): GatewayConfig {
        val current = load()
        if (current.providers.none { it.id == providerId }) return current
        val updated = current.copy(currentProviderId = providerId)
        save(updated)
        return updated
    }

    @Synchronized
    fun createProvider(name: String, upstreamUrl: String = "", upstreamKey: String = ""): Pair<GatewayConfig, Provider> {
        val current = load()
        val actualName = name.trim().ifEmpty {
            "供应商 ${current.providers.size + 1}"
        }
        val newProvider = Provider(
            id = UUID.randomUUID().toString(),
            name = actualName,
            upstreamUrl = upstreamUrl.trim(),
            upstreamKey = upstreamKey
        )
        val updatedList = current.providers + newProvider
        val updated = current.copy(
            currentProviderId = newProvider.id,
            providers = updatedList
        )
        save(updated)
        return Pair(updated, newProvider)
    }

    @Synchronized
    fun renameProvider(providerId: String, newName: String): GatewayConfig {
        val current = load()
        val trimmed = newName.trim()
        if (trimmed.isEmpty()) return current
        val updatedList = current.providers.map {
            if (it.id == providerId) {
                it.copy(name = trimmed)
            } else {
                it
            }
        }
        val updated = current.copy(providers = updatedList)
        save(updated)
        return updated
    }

    @Synchronized
    fun deleteProvider(providerId: String): GatewayConfig {
        val current = load()
        if (current.providers.size <= 1) {
            // Cannot delete the last provider
            return current
        }
        val updatedList = current.providers.filter { it.id != providerId }
        val newCurrentId = if (current.currentProviderId == providerId) {
            updatedList.first().id
        } else {
            current.currentProviderId
        }
        val updated = current.copy(
            currentProviderId = newCurrentId,
            providers = updatedList
        )
        save(updated)
        return updated
    }

    @Synchronized
    fun regenerateDownstreamKey(): GatewayConfig {
        val current = load()
        val newKey = generateDownstreamKey()
        val updated = current.copy(downstreamKey = newKey)
        save(updated)
        return updated
    }
}

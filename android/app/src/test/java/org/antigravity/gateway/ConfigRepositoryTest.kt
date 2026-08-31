package org.antigravity.gateway

import org.antigravity.gateway.data.ConfigRepository
import org.antigravity.gateway.data.JvmSoftwareCryptoProvider
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import java.io.File

class ConfigRepositoryTest {

    private lateinit var tempDir: File
    private lateinit var configFile: File
    private lateinit var cryptoProvider: JvmSoftwareCryptoProvider
    private lateinit var repository: ConfigRepository

    @Before
    fun setUp() {
        tempDir = File(System.getProperty("java.io.tmpdir"), "agw_test_${System.currentTimeMillis()}")
        tempDir.mkdirs()
        configFile = File(tempDir, "config.json")
        cryptoProvider = JvmSoftwareCryptoProvider()
        repository = ConfigRepository(configFile, cryptoProvider)
    }

    @After
    fun tearDown() {
        tempDir.deleteRecursively()
    }

    @Test
    fun testInitialConfigCreation() {
        val config = repository.load()
        assertNotNull(config)
        assertEquals(1, config.providers.size)
        assertEquals("供应商 1", config.providers[0].name)
        assertTrue(config.downstreamKey.startsWith("sk-agw-"))
        assertEquals(config.currentProviderId, config.providers[0].id)
        assertTrue(configFile.exists())
    }

    @Test
    fun testCryptoProviderEncryptionAndDecryption() {
        val plain = "sk-test-secret-upstream-key-12345"
        val cipher = cryptoProvider.encrypt(plain)
        assertNotEquals(plain, cipher)
        val decrypted = cryptoProvider.decrypt(cipher)
        assertEquals(plain, decrypted)
    }

    @Test
    fun testProviderSwitchingAndDrafts() {
        // 1. Initial config
        val config1 = repository.load()
        val initialDownstreamKey = config1.downstreamKey

        // 2. Update provider 1 draft
        repository.updateCurrentProviderDraft("https://api.openai.com", "sk-openai-key")
        val updated1 = repository.load()
        assertEquals("https://api.openai.com", updated1.getCurrentProvider()?.upstreamUrl)
        assertEquals("sk-openai-key", updated1.getCurrentProvider()?.upstreamKey)
        assertEquals(initialDownstreamKey, updated1.downstreamKey)

        // 3. Create provider 2
        val (configWithP2, p2) = repository.createProvider("Anthropic Claude", "https://api.anthropic.com", "sk-claude-key")
        assertEquals(2, configWithP2.providers.size)
        assertEquals(p2.id, configWithP2.currentProviderId)
        assertEquals("Anthropic Claude", configWithP2.getCurrentProvider()?.name)
        assertEquals("https://api.anthropic.com", configWithP2.getCurrentProvider()?.upstreamUrl)
        // Downstream key remains unchanged!
        assertEquals(initialDownstreamKey, configWithP2.downstreamKey)

        // 4. Switch back to provider 1
        val p1Id = config1.providers[0].id
        val switchedBack = repository.selectProvider(p1Id)
        assertEquals(p1Id, switchedBack.currentProviderId)
        assertEquals("供应商 1", switchedBack.getCurrentProvider()?.name)
        assertEquals("https://api.openai.com", switchedBack.getCurrentProvider()?.upstreamUrl)
        assertEquals("sk-openai-key", switchedBack.getCurrentProvider()?.upstreamKey)
        // Downstream key still unchanged!
        assertEquals(initialDownstreamKey, switchedBack.downstreamKey)
    }

    @Test
    fun testDownstreamKeyRegenerationDoesNotAffectProviders() {
        repository.updateCurrentProviderDraft("https://api.openai.com", "sk-openai-key")
        val before = repository.load()
        val oldDownstreamKey = before.downstreamKey

        val after = repository.regenerateDownstreamKey()
        assertNotEquals(oldDownstreamKey, after.downstreamKey)
        assertTrue(after.downstreamKey.startsWith("sk-agw-"))

        // Providers are completely unaffected
        assertEquals(before.currentProviderId, after.currentProviderId)
        assertEquals("https://api.openai.com", after.getCurrentProvider()?.upstreamUrl)
        assertEquals("sk-openai-key", after.getCurrentProvider()?.upstreamKey)
    }

    @Test
    fun testDeleteProviderProtection() {
        val (_, p2) = repository.createProvider("Provider 2")
        val (c2, p3) = repository.createProvider("Provider 3")
        assertEquals(3, c2.providers.size)

        val afterDeleteP2 = repository.deleteProvider(p2.id)
        assertEquals(2, afterDeleteP2.providers.size)
        assertFalse(afterDeleteP2.providers.any { it.id == p2.id })

        // Cannot delete down to 0 providers
        val p1Id = afterDeleteP2.providers[0].id
        val afterDeleteP3 = repository.deleteProvider(p3.id)
        assertEquals(1, afterDeleteP3.providers.size)
        assertEquals(p1Id, afterDeleteP3.providers[0].id)

        // Attempting to delete the last one should be ignored
        val afterDeleteLast = repository.deleteProvider(p1Id)
        assertEquals(1, afterDeleteLast.providers.size)
        assertEquals(p1Id, afterDeleteLast.providers[0].id)
    }

    @Test
    fun testRenameProvider() {
        val initial = repository.load()
        val p1 = initial.getCurrentProvider()!!
        assertEquals("供应商 1", p1.name)

        val renamed = repository.renameProvider(p1.id, "OpenAI-Official")
        assertEquals("OpenAI-Official", renamed.getCurrentProvider()?.name)

        // Re-load from persistence to ensure saved properly
        val reloaded = repository.load()
        assertEquals("OpenAI-Official", reloaded.getCurrentProvider()?.name)
    }

    @Test
    fun testCorruptedFileRecovery() {
        configFile.writeText("{ corrupted_json !!!")
        val recovered = repository.load()
        assertNotNull(recovered)
        assertEquals(1, recovered.providers.size)
        assertTrue(recovered.downstreamKey.startsWith("sk-agw-"))
    }
}

package org.antigravity.gateway

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.antigravity.gateway.net.GatewayTestClient
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test

class GatewayTestClientTest {

    private lateinit var mockServer: MockWebServer
    private var testPort: Int = 0

    @Before
    fun setUp() {
        mockServer = MockWebServer()
        mockServer.start()
        testPort = mockServer.port
    }

    @After
    fun tearDown() {
        mockServer.shutdown()
    }

    @Test
    fun testFetchModelsSuccess() = runBlocking {
        val responseBody = """
            {
                "object": "list",
                "data": [
                    {"id": "gpt-4o", "object": "model"},
                    {"id": "claude-3-5-sonnet", "object": "model"},
                    {"id": "gemini-1.5-pro", "object": "model"}
                ]
            }
        """.trimIndent()

        mockServer.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody(responseBody)
        )

        val result = GatewayTestClient.fetchModels("sk-agw-test12345", testPort)
        assertTrue(result.isSuccess)
        val models = result.getOrNull()
        assertNotNull(models)
        assertEquals(3, models!!.size)
        assertEquals("gpt-4o", models[0])
        assertEquals("claude-3-5-sonnet", models[1])
        assertEquals("gemini-1.5-pro", models[2])

        val recorded = mockServer.takeRequest()
        assertEquals("/v1/models", recorded.path)
        assertEquals("Bearer sk-agw-test12345", recorded.getHeader("Authorization"))
    }

    @Test
    fun testFetchModelsError() = runBlocking {
        val errorBody = """
            {
                "error": {
                    "message": "Invalid API key provided",
                    "type": "invalid_request_error"
                }
            }
        """.trimIndent()

        mockServer.enqueue(
            MockResponse()
                .setResponseCode(401)
                .setHeader("Content-Type", "application/json")
                .setBody(errorBody)
        )

        val result = GatewayTestClient.fetchModels("bad-key", testPort)
        assertTrue(result.isFailure)
        val err = result.exceptionOrNull()?.message
        assertNotNull(err)
        assertTrue(err!!.contains("Invalid API key provided"))
    }

    @Test
    fun testChatMessageSuccess() = runBlocking {
        val responseBody = """
            {
                "id": "chatcmpl-test",
                "object": "chat.completion",
                "choices": [
                    {
                        "index": 0,
                        "message": {
                            "role": "assistant",
                            "content": "我是 Antigravity Gateway 测试助手。"
                        },
                        "finish_reason": "stop"
                    }
                ]
            }
        """.trimIndent()

        mockServer.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody(responseBody)
        )

        val result = GatewayTestClient.testChatMessage("sk-agw-test12345", "gpt-4o", port = testPort)
        assertTrue(result.isSuccess)
        val reply = result.getOrNull()
        assertEquals("我是 Antigravity Gateway 测试助手。", reply)

        val recorded = mockServer.takeRequest()
        assertEquals("/v1/chat/completions", recorded.path)
        assertEquals("Bearer sk-agw-test12345", recorded.getHeader("Authorization"))
        val requestBody = recorded.body.readUtf8()
        assertTrue(requestBody.contains("gpt-4o"))
        assertTrue(requestBody.contains("你好"))
    }

    @Test
    fun testChatMessageUpstreamError() = runBlocking {
        val errorBody = """
            {
                "error": {
                    "message": "Model 'nonexistent' not found"
                }
            }
        """.trimIndent()

        mockServer.enqueue(
            MockResponse()
                .setResponseCode(404)
                .setHeader("Content-Type", "application/json")
                .setBody(errorBody)
        )

        val result = GatewayTestClient.testChatMessage("sk-agw-test12345", "nonexistent", port = testPort)
        assertTrue(result.isFailure)
        val err = result.exceptionOrNull()?.message
        assertNotNull(err)
        assertTrue(err!!.contains("Model 'nonexistent' not found"))
    }
}

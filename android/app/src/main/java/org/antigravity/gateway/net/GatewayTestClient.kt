package org.antigravity.gateway.net

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.BufferedReader
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URL

object GatewayTestClient {

    private const val TIMEOUT_MS = 15000

    suspend fun fetchModels(downstreamKey: String, port: Int = 38472): Result<List<String>> =
        withContext(Dispatchers.IO) {
            var conn: HttpURLConnection? = null
            try {
                val url = URL("http://127.0.0.1:$port/v1/models")
                conn = (url.openConnection() as HttpURLConnection).apply {
                    requestMethod = "GET"
                    connectTimeout = TIMEOUT_MS
                    readTimeout = TIMEOUT_MS
                    setRequestProperty("Authorization", "Bearer $downstreamKey")
                    setRequestProperty("Accept", "application/json")
                }

                val responseCode = conn.responseCode
                val stream = if (responseCode in 200..299) conn.inputStream else conn.errorStream
                val body = BufferedReader(InputStreamReader(stream, Charsets.UTF_8)).use { it.readText() }

                if (responseCode !in 200..299) {
                    val errMsg = parseErrorDetail(body) ?: "HTTP $responseCode"
                    return@withContext Result.failure(RuntimeException("拉取模型失败: $errMsg"))
                }

                val json = JSONObject(body)
                val dataArray = json.optJSONArray("data")
                val models = mutableListOf<String>()
                if (dataArray != null) {
                    for (i in 0 until dataArray.length()) {
                        val obj = dataArray.optJSONObject(i)
                        val id = obj?.optString("id")
                        if (!id.isNullOrBlank()) {
                            models.add(id)
                        }
                    }
                }
                Result.success(models)
            } catch (e: Exception) {
                Result.failure(RuntimeException("连接本地网关失败: ${e.message}"))
            } finally {
                conn?.disconnect()
            }
        }

    suspend fun testChatMessage(
        downstreamKey: String,
        modelId: String,
        prompt: String = "你好，请用一句话介绍你自己。",
        port: Int = 38472
    ): Result<String> = withContext(Dispatchers.IO) {
        var conn: HttpURLConnection? = null
        try {
            val url = URL("http://127.0.0.1:$port/v1/chat/completions")
            conn = (url.openConnection() as HttpURLConnection).apply {
                requestMethod = "POST"
                connectTimeout = TIMEOUT_MS
                readTimeout = TIMEOUT_MS
                doOutput = true
                setRequestProperty("Content-Type", "application/json; charset=utf-8")
                setRequestProperty("Authorization", "Bearer $downstreamKey")
                setRequestProperty("Accept", "application/json")
            }

            val requestJson = JSONObject().apply {
                put("model", modelId)
                put("stream", false)
                val messages = org.json.JSONArray().apply {
                    put(JSONObject().apply {
                        put("role", "user")
                        put("content", prompt)
                    })
                }
                put("messages", messages)
            }

            OutputStreamWriter(conn.outputStream, Charsets.UTF_8).use {
                it.write(requestJson.toString())
                it.flush()
            }

            val responseCode = conn.responseCode
            val stream = if (responseCode in 200..299) conn.inputStream else conn.errorStream
            val body = BufferedReader(InputStreamReader(stream, Charsets.UTF_8)).use { it.readText() }

            if (responseCode !in 200..299) {
                val errMsg = parseErrorDetail(body) ?: "HTTP $responseCode"
                return@withContext Result.failure(RuntimeException("测试失败 ($responseCode): $errMsg"))
            }

            val json = JSONObject(body)
            val choices = json.optJSONArray("choices")
            val firstChoice = choices?.optJSONObject(0)
            val message = firstChoice?.optJSONObject("message")
            val content = message?.optString("content")?.trim() ?: ""

            if (content.isEmpty()) {
                val toolCalls = message?.optJSONArray("tool_calls")
                if (toolCalls != null && toolCalls.length() > 0) {
                    Result.success("[Tool Call] ${toolCalls.getJSONObject(0).optJSONObject("function")?.optString("name")}")
                } else {
                    Result.success("[响应为空]")
                }
            } else {
                Result.success(content)
            }
        } catch (e: Exception) {
            Result.failure(RuntimeException("请求异常: ${e.message}"))
        } finally {
            conn?.disconnect()
        }
    }

    private fun parseErrorDetail(body: String): String? {
        if (body.isBlank()) return null
        return try {
            val root = JSONObject(body)
            val err = root.optJSONObject("error")
            when {
                err != null && err.has("message") -> err.optString("message")
                root.has("message") -> root.optString("message")
                else -> null
            }
        } catch (_: Exception) {
            body.take(200)
        }
    }
}

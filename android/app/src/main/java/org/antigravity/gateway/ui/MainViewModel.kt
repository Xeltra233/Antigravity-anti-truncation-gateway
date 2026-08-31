package org.antigravity.gateway.ui

import android.app.Application
import android.content.Context
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import org.antigravity.gateway.data.AndroidKeystoreCryptoProvider
import org.antigravity.gateway.data.ConfigRepository
import org.antigravity.gateway.data.GatewayConfig
import org.antigravity.gateway.data.Provider
import org.antigravity.gateway.net.GatewayTestClient
import org.antigravity.gateway.service.GatewayServiceController
import org.antigravity.gateway.util.NetworkUtils
import java.io.File

enum class GatewayState {
    IDLE,
    STARTING,
    RUNNING,
    STOPPING,
    ERROR
}

data class UiState(
    val config: GatewayConfig,
    val gatewayState: GatewayState = GatewayState.IDLE,
    val appLink: String = "http://127.0.0.1:38472/v1",
    val statusMessage: String = "",
    val testResult: String = "网关未启动",
    val isTesting: Boolean = false,
    val lastFetchedModels: List<String> = emptyList(),
    val keyNeedsRestart: Boolean = false
)

class MainViewModel(application: Application) : AndroidViewModel(application) {

    private val repository: ConfigRepository by lazy {
        val file = File(application.filesDir, ConfigRepository.DEFAULT_CONFIG_FILE_NAME)
        ConfigRepository(file, AndroidKeystoreCryptoProvider())
    }

    private val _uiState = MutableStateFlow(
        UiState(
            config = GatewayConfig(
                schemaVersion = 1,
                currentProviderId = "",
                downstreamKey = "",
                providers = emptyList()
            )
        )
    )
    val uiState: StateFlow<UiState> = _uiState.asStateFlow()

    init {
        loadConfigAndNetwork()
        observeServiceState()
    }

    private fun observeServiceState() {
        viewModelScope.launch {
            GatewayServiceController.serviceState.collect { state ->
                val defaultTestText = when (state) {
                    GatewayState.RUNNING -> if (_uiState.value.testResult == "网关未启动" || _uiState.value.testResult.startsWith("网关正在停止")) "网关运行中，可执行拉取模型或消息测试" else _uiState.value.testResult
                    GatewayState.IDLE -> "网关未启动"
                    GatewayState.STARTING -> "正在启动网关..."
                    GatewayState.STOPPING -> "正在停止网关..."
                    GatewayState.ERROR -> _uiState.value.testResult
                }
                _uiState.value = _uiState.value.copy(
                    gatewayState = state,
                    testResult = defaultTestText,
                    keyNeedsRestart = if (state == GatewayState.RUNNING) _uiState.value.keyNeedsRestart else false
                )
            }
        }
        viewModelScope.launch {
            GatewayServiceController.statusMessage.collect { msg ->
                if (msg.isNotEmpty()) {
                    _uiState.value = _uiState.value.copy(statusMessage = msg)
                }
            }
        }
    }

    fun loadConfigAndNetwork() {
        viewModelScope.launch(Dispatchers.IO) {
            val config = repository.load()
            val appLink = NetworkUtils.buildAppLink()
            _uiState.value = _uiState.value.copy(
                config = config,
                appLink = appLink
            )
        }
    }

    fun updateDraft(url: String, key: String) {
        viewModelScope.launch(Dispatchers.IO) {
            val updated = repository.updateCurrentProviderDraft(url, key)
            _uiState.value = _uiState.value.copy(config = updated)
        }
    }

    fun selectProvider(providerId: String) {
        if (_uiState.value.gatewayState == GatewayState.RUNNING || _uiState.value.gatewayState == GatewayState.STARTING) {
            _uiState.value = _uiState.value.copy(statusMessage = "网关运行中，切换供应商需先停止")
            return
        }
        viewModelScope.launch(Dispatchers.IO) {
            val updated = repository.selectProvider(providerId)
            _uiState.value = _uiState.value.copy(
                config = updated,
                statusMessage = "已切换至: ${updated.getCurrentProvider()?.name}"
            )
        }
    }

    fun createProvider(name: String) {
        viewModelScope.launch(Dispatchers.IO) {
            val (updated, newProvider) = repository.createProvider(name)
            _uiState.value = _uiState.value.copy(
                config = updated,
                statusMessage = "已创建并切换至: ${newProvider.name}"
            )
        }
    }

    fun renameProvider(providerId: String, newName: String) {
        val trimmed = newName.trim()
        if (trimmed.isEmpty()) return
        viewModelScope.launch(Dispatchers.IO) {
            val updated = repository.renameProvider(providerId, trimmed)
            _uiState.value = _uiState.value.copy(config = updated, statusMessage = "已重命名为: $trimmed")
        }
    }

    fun deleteProvider(providerId: String) {
        viewModelScope.launch(Dispatchers.IO) {
            val updated = repository.deleteProvider(providerId)
            _uiState.value = _uiState.value.copy(config = updated, statusMessage = "已删除供应商")
        }
    }

    fun regenerateDownstreamKey() {
        viewModelScope.launch(Dispatchers.IO) {
            val updated = repository.regenerateDownstreamKey()
            val isRunning = _uiState.value.gatewayState == GatewayState.RUNNING
            _uiState.value = _uiState.value.copy(
                config = updated,
                keyNeedsRestart = isRunning,
                statusMessage = if (isRunning) "下游 Key 已更新（重启网关后生效）" else "已生成新的下游 Key"
            )
        }
    }

    fun startGateway(context: Context) {
        val currentProvider = _uiState.value.config.getCurrentProvider()
        val url = currentProvider?.upstreamUrl?.trim() ?: ""
        val key = currentProvider?.upstreamKey?.trim() ?: ""
        val downstream = _uiState.value.config.downstreamKey

        if (url.isEmpty()) {
            _uiState.value = _uiState.value.copy(statusMessage = "请输入上游 URL")
            return
        }
        if (key.isEmpty()) {
            _uiState.value = _uiState.value.copy(statusMessage = "请输入上游 Key")
            return
        }

        GatewayServiceController.start(
            context = context,
            upstreamUrl = url,
            upstreamKey = key,
            downstreamKey = downstream,
            port = NetworkUtils.GATEWAY_PORT
        )
    }

    fun stopGateway(context: Context) {
        GatewayServiceController.stop(context)
    }

    fun fetchModels() {
        if (_uiState.value.gatewayState != GatewayState.RUNNING) {
            _uiState.value = _uiState.value.copy(statusMessage = "网关尚未运行")
            return
        }

        _uiState.value = _uiState.value.copy(isTesting = true, testResult = "正在拉取上游模型列表...")
        viewModelScope.launch {
            val result = GatewayTestClient.fetchModels(_uiState.value.config.downstreamKey)
            if (result.isSuccess) {
                val models = result.getOrNull() ?: emptyList()
                val text = if (models.isEmpty()) {
                    "成功连接，但上游未返回任何模型 ID"
                } else {
                    "✅ 成功获取 ${models.size} 个模型:\n" + models.take(30).joinToString("\n") { "• $it" } +
                            if (models.size > 30) "\n... (以及其余 ${models.size - 30} 个)" else ""
                }
                _uiState.value = _uiState.value.copy(
                    isTesting = false,
                    testResult = text,
                    lastFetchedModels = models
                )
            } else {
                val err = result.exceptionOrNull()?.message ?: "未知错误"
                _uiState.value = _uiState.value.copy(
                    isTesting = false,
                    testResult = "❌ $err"
                )
            }
        }
    }

    fun testMessage(customModelId: String = "") {
        if (_uiState.value.gatewayState != GatewayState.RUNNING) {
            _uiState.value = _uiState.value.copy(statusMessage = "网关尚未运行")
            return
        }

        val targetModel = customModelId.trim().ifEmpty {
            _uiState.value.lastFetchedModels.firstOrNull() ?: "gpt-4o"
        }

        _uiState.value = _uiState.value.copy(
            isTesting = true,
            testResult = "正在向模型「$targetModel」发送测试消息..."
        )

        viewModelScope.launch {
            val result = GatewayTestClient.testChatMessage(
                downstreamKey = _uiState.value.config.downstreamKey,
                modelId = targetModel
            )
            if (result.isSuccess) {
                val reply = result.getOrNull() ?: ""
                _uiState.value = _uiState.value.copy(
                    isTesting = false,
                    testResult = "✅ [$targetModel 回复]:\n$reply"
                )
            } else {
                val err = result.exceptionOrNull()?.message ?: "未知错误"
                _uiState.value = _uiState.value.copy(
                    isTesting = false,
                    testResult = "❌ $err"
                )
            }
        }
    }

    fun clearStatusMessage() {
        _uiState.value = _uiState.value.copy(statusMessage = "")
    }
}

package org.antigravity.gateway.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.antigravity.gateway.R
import org.antigravity.gateway.bridge.GatewayBridge
import org.antigravity.gateway.ui.GatewayState
import org.antigravity.gateway.ui.MainActivity
import java.net.HttpURLConnection
import java.net.URL

object GatewayServiceController {
    private val _serviceState = MutableStateFlow(GatewayState.IDLE)
    val serviceState: StateFlow<GatewayState> = _serviceState.asStateFlow()

    private val _statusMessage = MutableStateFlow("")
    val statusMessage: StateFlow<String> = _statusMessage.asStateFlow()

    fun updateState(state: GatewayState, message: String = "") {
        _serviceState.value = state
        _statusMessage.value = message
    }

    fun start(
        context: Context,
        upstreamUrl: String,
        upstreamKey: String,
        downstreamKey: String,
        port: Int = 38472
    ) {
        val intent = Intent(context, GatewayService::class.java).apply {
            action = GatewayService.ACTION_START
            putExtra(GatewayService.EXTRA_UPSTREAM_URL, upstreamUrl)
            putExtra(GatewayService.EXTRA_UPSTREAM_KEY, upstreamKey)
            putExtra(GatewayService.EXTRA_DOWNSTREAM_KEY, downstreamKey)
            putExtra(GatewayService.EXTRA_PORT, port)
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            context.startForegroundService(intent)
        } else {
            context.startService(intent)
        }
    }

    fun stop(context: Context) {
        val intent = Intent(context, GatewayService::class.java).apply {
            action = GatewayService.ACTION_STOP
        }
        context.startService(intent)
    }
}

class GatewayService : Service() {

    companion object {
        const val ACTION_START = "org.antigravity.gateway.action.START"
        const val ACTION_STOP = "org.antigravity.gateway.action.STOP"

        const val EXTRA_UPSTREAM_URL = "extra_upstream_url"
        const val EXTRA_UPSTREAM_KEY = "extra_upstream_key"
        const val EXTRA_DOWNSTREAM_KEY = "extra_downstream_key"
        const val EXTRA_PORT = "extra_port"

        private const val NOTIFICATION_ID = 1001
        private const val CHANNEL_ID = "gateway_foreground_service"
    }

    private val serviceScope = CoroutineScope(Dispatchers.IO + Job())

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> {
                val url = intent.getStringExtra(EXTRA_UPSTREAM_URL) ?: ""
                val upKey = intent.getStringExtra(EXTRA_UPSTREAM_KEY) ?: ""
                val downKey = intent.getStringExtra(EXTRA_DOWNSTREAM_KEY) ?: ""
                val port = intent.getIntExtra(EXTRA_PORT, 38472)

                startGatewayService(url, upKey, downKey, port)
            }
            ACTION_STOP -> {
                stopGatewayService()
            }
            else -> {
                if (GatewayBridge.isRunning()) {
                    GatewayServiceController.updateState(GatewayState.RUNNING)
                } else {
                    stopSelf()
                }
            }
        }
        return START_NOT_STICKY
    }

    private fun startGatewayService(url: String, upKey: String, downKey: String, port: Int) {
        val notification = buildNotification("网关正在启动...")
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(
                NOTIFICATION_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE
            )
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }

        GatewayServiceController.updateState(GatewayState.STARTING, "正在启动网关核心...")

        serviceScope.launch {
            val result = GatewayBridge.startGateway(url, upKey, downKey, port)
            if (result.isSuccess) {
                // Poll healthz to ensure port is actively listening
                var isHealthy = false
                for (i in 1..30) {
                    kotlinx.coroutines.delay(100)
                    if (checkHealth(port)) {
                        isHealthy = true
                        break
                    }
                }

                if (isHealthy) {
                    updateNotification("网关正在监听端口 $port")
                    GatewayServiceController.updateState(GatewayState.RUNNING, "网关已就绪")
                } else {
                    GatewayBridge.stopGateway()
                    stopForeground(STOP_FOREGROUND_REMOVE)
                    GatewayServiceController.updateState(GatewayState.ERROR, "启动失败: 服务就绪超时或端口被占用")
                    stopSelf()
                }
            } else {
                val err = result.exceptionOrNull()?.message ?: "未知错误"
                stopForeground(STOP_FOREGROUND_REMOVE)
                GatewayServiceController.updateState(GatewayState.ERROR, "启动失败: $err")
                stopSelf()
            }
        }
    }

    private fun stopGatewayService() {
        GatewayServiceController.updateState(GatewayState.STOPPING, "正在停止...")
        serviceScope.launch {
            GatewayBridge.stopGateway()
            withContext(Dispatchers.Main) {
                stopForeground(STOP_FOREGROUND_REMOVE)
                GatewayServiceController.updateState(GatewayState.IDLE, "网关已停止")
                stopSelf()
            }
        }
    }

    override fun onTaskRemoved(rootIntent: Intent?) {
        super.onTaskRemoved(rootIntent)
        // User swiped app away from Recents -> Stop gateway cleanly
        GatewayBridge.stopGateway()
        stopForeground(STOP_FOREGROUND_REMOVE)
        GatewayServiceController.updateState(GatewayState.IDLE)
        stopSelf()
    }

    override fun onDestroy() {
        super.onDestroy()
        GatewayBridge.stopGateway()
        GatewayServiceController.updateState(GatewayState.IDLE)
    }

    private fun checkHealth(port: Int): Boolean {
        return try {
            val url = URL("http://127.0.0.1:$port/healthz")
            val conn = url.openConnection() as HttpURLConnection
            conn.connectTimeout = 500
            conn.readTimeout = 500
            conn.requestMethod = "GET"
            val code = conn.responseCode
            conn.disconnect()
            code == 200
        } catch (_: Exception) {
            false
        }
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                getString(R.string.notification_channel_name),
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = getString(R.string.notification_channel_desc)
                setShowBadge(false)
            }
            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(channel)
        }
    }

    private fun buildNotification(contentText: String): Notification {
        val pendingIntent = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(contentText)
            .setSmallIcon(R.mipmap.ic_launcher)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    private fun updateNotification(contentText: String) {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        manager.notify(NOTIFICATION_ID, buildNotification(contentText))
    }
}

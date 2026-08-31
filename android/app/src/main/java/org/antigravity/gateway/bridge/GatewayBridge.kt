package org.antigravity.gateway.bridge

object GatewayBridge {
    private var isLoaded = false
    private var loadError: Throwable? = null

    init {
        try {
            System.loadLibrary("antigravity")
            isLoaded = true
        } catch (t: Throwable) {
            loadError = t
        }
    }

    fun isNativeLoaded(): Boolean = isLoaded
    fun getLoadError(): Throwable? = loadError

    @Synchronized
    fun startGateway(
        upstreamUrl: String,
        upstreamKey: String,
        downstreamKey: String,
        port: Int = 38472
    ): Result<Unit> {
        if (!isLoaded) {
            return Result.failure(IllegalStateException("Native library not loaded: ${loadError?.message}"))
        }
        val code = nativeStartGateway(upstreamUrl, upstreamKey, downstreamKey, port)
        return if (code == 0) {
            Result.success(Unit)
        } else {
            val err = nativeGetLastError()
            Result.failure(RuntimeException(if (err.isNullOrEmpty()) "Failed to start gateway (code $code)" else err))
        }
    }

    @Synchronized
    fun stopGateway(): Result<Unit> {
        if (!isLoaded) {
            return Result.failure(IllegalStateException("Native library not loaded: ${loadError?.message}"))
        }
        val code = nativeStopGateway()
        return if (code == 0) {
            Result.success(Unit)
        } else {
            val err = nativeGetLastError()
            Result.failure(RuntimeException(if (err.isNullOrEmpty()) "Failed to stop gateway (code $code)" else err))
        }
    }

    @Synchronized
    fun isRunning(): Boolean {
        if (!isLoaded) return false
        return nativeIsRunning()
    }

    @Synchronized
    fun getLastError(): String {
        if (!isLoaded) return loadError?.message ?: "Library not loaded"
        return nativeGetLastError() ?: ""
    }

    @JvmStatic
    private external fun nativeStartGateway(
        upstreamUrl: String,
        upstreamKey: String,
        downstreamKey: String,
        port: Int
    ): Int

    @JvmStatic
    private external fun nativeStopGateway(): Int

    @JvmStatic
    private external fun nativeIsRunning(): Boolean

    @JvmStatic
    private external fun nativeGetLastError(): String?
}

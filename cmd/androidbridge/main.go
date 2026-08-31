//go:build android || cgo_jni

package main

/*
#include <jni.h>
#include <stdlib.h>

static const char* get_jstring_chars(JNIEnv *env, jstring jstr) {
    if (!jstr) return NULL;
    return (*env)->GetStringUTFChars(env, jstr, NULL);
}

static void release_jstring_chars(JNIEnv *env, jstring jstr, const char *chars) {
    if (jstr && chars) {
        (*env)->ReleaseStringUTFChars(env, jstr, chars);
    }
}

static jstring create_jstring(JNIEnv *env, const char *str) {
    if (!str) return NULL;
    return (*env)->NewStringUTF(env, str);
}
*/
import "C"
import (
	"strconv"
	"sync"
	"unsafe"

	"antigravity-gateway/pkg/gateway"
)

var (
	engineMu     sync.Mutex
	activeEngine *gateway.Engine
	lastError    string
)

func extractJString(env *C.JNIEnv, jstr C.jstring) string {
	cStr := C.get_jstring_chars(env, jstr)
	if cStr == nil {
		return ""
	}
	defer C.release_jstring_chars(env, jstr, cStr)
	return C.GoString(cStr)
}

func createJString(env *C.JNIEnv, str string) C.jstring {
	cStr := C.CString(str)
	defer C.free(unsafe.Pointer(cStr))
	return C.create_jstring(env, cStr)
}

//export Java_org_antigravity_gateway_bridge_GatewayBridge_nativeStartGateway
func Java_org_antigravity_gateway_bridge_GatewayBridge_nativeStartGateway(
	env *C.JNIEnv,
	clazz C.jclass,
	jUpstreamUrl C.jstring,
	jUpstreamKey C.jstring,
	jDownstreamKey C.jstring,
	jPort C.jint,
) C.jint {
	engineMu.Lock()
	defer engineMu.Unlock()

	if activeEngine != nil && activeEngine.IsRunning() {
		return 0 // Already running
	}

	upstreamUrl := extractJString(env, jUpstreamUrl)
	upstreamKey := extractJString(env, jUpstreamKey)
	downstreamKey := extractJString(env, jDownstreamKey)
	port := int(jPort)
	if port <= 0 {
		port = 38472
	}

	if upstreamUrl == "" || upstreamKey == "" {
		lastError = "upstream URL and upstream Key are required"
		return 1
	}

	envMap := map[string]string{
		"UPSTREAM_BASE_URL":  upstreamUrl,
		"UPSTREAM_API_KEY":   upstreamKey,
		"UPSTREAM_AUTH_MODE": "bearer",
		"API_KEY":            downstreamKey,
		"KEY_DB_PATH":        ":memory:",
		"HOST":               "0.0.0.0",
		"PORT":               strconv.Itoa(port),
		"WRAPPER_MODE":       "prefer",
		"RECOVERY_POLICY":    "repair",
	}

	getenv := func(key string) string {
		return envMap[key]
	}

	engine, err := gateway.StartEngine(getenv)
	if err != nil {
		lastError = err.Error()
		return 2
	}

	activeEngine = engine
	lastError = ""
	return 0
}

//export Java_org_antigravity_gateway_bridge_GatewayBridge_nativeStopGateway
func Java_org_antigravity_gateway_bridge_GatewayBridge_nativeStopGateway(
	env *C.JNIEnv,
	clazz C.jclass,
) C.jint {
	engineMu.Lock()
	defer engineMu.Unlock()

	if activeEngine == nil {
		return 0
	}

	err := activeEngine.Stop()
	activeEngine = nil
	if err != nil {
		lastError = err.Error()
		return 1
	}
	lastError = ""
	return 0
}

//export Java_org_antigravity_gateway_bridge_GatewayBridge_nativeIsRunning
func Java_org_antigravity_gateway_bridge_GatewayBridge_nativeIsRunning(
	env *C.JNIEnv,
	clazz C.jclass,
) C.jboolean {
	engineMu.Lock()
	defer engineMu.Unlock()

	if activeEngine != nil && activeEngine.IsRunning() {
		return 1
	}
	return 0
}

//export Java_org_antigravity_gateway_bridge_GatewayBridge_nativeGetLastError
func Java_org_antigravity_gateway_bridge_GatewayBridge_nativeGetLastError(
	env *C.JNIEnv,
	clazz C.jclass,
) C.jstring {
	engineMu.Lock()
	errStr := lastError
	engineMu.Unlock()

	return createJString(env, errStr)
}

func main() {}

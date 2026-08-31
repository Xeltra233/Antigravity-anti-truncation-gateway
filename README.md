# Antigravity Gateway for Android (防截断兼容网关 Android 移动端)

<p align="center">
  <img src="android/app/src/main/res/mipmap-xxxhdpi/ic_launcher.png" width="96" alt="App Icon" />
</p>

---

## 📑 目录 (Table of Contents)

- [🇨🇳 中文说明](#-中文说明)
  - [项目简介](#-项目简介)
  - [🌟 核心特性](#-核心特性)
  - [📱 界面与功能使用](#-界面与功能使用)
  - [⚙️ 技术架构与安全性](#️-技术架构与安全性)
  - [🛠️ 本地编译与构建](#️-本地编译与构建)
- [🇬🇧 English Documentation](#-english-documentation)
  - [Overview](#overview)
  - [🌟 Key Features](#-key-features)
  - [📱 UI & Usage Guide](#-ui--usage-guide)
  - [⚙️ Technical Architecture & Security](#️-technical-architecture--security)
  - [🛠️ Build from Source](#️-build-from-source)
- [📄 License](#-license)

---

<a name="中文说明"></a>
## 🇨🇳 中文说明

### 📖 项目简介

**Antigravity Gateway for Android**（包名 `org.antigravity.gateway`）是专为 Android 平台定制的极简本地 OpenAI Chat Completions 防截断代理网关。

移动端通过 JNI 内嵌编译好的 Go 原生核心动态库（`libantigravity.so`），无需依赖任何云端中转或外部服务器，直接在 Android 手机/平板/车机/模拟器本地启动防截断网关，彻底解决 **SillyTavern 酒馆、Cherry Studio、NextChat** 等客户端在移动设备上游玩时的回答截断、Gemini/CPA 轮次 400 报错及未闭合 Markdown/JSON 代码块问题。

---

### 🌟 核心特性

1. **极简单屏交互**：
   - 专为移动端设计的单屏 Material Design 界面，无繁琐层级，所有关键信息与操作一屏触达。
2. **多供应商独立管理**：
   - 支持多套上游供应商配置自由切换与新建；
   - 支持通过卡片右侧 ✏️ 编辑图标或长按供应商名称直接重命名；
   - 具备最后一个供应商删除保护机制，防止误删配置。
3. **全局独立下游 Key**：
   - 首次启动自动生成高熵 `sk-agw-...` 密钥，独立于各个供应商；
   - 切换或修改供应商完全不影响下游客户端配置；
   - 支持重新生成并一键复制到剪贴板，支持明文/密文切换查看。
4. **系统级 Keystore 硬件加密**：
   - 所有上游 API Key 与全局下游 Key 均采用 **Android Keystore AES-GCM** 加密后持久化存储；
   - 绝不向外部存储、明文 SharedPreferences 或系统日志泄露密钥。
5. **固定冷门端口与局域网应用链接**：
   - 进程绑定监听冷门固定端口 `38472`（`HOST=0.0.0.0`）；
   - 界面自动识别并显示本机局域网 IP 与应用链接（`http://<IP>:38472/v1`），支持一键复制到酒馆或客户端中。
6. **前台服务后台保活 & 划走停机**：
   - 启动网关后自动挂载常驻前台服务通知，退至后台、息屏或切换 App 时代理服务不中断；
   - 在系统多任务卡片中划走应用或在界面点击「停止」时，网关优雅退出并释放端口占用。
7. **内置直连验证客户端**：
   - 界面内置「拉取模型」（`GET /v1/models`）与「消息测试」（`POST /v1/chat/completions`）功能；
   - 拉取成功后自动回填首个可用模型 ID，点击即可完成端到端收发连通性测试。
8. **全 CPU 架构 ABI 适配**：
   - 预编译打包全部 4 种 Android ABI：`arm64-v8a`、`x86_64`、`armeabi-v7a`、`x86`。

---

### 📱 界面与功能使用

#### 1. 配置供应商
- 点击「切换/新建」添加新供应商或切换已有供应商；
- 点击供应商名称右侧的 ✏️ 图标或长按名称可重命名当前供应商；
- 填入对应供应商的 **上游 URL** 与 **上游 Key**。

#### 2. 配置下游与启动
- 在「全局下游 Key」区域点击「生成」生成专属 Key，或点击「复制」将其填入客户端；
- 点击蓝色 **「启动」** 按钮，状态变为「停止」且通知栏显示前台保活服务，应用链接高亮显示；
- 复制界面展示的 **应用链接**（例如 `http://192.168.1.100:38472/v1`）填入客户端的 Base URL。

#### 3. 客户端接入示例 (SillyTavern / Cherry Studio)
- **API 类型**: `OpenAI / Chat Completions`
- **Base URL / 自定义端点**: `http://<手机IP>:38472/v1`（同设备运行填 `http://127.0.0.1:38472/v1`）
- **API Key**: 填入应用内复制的全局下游 Key（如 `sk-agw-...`）
- **模型**: 选择或输入上游原生模型名称（如 `gemini-3.5-flash-low`, `claude-sonnet-4-6` 等）

---

### ⚙️ 技术架构与安全性

```
┌────────────────────────────────────────────────────────┐
│                   Android Application                  │
│  ┌───────────────────────┐  ┌───────────────────────┐  │
│  │   UI (Material 3)     │  │  GatewayService       │  │
│  │   MainActivity / VM   │  │  Foreground KeepAlive │  │
│  └───────────┬───────────┘  └───────────┬───────────┘  │
│              │                          │              │
│  ┌───────────▼───────────┐              │              │
│  │   ConfigRepository    │              │              │
│  │   Keystore AES-GCM    │              │              │
│  └───────────────────────┘              │              │
│                                         │              │
│  ┌──────────────────────────────────────▼───────────┐  │
│  │ JNI Bridge (org.antigravity.gateway.bridge)      │  │
│  └──────────────────────┬───────────────────────────┘  │
└─────────────────────────┼──────────────────────────────┘
                          │ CGO / JNI
┌─────────────────────────▼──────────────────────────────┐
│  Go Native Core (libantigravity.so)                    │
│  • Synthetic Transport Tool Engine                     │
│  • Incremental SSE State Machine                       │
│  • Chat Turn Sanitizer & Bounded Repair                │
│  • Embedded SQLite (:memory:)                          │
│  • Listening on 0.0.0.0:38472                          │
└────────────────────────────────────────────────────────┘
```

- **数据安全**：所有配置采用系统级 Hardware-backed Keystore 提供的 AES-GCM-256 算法加密存储于应用内部隔离空间。
- **内存数据库**：Android 运行时将 Key 鉴权 SQLite 数据库配置为 `:memory:` 纯内存模式，杜绝文件系统持久化明文残留与权限异常。

---

### 🛠️ 本地编译与构建

#### 环境要求
- JDK 17+
- Android SDK (API 34, Build-Tools 34.0.0)
- NDK (26.3.11579264 或兼容版本)
- Go 1.24+ (如需重新交叉编译 JNI `.so` 库)

#### 构建步骤
```bash
# 1. 进入 Android 项目目录
cd android

# 2. 运行单元测试
./gradlew :app:testDebugUnitTest

# 3. 构建 Debug APK
./gradlew :app:assembleDebug

# 产物输出路径:
# android/app/build/outputs/apk/debug/app-debug.apk
```

---

<a name="english"></a>
## 🇬🇧 English Documentation

### Overview

**Antigravity Gateway for Android** (`org.antigravity.gateway`) is a standalone, lightweight Android application delivering an embedded OpenAI Chat Completions anti-truncation proxy gateway.

By embedding the high-performance Go native core (`libantigravity.so`) via JNI, the app acts as a local proxy on your Android phone, tablet, emulator, or head unit. It completely resolves truncation, missing code block enclosures, and Gemini/CPA trailing turn 400 errors for clients such as **SillyTavern, Cherry Studio, and NextChat**.

---

### 🌟 Key Features

1. **Minimal Single-Screen UI**: Clean Material interface designed specifically for mobile devices.
2. **Multi-Provider Switcher & Rename**: Add, switch, and rename providers (via ✏️ icon or long-press) with deletion safety for the default provider.
3. **Global Downstream Key**: Auto-generated high-entropy `sk-agw-...` key independent of upstream provider selections.
4. **Android Keystore AES-GCM Encryption**: Hardware-backed credential encryption preventing sensitive API keys from leaking into logs or storage.
5. **Fixed Cold Port & LAN Detection**: Fixed cold port `38472` with auto-detected LAN link (`http://<LAN-IP>:38472/v1`).
6. **Foreground Service Keep-Alive**: Persistent notification keeps the proxy running in the background; swiping the app away from recent tasks automatically shuts down the gateway and frees the port.
7. **Built-in Verification Client**: In-app `GET /v1/models` and `POST /v1/chat/completions` testing with auto-fill for the first available model.
8. **Multi-ABI Support**: Packaged with native libraries for `arm64-v8a`, `x86_64`, `armeabi-v7a`, and `x86`.

---

### 📱 UI & Usage Guide

1. **Configure Provider**: Select or add a provider, rename if needed, and enter the upstream URL and Key.
2. **Downstream Key**: Generate or copy the global downstream key.
3. **Start Gateway**: Tap **Start**, then copy the displayed App URL (`http://<IP>:38472/v1`).
4. **Client Setup**: Configure your client (e.g. SillyTavern) with the App URL as Base URL, the Downstream Key as API Key, and your chosen model name.

---

### ⚙️ Technical Architecture & Security

- **Native JNI Engine**: Core routing, synthetic transport protocol, and streaming state machine execute natively via `libantigravity.so`.
- **In-Memory SQLite**: The Android bridge initializes SQLite with `:memory:` to ensure no plaintext key residue exists on disk.
- **Keystore Encryption**: All credentials stored in `ConfigRepository` are encrypted with AES-256-GCM using keys generated inside the Android Keystore.

---

### 🛠️ Build from Source

```bash
cd android
./gradlew :app:testDebugUnitTest
./gradlew :app:assembleDebug
# Output APK: android/app/build/outputs/apk/debug/app-debug.apk
```

---

<a name="license"></a>
## 📄 License

This project is licensed under the [MIT License](LICENSE).
Copyright (c) 2026 Xeltra233.

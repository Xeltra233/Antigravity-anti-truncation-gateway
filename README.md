# Antigravity Anti-Truncation Gateway (OpenAI Chat 防截断兼容网关)

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/Architecture-Headless%20%7C%20High--Concurrency-blue.svg?style=flat-square" alt="Architecture">
  <img src="https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS%20%7C%20Docker-orange.svg?style=flat-square" alt="Platform">
</p>

---

## 📑 目录 (Table of Contents)

- [🇨🇳 中文说明](#-中文说明)
  - [项目简介与解决痛点](#-项目简介与解决痛点)
  - [🌟 核心工作原理与特性](#-核心工作原理与特性)
  - [📥 获取与运行方式（免安装/Docker/源码）](#-获取与运行方式按你的环境选择)
  - [⚙️ 配置详解与环境变量](#️-配置详解与环境变量)
  - [🚀 部署与运行方式 (Windows/Linux/Docker)](#-部署与运行方式)
  - [📱 客户端接入实战 (SillyTavern / Cherry Studio 等)](#-客户端接入实战)
  - [🛡️ 运维监控与健康检查](#️-运维监控与健康检查)
  - [❓ 常见问题排查 (FAQ)](#-常见问题排查-faq)
- [🇬🇧 English Documentation](#-english-documentation)
  - [Overview & Problems Solved](#overview--problems-solved)
  - [🌟 Key Architecture & Features](#-key-architecture--features)
  - [📥 Installation & Getting Started (No-Go / Docker / Source)](#-installation--getting-started)
  - [⚙️ Configuration & Environment Variables](#️-configuration--environment-variables)
  - [🚀 Deployment Methods (Windows / Linux / Docker / Systemd)](#-deployment-methods)
  - [📱 Client Integration (SillyTavern, Cherry Studio, etc.)](#-client-integration)
  - [🛡️ Health Checks & Observability](#️-health-checks--observability)
  - [❓ Frequently Asked Questions (FAQ)](#-frequently-asked-questions-faq)
- [📄 License](#-license)

---

<a name="中文说明"></a>
## 🇨🇳 中文说明

### 📖 项目简介与解决痛点

**Antigravity Anti-Truncation Gateway** 是一款专为 **OpenAI Chat Completions 协议** 设计的高性能、无 UI（Headless）、环境变量优先配置的防截断与格式修复中间件网关。

在重度提示词环境（例如 **SillyTavern 酒馆复杂预设、超长角色卡、Prompt Pre-fill 续写、工具调用协同**）下，主流大模型（Claude、Gemini、GPT 等）经常出现以下问题：
1. **输出意外截断**：长文本生成时受限于普通补全通道的输出长度限制或安全过滤器，导致生成中途断崖式截断；
2. **上游协议报错 (HTTP 400)**：如使用 Gemini / CPA 上游时，客户端包含 Assistant 预填或以非标准轮次收尾，触发 `Requests ending with a model turn are not supported.` 报错；
3. **格式坍塌与未闭合语法**：JSON/Markdown 代码块未闭合、遗漏引号、末尾悬空逗号，导致下游客户端解析崩溃；
4. **工具调用冲突**：在注入控制协议时污染了客户端自身的实际工具定义（如联网搜索、文件读取等）。

本网关通过**动态合成高熵工具协议（Synthetic Transport Tool）**、**流式增量 UTF-8/JSON 状态机**与**智能对话轮次自适应修复**，在完全不侵入客户端与上游原生模型 ID 的前提下，彻底解决上述痛点。

---

### 🌟 核心工作原理与特性

1. **防截断合成传输协议 (Synthetic Transport Tool)**：
   - 为每个请求动态生成高熵（96-bit 随机 Nonce）的传输工具（如 `agw_emit_7f3a91c8d42e6b1a9c02a547`）。
   - 引导模型将最终可见回答放入该工具参数中输出，利用底层 Function Call 通道突破普通文本补全的长度截断限制。
   - 网关接收后在流式/非流式层即时解构，还原为干净的标准 `assistant.content`，对下游客户端完全透明。
2. **模型 ID 零映射原生透传**：
   - 客户端传入的任何模型 ID（如 `gemini-3.5-flash-low`、`gemini-3.7-flash-high`、`claude-sonnet-4-6` 等）直接逐字转发给上游，不建立别名，不污染 ID。
3. **真实工具定义（Genuine Tools）完全保真**：
   - 客户端自身携带的 Tools（如 Web Search、代码执行等）会被原样保留，模型生成的真实工具调用不被吞噬、不被篡改。
4. **空 Assistant 内容强约束与绝对防拼接**：
   - 严格杜绝合成协议回答与普通正文发生拼接，保证单一事实来源，消除混淆输出。
5. **极速增量流式 (SSE) 状态机**：
   - 逐字节增量解析 UTF-8 与 JSON 转义序列，已验证的合法字符即刻 Flush 下发，首字几乎零附加延迟。
6. **确定性有界格式修复与空回自动重试**：
   - 针对可能产生的 Markdown 围栏、未闭合引号、尾逗号提供确定性本地修复。
   - 上游发生空回（完全没有任何输出）时，自动触发重试（默认 3 次）。
7. **智能模型过滤分类**：
   - 自动识别文本/对话模型并启用防截断保护；对于图像生成、语音、嵌入（Embedding）等非文本模型，自动执行原生纯透明透传。
8. **对话轮次自适应修复 (Gemini 兼容)**：
   - 智能识别上下文末尾。若末尾仅包含 `assistant` 预填或 `system` 规则，网关自动在末尾自适应收口为 `user` 轮次，彻底解决 Gemini/CPA 上游 400 报错。
9. **高并发无锁热路径**：
   - 基于不可变内存快照与原子指针，鉴权与路由全程无锁，轻松支撑数千 QPS 高并发。

---

### 📥 获取与运行方式（按你的环境选择）

本程序是使用 Go 编写的**纯静态单文件独立可执行程序**（所有依赖均已打包在单个二进制文件中）。

> 💡 **如果没有安装 Go 语言环境？**
> 你**完全不需要安装 Go**！可直接按 **【方案 A：使用预编译单文件】** 或 **【方案 B：Docker 容器运行】** 极速上手。

---

#### 方案 A：直接使用预编译可执行文件（免安装 Go，推荐小白/快速部署）
1. 在 [Releases 发布页面](https://github.com/Xeltra233/Antigravity-anti-truncation-gateway/releases) 下载对应系统的打包压缩包：
   - **Windows**: 下载 `gateway-windows-amd64.zip`（解压得到 `gateway.exe` 与 `run.bat`）
   - **Linux**: 下载 `gateway-linux-amd64.tar.gz`
   - **macOS**: 下载 `gateway-darwin-arm64.tar.gz` 或 `gateway-darwin-amd64.tar.gz`
2. 解压到任意目录，配置环境变量即可直接运行（Windows 双击 `run.bat`，Linux 执行 `./gateway`），无需安装任何运行时或依赖库。

---

#### 方案 B：使用 Docker 容器（免安装 Go）
只要你的机器安装了 Docker，无需安装 Go，直接利用内置 Dockerfile 构建或运行：
```bash
# 1. 克隆代码库
git clone https://github.com/Xeltra233/Antigravity-anti-truncation-gateway.git
cd Antigravity-anti-truncation-gateway

# 2. 构建并启动容器
docker build -t antigravity-gateway:latest .
docker run -d \
  --name antigravity-gateway \
  -p 8080:8080 \
  -e UPSTREAM_BASE_URL="https://your-upstream-url.com" \
  -e UPSTREAM_API_KEY="sk-your-upstream-key" \
  -e API_KEY="sk-antigravity-123456" \
  --restart unless-stopped \
  antigravity-gateway:latest
```

---

#### 方案 C：从源码自行编译（需安装 Go 1.24+）
如果你希望自己修改代码并编译：

##### 1. 安装 Go 语言环境（如尚未安装）
- **Windows**: 打开 PowerShell 执行 `winget install GoLang.Go`，或从 [Go 官网下载安装包](https://go.dev/dl/)。
- **Ubuntu / Debian**: 
  ```bash
  sudo apt update && sudo apt install -y golang
  ```
- **macOS**: 
  ```bash
  brew install go
  ```

##### 2. 获取源码
```bash
git clone https://github.com/Xeltra233/Antigravity-anti-truncation-gateway.git
cd Antigravity-anti-truncation-gateway
```

##### 3. 执行编译命令
- **Linux / macOS 本地编译**:
  ```bash
  go build -trimpath -ldflags="-s -w" -o gateway ./cmd/gateway
  chmod +x gateway
  ```

- **Windows 本地编译 (PowerShell / CMD)**:
  ```bash
  go build -trimpath -ldflags="-s -w" -o gateway.exe ./cmd/gateway
  ```

- **跨平台交叉编译 (在一台机器上为其他系统编译)**:
  ```bash
  # 编译 Linux 64位服务器运行的二进制
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o gateway-linux-amd64 ./cmd/gateway

  # 编译 Windows 运行的 EXE 文件
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o gateway.exe ./cmd/gateway
  ```
---

### ⚙️ 配置详解与环境变量

网关采用**环境变量优先（12-Factor App）**的设计，可以直接通过系统环境变量、启动脚本或 `.env` 文件进行配置。

#### 环境变量完整参考表

| 环境变量 | 必需 | 默认值 | 详细说明 |
| :--- | :---: | :--- | :--- |
| **`UPSTREAM_BASE_URL`** | **是** | - | 上游 API 根链接（如 `https://api.openai.com` 或你的代理中转地址，无需写 `/v1`，网关会自动补齐） |
| **`UPSTREAM_API_KEY`** | 视模式 | - | 上游 Bearer API 密钥（当 `UPSTREAM_AUTH_MODE=bearer` 时必需） |
| `UPSTREAM_AUTH_MODE` | 否 | `bearer` | 上游鉴权模式：`bearer` 或 `none`（上游无需密码时） |
| `UPSTREAM_TIMEOUT_MS` | 否 | `120000` | 上游请求超时时间（毫秒，默认 2 分钟） |
| **`API_KEY`** | 否 | - | 下游客户端连接网关时使用的 API Key（单 Key 极简模式） |
| `PORT` | 否 | `8080` | 网关监听端口 |
| `HOST` | 否 | `0.0.0.0` | 网关监听地址 |
| `WRAPPER_MODE` | 否 | `prefer` | 包装模式：`prefer`（推荐，注入合成协议）、`required`（强制）、`off`（紧急回滚透传） |
| `RECOVERY_POLICY` | 否 | `repair` | 格式恢复策略：`repair`（本地确定性修复）、`repair_then_retry`（修复失败后安全单次重试）、`fail`（直接报错） |
| `UPSTREAM_EMPTY_RETRIES` | 否 | `3` | 上游空回（完全没有任何输出）时的自动重试次数 |
| `CONTROL_MESSAGE_ROLE` | 否 | `system` | 控制提示词的角色：`system` 或 `developer` |
| `CONTROL_MESSAGE_POSITION`| 否 | `tail` | 控制提示词注入位置：`tail`（末尾强生效）、`head`、`system_tail` |
| `SYNTHETIC_TOOL_PREFIX` | 否 | `agw_emit_` | 动态合成工具的前缀（用于生成随机工具名） |
| `MAX_REQUEST_BYTES` | 否 | `16777216` | 最大允许请求大小（默认 16MB） |
| `MAX_RESPONSE_BYTES` | 否 | `16777216` | 最大允许响应大小（默认 16MB） |
| `MAX_CONCURRENT_REQUESTS` | 否 | `1024` | 全局最大并发请求数 |
| `MAX_CONCURRENT_REQUESTS_PER_KEY` | 否 | `64` | 单个 API Key 最大并发请求数 |
| `ADMIN_API_KEY` | 否 | `admin-secret-key-12345` | 多 Key 动态管理接口鉴权 Key（用于 `/admin/keys`） |
| `LOG_LEVEL` | 否 | `info` | 日志级别：`debug`、`info`、`warn`、`error` |

#### `.env` 配置文件示例 (可参考项目中的 `.env.example`)
```env
# 必需：上游根地址与密钥
UPSTREAM_BASE_URL=https://api.openai.com
UPSTREAM_API_KEY=sk-your-upstream-key-here

# 必需：给下游客户端配置的 Key 与端口
API_KEY=sk-antigravity-123456
PORT=8080
HOST=0.0.0.0

# 防截断策略
WRAPPER_MODE=prefer
RECOVERY_POLICY=repair
UPSTREAM_EMPTY_RETRIES=3
```

---

### 🚀 部署与运行方式

#### 方式 1：Linux / macOS 命令行直接启动
```bash
export UPSTREAM_BASE_URL="https://your-upstream-domain.com"
export UPSTREAM_API_KEY="sk-your-upstream-key"
export API_KEY="sk-antigravity-123456"
export PORT="8080"

./gateway
```

#### 方式 2：Windows 一键批处理 (`run.bat`)
在项目目录下直接双击或运行 `run.bat`：
```bat
@echo off
set UPSTREAM_BASE_URL=https://your-upstream-domain.com
set UPSTREAM_API_KEY=sk-your-upstream-key
set API_KEY=sk-antigravity-123456
set PORT=8080

"%~dp0gateway.exe"
pause
```

#### 方式 3：Linux Systemd 守护进程部署
创建 systemd 服务文件 `/etc/systemd/system/antigravity-gateway.service`：
```ini
[Unit]
Description=Antigravity Anti-Truncation Gateway Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/antigravity-gateway
ExecStart=/opt/antigravity-gateway/gateway
Restart=always
RestartSec=5
Environment=UPSTREAM_BASE_URL=https://your-upstream-domain.com
Environment=UPSTREAM_API_KEY=sk-your-upstream-key
Environment=API_KEY=sk-antigravity-123456
Environment=PORT=8080
Environment=HOST=0.0.0.0

[Install]
WantedBy=multi-user.target
```
启动并开机自启：
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now antigravity-gateway
sudo systemctl status antigravity-gateway
```

#### 方式 4：Docker / Docker Compose 部署
使用 `docker run` 快速启动：
```bash
docker run -d \
  --name antigravity-gateway \
  -p 8080:8080 \
  -e UPSTREAM_BASE_URL="https://your-upstream-domain.com" \
  -e UPSTREAM_API_KEY="sk-your-upstream-key" \
  -e API_KEY="sk-antigravity-123456" \
  --restart unless-stopped \
  antigravity-gateway:latest
```

或使用 `docker-compose.yml`：
```yaml
version: '3.8'

services:
  gateway:
    build: .
    container_name: antigravity-gateway
    ports:
      - "8080:8080"
    environment:
      - UPSTREAM_BASE_URL=https://your-upstream-domain.com
      - UPSTREAM_API_KEY=sk-your-upstream-key
      - API_KEY=sk-antigravity-123456
      - PORT=8080
    restart: unless-stopped
```

---

### 📱 客户端接入实战

网关完全兼容标准 OpenAI `/v1` 协议端点。

#### 1. 通用连接参数
- **API 接口地址 (Base URL)**: `http://127.0.0.1:8080/v1`（局域网/公网部署请替换为对应 IP 或域名）
- **API 密钥 (API Key)**: 填入网关中配置的 `API_KEY`（如 `sk-antigravity-123456`）
- **模型名称**: 填入上游支持的任何原生模型（如 `gemini-3.5-flash-low`, `gemini-3.7-flash-high`, `claude-sonnet-4-6` 等）

#### 2. SillyTavern (酒馆) 设置
1. 打开酒馆 API 设置面板，选择 **API**: `Chat Completion (OpenAI Compatible)`；
2. **Custom Endpoint (自定义端点)**: `http://127.0.0.1:8080/v1`；
3. **API Key**: 输入你的 `API_KEY`；
4. 点击 **Connect (连接)**，在下拉列表中选择你的目标模型即可畅快游玩，彻底告别回答截断与轮次 400 报错。

#### 3. Cherry Studio / NextChat / Chatbox 设置
- **提供商类型**: OpenAI / OpenAI 兼容
- **API 地址**: `http://127.0.0.1:8080/v1`
- **API Key**: `sk-antigravity-123456`

#### 4. cURL 快速测试验证
- **测试模型列表获取**:
  ```bash
  curl -s -H "Authorization: Bearer sk-antigravity-123456" http://127.0.0.1:8080/v1/models
  ```
- **测试对话流式输出 (SSE)**:
  ```bash
  curl http://127.0.0.1:8080/v1/chat/completions \
    -H "Authorization: Bearer sk-antigravity-123456" \
    -H "Content-Type: application/json" \
    -d '{
      "model": "gemini-3.5-flash-low",
      "messages": [{"role": "user", "content": "你好，请写一段500字的故事。"}],
      "stream": true
    }'
  ```

---

### 🛡️ 运维监控与健康检查

- **存活探针 (Liveness)**: `GET /healthz` → 返回 `{"status":"ok"}` (200 OK)
- **就绪探针 (Readiness)**: `GET /readyz` → 返回 `{"status":"ready"}` (200 OK)
- **Prometheus 监控指标**: `GET /metrics` → 输出标准 Prometheus Metrics，包含请求总量、活跃连接数、响应延迟统计等。
- **紧急回滚**: 若遇到突发未知上游格式异常，可将环境变量设置为 `WRAPPER_MODE=off` 无缝切换为原生纯透传模式。

---

### ❓ 常见问题排查 (FAQ)

#### Q1: 为什么会出现 400 "Requests ending with a model turn are not supported"?
**A**: 这是 Google Gemini / CPA 上游的特定限制，原生接口要求对话内容必须以 `user` 角色结尾。网关已内置智能轮次闭环机制，会自动将尾部悬空的 Assistant 预填或系统提示词自适应修正为 User 轮次，避免触发上游报错。

#### Q2: 遇到模型拒答（如“抱歉，我无法生成该内容”）是网关问题吗？
**A**: 不是。模型拒答分为两种：
1. **外审拦截**（返回 `finish_reason: "SAFETY"` 或 HTTP 400）；
2. **内审拒答**（模型本身回复道歉文本，返回 `role: "assistant"`，`status: 200`）。
如果是内审拒答，说明提示词命中了模型的关键词红线，建议调整人物设定或选用对齐规则更宽松/带有 Thinking 链的模型。

#### Q3: 网关支持非文本模型（如生图/声音）吗？
**A**: 完全支持。网关内置了智能正则过滤器，检测到生图、图像分析、音频等模型时会自动跳过合成包装，执行 100% 原生透传。

---

<a name="english"></a>
## 🇬🇧 English Documentation

### 📖 Overview & Problems Solved

**Antigravity Anti-Truncation Gateway** is a high-performance, headless, environment-variable-configured OpenAI Chat Completions compatibility middleware.

Under heavy prompt workloads (e.g., **SillyTavern complex presets, massive character cards, prompt pre-fills, and multi-turn tool calling**), mainstream large language models (Claude, Gemini, GPT, etc.) frequently encounter:
1. **Abrupt Output Truncation**: Standard completion channels cut off long-form responses prematurely due to hard output token limits or filters.
2. **Upstream Turn Validation Errors (HTTP 400)**: Using Gemini / CPA upstream with pre-filled Assistant messages or non-standard concluding turns triggers `Requests ending with a model turn are not supported.`.
3. **Format Breakdown & Dangling Syntax**: Unclosed Markdown code fences, missing JSON quotes, or dangling commas crashing downstream client parsers.
4. **Tool Call Collision**: Synthetic control prompts corrupting genuine client-defined tools (e.g., web search, code execution).

By leveraging a **Dynamic Synthetic Transport Tool Protocol**, an **Incremental UTF-8 / JSON Stream State Machine**, and **Adaptive Conversation Turn Sanitization**, this gateway transparently eliminates these issues without altering client workflows or upstream model IDs.

---

### 🌟 Key Architecture & Features

1. **Synthetic Transport Protocol**:
   - Provisions a high-entropy (96-bit random nonce) transport tool (e.g., `agw_emit_7f3a91c8d42e6b1a9c02a547`) per request.
   - Instructs the model to output visible responses within tool arguments, leveraging the structured function call channel to bypass output text truncation limits.
   - Deconstructed in real-time and reconstructed into standard `assistant.content` transparently for downstream clients.
2. **100% Native Model ID Passthrough**:
   - Verbatim model forwarding (e.g., `gemini-3.5-flash-low`, `gemini-3.7-flash-high`, `claude-sonnet-4-6`) without alias mappings or ID pollution.
3. **Genuine Downstream Tool Fidelity**:
   - Client tools (Web Search, custom extensions) are fully preserved. Genuine model-generated tool calls pass through intact.
4. **Single Source of Truth (No Concatenation)**:
   - Enforces strict isolation to prevent synthetic tool outputs from colliding or concatenating with plain assistant content.
5. **Instant-Emission SSE Streaming (Low Latency)**:
   - Byte-level incremental UTF-8 and JSON escape state machine flushes validated chunks immediately, adding near-zero latency.
6. **Deterministic Bounded Repair & Empty Retries**:
   - Local deterministic formatting repair for unclosed markdown fences and quotes.
   - Automatic upstream retries (default: 3 attempts) when upstream returns an empty response.
7. **Intelligent Model Classification**:
   - Automatically activates protection for conversational/text models while transparently passing through non-text models (image generation, audio, embeddings, moderation).
8. **Adaptive Turn Sanitization (Gemini Compatibility)**:
   - Scans conversational history to ensure turn structures conclude on a valid `user` turn, resolving Gemini/CPA HTTP 400 errors.
9. **Lock-Free Hot Path**:
   - Atomic pointer swaps and immutable memory snapshots for authentication and routing, effortlessly supporting thousands of QPS.

---

### 📥 Installation & Getting Started

Antigravity Gateway is distributed as a **statically linked, standalone single binary** with zero external runtime dependencies.

> 💡 **No Go Environment Installed?**
> You **do not need Go installed**! Choose **[Option A: Prebuilt Binary]** or **[Option B: Docker Container]** to get started immediately.

---

#### Option A: Prebuilt Standalone Binary (Recommended / No Go Needed)
1. Download the archive for your operating system from [Releases](https://github.com/Xeltra233/Antigravity-anti-truncation-gateway/releases):
   - **Windows**: `antigravity-gateway_windows_amd64.zip` (includes `gateway.exe` & `run.bat`)
   - **Linux**: `antigravity-gateway_linux_amd64.tar.gz`
   - **macOS**: `antigravity-gateway_darwin_arm64.tar.gz` or `antigravity-gateway_darwin_amd64.tar.gz`
2. Extract the archive, configure your environment variables (or edit `.env`), and run (`run.bat` on Windows, `./gateway` on Linux/macOS).

---

#### Option B: Docker Container (No Go Needed)
```bash
# 1. Clone repository
git clone https://github.com/Xeltra233/Antigravity-anti-truncation-gateway.git
cd Antigravity-anti-truncation-gateway

# 2. Build and run container
docker build -t antigravity-gateway:latest .
docker run -d \
  --name antigravity-gateway \
  -p 8080:8080 \
  -e UPSTREAM_BASE_URL="https://your-upstream-url.com" \
  -e UPSTREAM_API_KEY="sk-your-upstream-key" \
  -e API_KEY="sk-antigravity-123456" \
  --restart unless-stopped \
  antigravity-gateway:latest
```

---

#### Option C: Build from Source (Requires Go 1.24+)

##### 1. Install Go (if not installed)
- **Windows**: In PowerShell run `winget install GoLang.Go` or download from [Go Downloads](https://go.dev/dl/).
- **Ubuntu / Debian**: `sudo apt update && sudo apt install -y golang`
- **macOS**: `brew install go`

##### 2. Clone & Compile
```bash
git clone https://github.com/Xeltra233/Antigravity-anti-truncation-gateway.git
cd Antigravity-anti-truncation-gateway

# Native compilation
go build -trimpath -ldflags="-s -w" -o gateway ./cmd/gateway
```

##### 3. Cross-Compilation
```bash
# Target Linux AMD64 from Windows/macOS
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o gateway-linux-amd64 ./cmd/gateway

# Target Windows from Linux/macOS
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o gateway.exe ./cmd/gateway
```

---

### ⚙️ Configuration & Environment Variables

The gateway follows 12-Factor App design principles and reads configuration directly from environment variables or `.env` files.

#### Complete Environment Variables Reference

| Variable | Required | Default | Description |
| :--- | :---: | :--- | :--- |
| **`UPSTREAM_BASE_URL`** | **Yes** | - | Upstream API base URL (e.g., `https://api.openai.com` or custom reverse proxy, `/v1` appended automatically) |
| **`UPSTREAM_API_KEY`** | Conditional | - | Upstream Bearer API key (required when `UPSTREAM_AUTH_MODE=bearer`) |
| `UPSTREAM_AUTH_MODE` | No | `bearer` | Upstream authentication mode: `bearer` or `none` |
| `UPSTREAM_TIMEOUT_MS` | No | `120000` | Upstream request timeout in milliseconds (default: 2 minutes) |
| **`API_KEY`** | No | - | Downstream client authentication key (single-key simplified mode) |
| `PORT` | No | `8080` | Gateway listening port |
| `HOST` | No | `0.0.0.0` | Gateway listening host interface |
| `WRAPPER_MODE` | No | `prefer` | Wrapper mode: `prefer` (recommended, injects synthetic protocol), `required` (strict), `off` (emergency passthrough) |
| `RECOVERY_POLICY` | No | `repair` | Recovery policy: `repair` (local deterministic fix), `repair_then_retry` (retry on failure), `fail` (error out) |
| `UPSTREAM_EMPTY_RETRIES` | No | `3` | Number of automatic retries upon receiving empty responses from upstream |
| `CONTROL_MESSAGE_ROLE` | No | `system` | Injected control message role: `system` or `developer` |
| `CONTROL_MESSAGE_POSITION`| No | `tail` | Injected message position: `tail` (recommended for strong adherence), `head`, `system_tail` |
| `SYNTHETIC_TOOL_PREFIX` | No | `agw_emit_` | Prefix for generated synthetic tool names |
| `MAX_REQUEST_BYTES` | No | `16777216` | Maximum allowed request size in bytes (default: 16MB) |
| `MAX_RESPONSE_BYTES` | No | `16777216` | Maximum allowed response size in bytes (default: 16MB) |
| `MAX_CONCURRENT_REQUESTS` | No | `1024` | Global maximum concurrent requests |
| `MAX_CONCURRENT_REQUESTS_PER_KEY` | No | `64` | Maximum concurrent requests per API key |
| `ADMIN_API_KEY` | No | `admin-secret-key-12345` | Authentication key for dynamic key management endpoint (`/admin/keys`) |
| `LOG_LEVEL` | No | `info` | Logging verbosity level: `debug`, `info`, `warn`, `error` |

#### `.env` File Example
```env
UPSTREAM_BASE_URL=https://api.openai.com
UPSTREAM_API_KEY=sk-your-upstream-key-here

API_KEY=sk-antigravity-123456
PORT=8080
HOST=0.0.0.0

WRAPPER_MODE=prefer
RECOVERY_POLICY=repair
UPSTREAM_EMPTY_RETRIES=3
```

---

### 🚀 Deployment Methods

#### Method 1: Linux / macOS CLI
```bash
export UPSTREAM_BASE_URL="https://your-upstream-domain.com"
export UPSTREAM_API_KEY="sk-your-upstream-key"
export API_KEY="sk-antigravity-123456"
export PORT=8080

./gateway
```

#### Method 2: Windows Batch Script (`run.bat`)
Double-click `run.bat` or run:
```bat
@echo off
set UPSTREAM_BASE_URL=https://your-upstream-domain.com
set UPSTREAM_API_KEY=sk-your-upstream-key
set API_KEY=sk-antigravity-123456
set PORT=8080

"%~dp0gateway.exe"
pause
```

#### Method 3: Linux Systemd Service
Create `/etc/systemd/system/antigravity-gateway.service`:
```ini
[Unit]
Description=Antigravity Anti-Truncation Gateway Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/antigravity-gateway
ExecStart=/opt/antigravity-gateway/gateway
Restart=always
RestartSec=5
Environment=UPSTREAM_BASE_URL=https://your-upstream-domain.com
Environment=UPSTREAM_API_KEY=sk-your-upstream-key
Environment=API_KEY=sk-antigravity-123456
Environment=PORT=8080
Environment=HOST=0.0.0.0

[Install]
WantedBy=multi-user.target
```
Enable and start the service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now antigravity-gateway
```

#### Method 4: Docker Compose
```yaml
version: '3.8'

services:
  gateway:
    build: .
    container_name: antigravity-gateway
    ports:
      - "8080:8080"
    environment:
      - UPSTREAM_BASE_URL=https://your-upstream-domain.com
      - UPSTREAM_API_KEY=sk-your-upstream-key
      - API_KEY=sk-antigravity-123456
      - PORT=8080
    restart: unless-stopped
```

---

### 📱 Client Integration

The gateway is fully compatible with standard OpenAI `/v1` endpoint specifications.

#### 1. General Connection Settings
- **Base URL**: `http://127.0.0.1:8080/v1` (replace with server IP / domain for remote deployment)
- **API Key**: The configured `API_KEY` (e.g., `sk-antigravity-123456`)
- **Model Name**: Any model supported upstream (e.g., `gemini-3.5-flash-low`, `claude-sonnet-4-6`, `gpt-4o`)

#### 2. SillyTavern Configuration
1. Open API settings in SillyTavern, set **API** to `Chat Completion (OpenAI Compatible)`.
2. **Custom Endpoint**: `http://127.0.0.1:8080/v1`.
3. **API Key**: Enter your `API_KEY`.
4. Click **Connect** and select your target model.

#### 3. Cherry Studio / NextChat / Chatbox
- **Provider**: OpenAI / OpenAI Compatible
- **API Host**: `http://127.0.0.1:8080/v1`
- **API Key**: `sk-antigravity-123456`

#### 4. cURL Verification
- **Fetch Models**:
  ```bash
  curl -s -H "Authorization: Bearer sk-antigravity-123456" http://127.0.0.1:8080/v1/models
  ```
- **Test Streaming Chat Completion**:
  ```bash
  curl http://127.0.0.1:8080/v1/chat/completions \
    -H "Authorization: Bearer sk-antigravity-123456" \
    -H "Content-Type: application/json" \
    -d '{
      "model": "gemini-3.5-flash-low",
      "messages": [{"role": "user", "content": "Write a 500-word short story."}],
      "stream": true
    }'
  ```

---

### 🛡️ Health Checks & Observability

- **Liveness Probe**: `GET /healthz` → returns `{"status":"ok","version":"..."}` (200 OK)
- **Readiness Probe**: `GET /readyz` → returns `{"status":"ready","version":"..."}` (200 OK)
- **Prometheus Metrics**: `GET /metrics` → exports standard metrics including request totals, latencies, active connections, and synthetic wrapper counts.
- **Emergency Fallback**: Set `WRAPPER_MODE=off` to revert instantly to transparent raw passthrough without restarting.

---

### ❓ Frequently Asked Questions (FAQ)

#### Q1: Why did I receive 400 "Requests ending with a model turn are not supported"?
**A**: This is a strict constraint enforced by Google Gemini / CPA upstream requiring conversations to end on a `user` turn. The gateway automatically detects dangling Assistant pre-fills and system messages, sanitizing trailing turns into compliant user turns to eliminate this error.

#### Q2: Is model refusal (e.g., "I cannot fulfill this request") caused by the gateway?
**A**: No. Refusal occurs either as:
1. **External Filter**: Guardrails returning `finish_reason: "SAFETY"` or HTTP 400.
2. **Internal Refusal**: The model itself generating an apology text (`role: "assistant"`, status 200).
Internal refusal indicates sensitive keywords in the prompt card. Adjust your character card or switch to models with reasoning/thinking enabled.

#### Q3: Does the gateway support non-text models (e.g., image generation, audio)?
**A**: Yes. The gateway includes an automatic classifier that skips synthetic wrapping for non-text models (DALL-E, Flux, Whisper, Embeddings) and performs 100% raw passthrough.

---
---

<a name="license"></a>
## 📄 License

This project is open-sourced under the [MIT License](LICENSE).
Copyright (c) 2026 Xeltra233.

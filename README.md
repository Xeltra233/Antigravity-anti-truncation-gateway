# Antigravity Anti-Truncation Gateway (OpenAI Chat 防截断兼容网关)

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/Architecture-Headless%20%7C%20High--Concurrency-blue.svg?style=flat-square" alt="Architecture">
  <img src="https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS%20%7C%20Android%20%7C%20Docker-orange.svg?style=flat-square" alt="Platform">
</p>

---

## 📑 目录 (Table of Contents)

- [🇨🇳 中文说明](#-中文说明)
  - [项目简介与解决痛点](#-项目简介与解决痛点)
  - [🌟 核心工作原理与特性](#-核心工作原理与特性)
  - [📥 获取与运行方式（免安装/Android/Docker/源码）](#-获取与运行方式按你的环境选择)
  - [⚙️ 配置详解与环境变量 (完整参考)](#️-配置详解与环境变量)
  - [🚀 部署与运行方式 (Windows/Linux/Docker/Systemd)](#-部署与运行方式)
  - [🔑 多用户动态 Key 管理 API (/admin/keys)](#-多用户动态-key-管理-api)
  - [📱 客户端接入实战 (SillyTavern / Cherry Studio 等)](#-客户端接入实战)
  - [🛡️ 运维监控与健康检查](#️-运维监控与健康检查)
  - [❓ 常见问题排查 (FAQ)](#-常见问题排查-faq)
- [🇬🇧 English Documentation](#-english-documentation)
  - [Overview & Problems Solved](#overview--problems-solved)
  - [🌟 Key Architecture & Features](#-key-architecture--features)
  - [📥 Installation & Getting Started (Prebuilt / Android / Docker / Source)](#-installation--getting-started)
  - [⚙️ Configuration & Environment Variables (Full Reference)](#️-configuration--environment-variables)
  - [🚀 Deployment Methods (Windows / Linux / Docker / Systemd)](#-deployment-methods)
  - [🔑 Dynamic Multi-Key Management API (/admin/keys)](#-dynamic-multi-key-management-api)
  - [📱 Client Integration (SillyTavern, Cherry Studio, etc.)](#-client-integration)
  - [🛡️ Health Checks & Observability](#️-health-checks--observability)
  - [❓ Frequently Asked Questions (FAQ)](#-frequently-asked-questions-faq)
- [📄 License](#-license)

---

<a name="中文说明"></a>
## 🇨🇳 中文说明

### 📖 项目简介与解决痛点

**Antigravity Anti-Truncation Gateway** 是一款专为 **OpenAI Chat Completions 协议** 设计的高性能、无 UI（Headless）、环境变量与 `.env` 配置文件优先的防截断与格式修复中间件网关。

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
9. **高并发无锁热路径与动态 Key 数据库**：
   - 基于不可变内存快照与原子指针，鉴权与路由全程无锁，轻松支撑数千 QPS 高并发；内置 SQLite 支撑动态多 Key 管理。
10. **原生 `.env` 自动加载与 Windows 友好**：
    - 程序启动自动加载当前目录或程序所在目录下的 `.env` 文件，自动剥离 Windows 记事本 UTF-8 BOM，开箱即用。

---

### 📥 获取与运行方式（按你的环境选择）

本程序是使用 Go 编写的**纯静态单文件独立可执行程序**（所有依赖均已打包在单个二进制文件中）。

> 💡 **如果没有安装 Go 语言环境？**
> 你**完全不需要安装 Go**！可直接按 **【方案 A：使用预编译单文件/安卓客户端】** 或 **【方案 B：Docker 容器运行】** 极速上手。

---

#### 方案 A：直接使用预编译可执行文件或安卓 APK（免安装 Go，推荐小白/快速部署）
1. 在 [GitHub Releases 发布页面](https://github.com/Xeltra233/Antigravity-gateway/releases) 下载对应系统的打包文件：
   - **Windows (x64)**: 下载 `antigravity-gateway_windows_amd64.zip`（解压得到 `gateway.exe`、`run.bat` 与 `.env.example`）
   - **Windows (ARM64)**: 下载 `antigravity-gateway_windows_arm64.zip`
   - **Linux (x64)**: 下载 `antigravity-gateway_linux_amd64.tar.gz`
   - **Linux (ARM64)**: 下载 `antigravity-gateway_linux_arm64.tar.gz`
   - **macOS (Apple Silicon M系列)**: 下载 `antigravity-gateway_darwin_arm64.tar.gz`
   - **macOS (Intel)**: 下载 `antigravity-gateway_darwin_amd64.tar.gz`
   - **Android 手机/平板**: 下载 `antigravity-gateway-v*.apk` 直接安装，随时随地在手机本地运行网关！
2. 解压到任意目录，复制 `.env.example` 为 `.env` 并填入您的上游地址即可直接运行（Windows 双击 `run.bat`，Linux/macOS 执行 `./gateway`）。

---

#### 方案 B：使用 Docker 容器（免安装 Go）
只要你的机器安装了 Docker，无需安装 Go，直接利用内置 Dockerfile 构建或运行：
```bash
# 1. 克隆代码库
git clone https://github.com/Xeltra233/Antigravity-gateway.git
cd Antigravity-gateway

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

#### 方案 C：从源码自行编译（需安装 Go 1.25+）
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
git clone https://github.com/Xeltra233/Antigravity-gateway.git
cd Antigravity-gateway
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

- **查看版本信息**:
  ```bash
  ./gateway -v
  # 输出: Antigravity Gateway version 1.0.7 (commit: ..., built: ...)
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

网关采用**环境变量与 `.env` 配置文件优先（12-Factor App）**的设计，遵循以下优先级：
1. **系统/终端已存在的环境变量**（最高优先级，适合 CI/CD 与 Docker `-e` 注入）；
2. **`ENV_FILE` 变量指定路径**的配置文件；
3. **当前工作目录下的 `.env`**；
4. **可执行文件同级目录下的 `.env`**（特别保证 Windows 双击运行与 Windows 服务自启）；
5. **父级目录下的 `../.env`**。

> 💡 **自动剥离 UTF-8 BOM**：支持在 Windows 记事本中直接新建并保存 `.env`，网关会自动处理 BOM 头，绝不报错。

#### 环境变量完整参考表

| 环境变量 | 必需 | 默认值 | 详细说明 |
| :--- | :---: | :--- | :--- |
| **`UPSTREAM_BASE_URL`** | **是** | - | 上游 API 根链接（如 `https://api.openai.com` 或代理中转地址，末尾 `/v1` 会自动补齐/规范化） |
| **`UPSTREAM_API_KEY`** | 视模式 | - | 上游 Bearer API 密钥（当 `UPSTREAM_AUTH_MODE=bearer` 时必需） |
| `UPSTREAM_AUTH_MODE` | 否 | `bearer` | 上游鉴权模式：`bearer` 或 `none`（上游无需鉴权或本地代理时） |
| `UPSTREAM_TIMEOUT_MS` | 否 | `120000` | 上游请求单次超时时间（毫秒，默认 2 分钟） |
| `PORT` | 否 | `8080` | 网关本地监听端口 |
| `HOST` | 否 | `0.0.0.0` | 网关监听网络地址 |
| `API_KEY` | 否 | - | 下游客户端简易访问 Key；也可通过 `DOWNSTREAM_KEYS_JSON` 配置多个 Key。`/v1/*` 始终要求有效 Bearer Key，未配置任何下游 Key 时不会自动变为免认证模式。 |
| `ENV_FILE` | 否 | - | 自定义指定加载的 `.env` 配置文件路径 |
| `WRAPPER_MODE` | 否 | `prefer` | 包装模式：`prefer`（推荐，自适应注入合成协议）、`required`（强制要求）、`off`（紧急透明透传回滚） |
| `RECOVERY_POLICY` | 否 | `repair` | 格式恢复策略：`repair`（本地确定性修复）、`repair_then_retry`（单次安全重试）、`fail`（直接报错） |
| `UPSTREAM_EMPTY_RETRIES` | 否 | `3` | 上游空回（完全没有任何输出）时的自动重试次数 |
| `CONTROL_MESSAGE_ROLE` | 否 | `system` | 控制提示词角色：`system` 或 `developer` |
| `CONTROL_MESSAGE_POSITION`| 否 | `tail` | 控制提示词注入位置：`tail`（末尾强生效）、`head`、`system_tail` |
| `SYNTHETIC_TOOL_PREFIX` | 否 | `agw_emit_` | 动态合成工具的前缀（用于生成 96-bit 随机工具名） |
| `SYNTHETIC_TOOL_STRICT` | 否 | `false` | 是否向模型发送严格（Strict）Tool Schema 约束 |
| `TEXT_MODEL_PATTERN` | 否 | - | 可选正则：自定义判定为文本对话模型并启用防截断保护的匹配规则 |
| `NON_TEXT_MODEL_PATTERN` | 否 | - | 可选正则：自定义判定为生图/语音/嵌入模型并跳过合成包装直接透传的匹配规则 |
| `MAX_REQUEST_BYTES` | 否 | `16777216` | 最大允许请求大小（字节，默认 16MB） |
| `MAX_RESPONSE_BYTES` | 否 | `16777216` | 最大允许响应大小（字节，默认 16MB） |
| `MAX_CONCURRENT_REQUESTS` | 否 | `1024` | 全局最大并发请求数 |
| `MAX_CONCURRENT_REQUESTS_PER_KEY` | 否 | `64` | 单个下游 API Key 最大并发请求数 |
| `REQUEST_QUEUE_TIMEOUT_MS`| 否 | `50` | 高并发下请求排队最大等待超时（毫秒） |
| `STREAM_SIDE_BUFFER_BYTES`| 否 | `0` | 流式 Side Buffer 大小（默认 `0` 即极速直发，零额外延迟） |
| `STREAM_REPAIR_BUFFER_BYTES`| 否 | `1048576` | 流式格式修复缓冲区大小（默认 1MB） |
| `STREAM_FLUSH_INTERVAL_MS`| 否 | `0` | 流式输出刷新间隔（毫秒，默认 0 即刻刷新） |
| `ADMIN_API_KEY` | 否 | `admin-secret-key-12345` | 多 Key 动态管理接口鉴权 Key（用于 `/admin/keys`，自定义时至少 8 字符） |
| `KEY_HMAC_SECRET` | 否 | 内置默认密钥 | 下游 Key 的 HMAC-SHA256 安全签名密钥（自定义时至少 16 字符） |
| `KEY_DB_PATH` | 否 | `./data/keys.sqlite` | 动态 Key 持久化存储 SQLite 数据库路径 |
| `DOWNSTREAM_KEYS_JSON` | 否 | `[]` | 静态下游分发 Key 配置（JSON 数组，纯环境变量配置时使用） |
| `SHUTDOWN_TIMEOUT_MS` | 否 | `30000` | 优雅关机超时时长（毫秒，默认 30 秒） |
| `MODELS_CACHE_TTL_MS` | 否 | `30000` | `/v1/models` 上游模型列表缓存有效期（毫秒） |
| `LOG_LEVEL` | 否 | `info` | 日志级别：`debug`、`info`、`warn`、`error` |
| `TRUST_PROXY` | 否 | `false` | 是否信任反向代理传递的 `X-Forwarded-For` 真实客户端 IP |
| `UPSTREAM_MAX_IDLE_CONNS` | 否 | `2048` | 上游 HTTP 连接池最大空闲连接数 |
| `UPSTREAM_MAX_IDLE_CONNS_PER_HOST` | 否 | `512` | 每个上游 Host 最大空闲连接数 |
| `UPSTREAM_MAX_CONNS_PER_HOST` | 否 | `512` | 每个上游 Host 最大并发连接数 |

#### `.env` 配置文件示例 (直接复制项目中的 `.env.example` 修改)
```env
# 必需：上游根地址与密钥
UPSTREAM_BASE_URL=https://api.openai.com
UPSTREAM_API_KEY=sk-your-upstream-key-here

# 可选：给下游客户端配置的简易访问 Key 与端口
API_KEY=sk-antigravity-123456
PORT=8080
HOST=0.0.0.0

# 防截断策略配置
WRAPPER_MODE=prefer
RECOVERY_POLICY=repair
UPSTREAM_EMPTY_RETRIES=3

# 日志级别
LOG_LEVEL=info
```

---

### 🚀 部署与运行方式

#### 方式 1：Windows 一键批处理 (`run.bat`)
在发布包中已内置智能优化的 `run.bat`，直接双击运行即可：
- 自动检测同级目录的 `.env` 文件；
- 若无 `.env` 且无环境变量，会自动从 `.env.example` 复制生成 `.env` 并友好引导配置；
- 若运行异常退出，会保留窗口并提示具体排查步骤（如检查端口占用、Key 是否填写等）。

#### 方式 2：Linux / macOS 命令行直接启动
将 `.env.example` 复制为 `.env` 并填写好配置，直接运行：
```bash
cp .env.example .env
# 编辑配置
nano .env

# 启动运行
./gateway
```
或直接通过命令行环境变量覆盖启动：
```bash
export UPSTREAM_BASE_URL="https://your-upstream-domain.com"
export UPSTREAM_API_KEY="sk-your-upstream-key"
export API_KEY="sk-antigravity-123456"
export PORT="8080"

./gateway
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
# 既可使用 Environment 指定，也可直接在 WorkingDirectory 下放置 .env 文件
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

### 🔑 多用户动态 Key 管理 API

网关内置了多用户 API Key 管理引擎（支持 SQLite 存储与不可变内存快速鉴权），管理员可以通过 `/admin/keys` 接口进行自动化签发与吊销。

#### 1. 创建新下游 Key
```bash
curl -X POST http://127.0.0.1:8080/admin/keys \
  -H "Authorization: Bearer admin-secret-key-12345" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "User-Alice",
    "allowed_models": ["gemini-3.5-flash-low", "gpt-4o"]
  }'
```
响应示例：
```json
{
  "id": "key_7f8a91b2c3d4",
  "key": "sk-agw-live-a1b2c3d4e5f6...",
  "name": "User-Alice",
  "allowed_models": ["gemini-3.5-flash-low", "gpt-4o"],
  "status": "active",
  "created_at": "2026-09-03T01:00:00Z"
}
```

#### 2. 列出所有 Key
```bash
curl -s http://127.0.0.1:8080/admin/keys \
  -H "Authorization: Bearer admin-secret-key-12345"
```

#### 3. 吊销特定 Key
```bash
curl -X POST http://127.0.0.1:8080/admin/keys/key_7f8a91b2c3d4/revoke \
  -H "Authorization: Bearer admin-secret-key-12345"
```

---

### 📱 客户端接入实战

网关完全兼容标准 OpenAI `/v1` 协议端点。

#### 1. 通用连接参数
- **API 接口地址 (Base URL)**: `http://127.0.0.1:8080/v1`（局域网/公网部署请替换为对应 IP 或域名）
- **API 密钥 (API Key)**: 填入网关中配置的 `API_KEY` 或 `DOWNSTREAM_KEYS_JSON` 中的有效 Key；网关的 `/v1/*` 接口始终要求 Bearer Key，未配置任何下游 Key 时不会变成免认证模式。
- **模型名称**: 填入上游支持的原生模型（如 `gemini-3.5-flash-low`, `gemini-3.7-flash-high`, `claude-sonnet-4-6` 等）

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

- **存活探针 (Liveness)**: `GET /healthz` → 返回 `{"status":"ok","version":"1.0.7"}` (200 OK)
- **就绪探针 (Readiness)**: `GET /readyz` → 返回 `{"status":"ready","version":"1.0.7"}` (200 OK)
- **Prometheus 监控指标**: `GET /metrics` → 输出请求总量、活跃请求数、过载拒绝、合成包装命中/修复/重试/冲突等指标。
- **紧急回滚**: 若遇到突发未知上游格式异常，修改 `.env` 中的 `WRAPPER_MODE=off` 并重启网关，即可切换为原生纯透传模式。

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

#### Q4: 为什么我在 `.env` 文件里修改了配置却不生效？
**A**: 网关自 `v1.0.7` 起已原生支持 `.env` 文件解析。请确认：
1. 操作系统环境变量中是否已设置了同名变量（系统环境变量优先级高于 `.env` 文件）；
2. 启动时注意观察首行日志 `loaded configuration from .env file (path: ...)`，查看网关实际加载的 `.env` 文件绝对路径；
3. 如果使用 Windows，推荐直接使用包内自带的 `run.bat`，它会自动保障读取脚本同级目录下的 `.env`。

---

<a name="english"></a>
## 🇬🇧 English Documentation

### Overview & Problems Solved

**Antigravity Anti-Truncation Gateway** is a high-performance, headless, environment-variable & `.env`-configured OpenAI Chat Completions compatibility middleware.

In complex, heavy-prompt environments (such as **SillyTavern complex presets, massive character cards, prompt pre-fills, and multi-turn workflows**), mainstream LLMs (Claude, Gemini, GPT) frequently encounter:
1. **Premature Output Truncation**: Standard chat completion channels truncate unexpectedly due to token limits or output length restrictions;
2. **Upstream Protocol Errors (HTTP 400)**: Upstreams like Google Gemini or CPA reject requests with trailing Assistant pre-fills with `Requests ending with a model turn are not supported.`;
3. **Format Collapses**: Broken JSON, unclosed quotes, or incomplete markdown fences breaking downstream UI parsers;
4. **Tool Collision**: System control prompts clobbering client-side tool definitions (Web Search, File Readers).

The gateway eliminates these problems through a **Dynamic Synthetic Transport Tool Protocol**, **Instant-Emission SSE Streaming Engine**, and **Adaptive Trailing Turn Normalization** with zero model ID renaming or upstream pollution.

---

### 🌟 Key Architecture & Features

1. **Synthetic Transport Tool Protocol**:
   - Generates a unique, high-entropy transport tool per request (e.g., `agw_emit_7f3a91c8d42e6b1a9c02a547`).
   - Instructs the model to emit its complete response via Function Call arguments, bypassing completion token truncation.
   - Transparently extracts and reconstructs the payload into standard `assistant.content`.
2. **Zero-Mapping Model ID Passthrough**:
   - Model IDs (such as `gemini-3.5-flash-low`, `gemini-3.7-flash-high`, `claude-sonnet-4-6`) are forwarded verbatim without artificial aliases.
3. **Genuine Tool Preservation**:
   - Preserves client tools (web search, python interpreters, etc.) without interference.
4. **No Standard Content Concatenation**:
   - Discards accidental standard preambles when synthetic tool emission succeeds, enforcing a single source of truth.
5. **Instant-Emission SSE Streaming (Low Latency)**:
   - Byte-level incremental UTF-8 and JSON escape state machine flushes validated chunks immediately, adding near-zero latency.
6. **Bounded Deterministic Repair & Empty Retries**:
   - Safely repairs trailing markdown fences, dangling quotes, and invalid syntax.
   - Automatically retries upstream when an empty response is received (default: 3 retries).
7. **Intelligent Model Classification**:
   - Transparently bypasses synthetic wrapping for vision, audio, diffusion, and embedding models.
8. **Adaptive Trailing Turn Normalization**:
   - Sanitizes trailing assistant turns into user turns, ensuring 100% compliance with Gemini/CPA requirements.
9. **Lock-Free Hot Path & Dynamic SQLite Key Store**:
   - Atomic pointer swaps and immutable memory snapshots for authentication and routing, effortlessly supporting thousands of QPS.
10. **Native `.env` File Loading & Windows Friendly**:
    - Automatically finds and parses `.env` in working directory or binary directory, gracefully stripping UTF-8 BOM.

---

### 📥 Installation & Getting Started

Antigravity Gateway is distributed as a **statically linked, standalone single binary** with zero external runtime dependencies.

> 💡 **No Go Environment Installed?**
> You **do not need Go installed**! Choose **[Option A: Prebuilt Binary / Android APK]** or **[Option B: Docker Container]** to get started immediately.

---

#### Option A: Prebuilt Standalone Binary or Android APK (Recommended / No Go Needed)
1. Download the archive for your operating system from [GitHub Releases](https://github.com/Xeltra233/Antigravity-gateway/releases):
   - **Windows (x64)**: `antigravity-gateway_windows_amd64.zip` (includes `gateway.exe`, `run.bat`, and `.env.example`)
   - **Windows (ARM64)**: `antigravity-gateway_windows_arm64.zip`
   - **Linux (x64)**: `antigravity-gateway_linux_amd64.tar.gz`
   - **Linux (ARM64)**: `antigravity-gateway_linux_arm64.tar.gz`
   - **macOS (Apple Silicon M-Series)**: `antigravity-gateway_darwin_arm64.tar.gz`
   - **macOS (Intel)**: `antigravity-gateway_darwin_amd64.tar.gz`
   - **Android Devices**: `antigravity-gateway-v*.apk` (install and run locally on your phone/tablet!)
2. Extract the archive, copy `.env.example` to `.env`, edit your upstream URL, and run (`run.bat` on Windows, `./gateway` on Linux/macOS).

---

#### Option B: Docker Container (No Go Needed)
```bash
# 1. Clone repository
git clone https://github.com/Xeltra233/Antigravity-gateway.git
cd Antigravity-gateway

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

#### Option C: Build from Source (Requires Go 1.25+)

##### 1. Install Go (if not installed)
- **Windows**: In PowerShell run `winget install GoLang.Go` or download from [Go Downloads](https://go.dev/dl/).
- **Ubuntu / Debian**: `sudo apt update && sudo apt install -y golang`
- **macOS**: `brew install go`

##### 2. Clone & Compile
```bash
git clone https://github.com/Xeltra233/Antigravity-gateway.git
cd Antigravity-gateway

# Native compilation
go build -trimpath -ldflags="-s -w" -o gateway ./cmd/gateway

# Print version
./gateway -v
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

The gateway follows 12-Factor App design principles and reads configuration following this precedence hierarchy:
1. **Operating System / Shell Environment Variables** (highest priority);
2. **File specified via `ENV_FILE`** environment variable;
3. **`.env` in current working directory**;
4. **`.env` in executable directory** (essential for Windows double-clicking or service auto-start);
5. **`../.env` in parent directory**.

#### Complete Environment Variables Reference

| Variable | Required | Default | Description |
| :--- | :---: | :--- | :--- |
| **`UPSTREAM_BASE_URL`** | **Yes** | - | Upstream API base URL (e.g., `https://api.openai.com` or custom reverse proxy, `/v1` normalized automatically) |
| **`UPSTREAM_API_KEY`** | Conditional | - | Upstream Bearer API key (required when `UPSTREAM_AUTH_MODE=bearer`) |
| `UPSTREAM_AUTH_MODE` | No | `bearer` | Upstream authentication mode: `bearer` or `none` |
| `UPSTREAM_TIMEOUT_MS` | No | `120000` | Upstream request timeout in milliseconds (default: 2 minutes) |
| `PORT` | No | `8080` | Gateway listening port |
| `HOST` | No | `0.0.0.0` | Gateway listening host interface |
| `API_KEY` | No | - | Downstream client authentication key; configure this or at least one valid key in `DOWNSTREAM_KEYS_JSON` |
| `ENV_FILE` | No | - | Custom path to load `.env` configuration file from |
| `WRAPPER_MODE` | No | `prefer` | Wrapper mode: `prefer` (recommended, injects synthetic protocol), `required` (strict), `off` (emergency passthrough) |
| `RECOVERY_POLICY` | No | `repair` | Recovery policy: `repair` (local deterministic fix), `repair_then_retry` (retry on failure), `fail` (error out) |
| `UPSTREAM_EMPTY_RETRIES` | No | `3` | Number of automatic retries upon receiving empty responses from upstream |
| `CONTROL_MESSAGE_ROLE` | No | `system` | Injected control message role: `system` or `developer` |
| `CONTROL_MESSAGE_POSITION`| No | `tail` | Injected message position: `tail` (recommended for strong adherence), `head`, `system_tail` |
| `SYNTHETIC_TOOL_PREFIX` | No | `agw_emit_` | Prefix for generated 96-bit synthetic tool names |
| `SYNTHETIC_TOOL_STRICT` | No | `false` | Whether to send strict schema validation constraint in tool definition |
| `TEXT_MODEL_PATTERN` | No | - | Optional regex override to classify models as text/chat models |
| `NON_TEXT_MODEL_PATTERN` | No | - | Optional regex override to classify models as non-text (vision/audio/embedding) to skip wrapper |
| `MAX_REQUEST_BYTES` | No | `16777216` | Maximum allowed request size in bytes (default: 16MB) |
| `MAX_RESPONSE_BYTES` | No | `16777216` | Maximum allowed response size in bytes (default: 16MB) |
| `MAX_CONCURRENT_REQUESTS` | No | `1024` | Global maximum concurrent requests |
| `MAX_CONCURRENT_REQUESTS_PER_KEY` | No | `64` | Maximum concurrent requests per API key |
| `REQUEST_QUEUE_TIMEOUT_MS`| No | `50` | Maximum queue wait time before timeout (milliseconds) |
| `STREAM_SIDE_BUFFER_BYTES`| No | `0` | Streaming side buffer size (default: 0 for instant emission) |
| `STREAM_REPAIR_BUFFER_BYTES`| No | `1048576` | Streaming repair buffer size (default: 1MB) |
| `STREAM_FLUSH_INTERVAL_MS`| No | `0` | Streaming flush interval in milliseconds |
| `ADMIN_API_KEY` | No | `admin-secret-key-12345` | Authentication key for dynamic key management endpoint (`/admin/keys`, min 8 chars) |
| `KEY_HMAC_SECRET` | No | Built-in secret | HMAC-SHA256 signature secret for downstream keys (min 16 chars) |
| `KEY_DB_PATH` | No | `./data/keys.sqlite` | SQLite database path for persistent downstream keys |
| `DOWNSTREAM_KEYS_JSON` | No | `[]` | Static downstream keys array in JSON format |
| `SHUTDOWN_TIMEOUT_MS` | No | `30000` | Graceful shutdown timeout in milliseconds |
| `MODELS_CACHE_TTL_MS` | No | `30000` | Upstream `/v1/models` cache TTL in milliseconds |
| `LOG_LEVEL` | No | `info` | Logging verbosity level: `debug`, `info`, `warn`, `error` |
| `TRUST_PROXY` | No | `false` | Trust `X-Forwarded-For` header from reverse proxies |
| `UPSTREAM_MAX_IDLE_CONNS` | No | `2048` | Maximum idle connections in HTTP transport pool |
| `UPSTREAM_MAX_IDLE_CONNS_PER_HOST` | No | `512` | Maximum idle connections per upstream host |
| `UPSTREAM_MAX_CONNS_PER_HOST` | No | `512` | Maximum total connections per upstream host |

#### `.env` File Example
```env
# Required: Upstream URL and Key
UPSTREAM_BASE_URL=https://api.openai.com
UPSTREAM_API_KEY=sk-your-upstream-key-here

# Optional: Downstream client authentication key and port
API_KEY=sk-antigravity-123456
PORT=8080
HOST=0.0.0.0

# Anti-truncation strategy
WRAPPER_MODE=prefer
RECOVERY_POLICY=repair
UPSTREAM_EMPTY_RETRIES=3

# Logging
LOG_LEVEL=info
```

---

### 🚀 Deployment Methods

#### Method 1: Windows Batch Script (`run.bat`)
Double-click `run.bat` included in the release package:
- Automatically loads configuration from `.env` in the same directory;
- If `.env` is absent and no environment variables are detected, automatically copies `.env.example` to `.env` and guides setup;
- Pauses with troubleshooting guidance upon any exit error.

#### Method 2: Linux / macOS CLI
```bash
cp .env.example .env
nano .env

./gateway
```
Or start directly via exported environment variables:
```bash
export UPSTREAM_BASE_URL="https://your-upstream-domain.com"
export UPSTREAM_API_KEY="sk-your-upstream-key"
export API_KEY="sk-antigravity-123456"
export PORT=8080

./gateway
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
sudo systemctl status antigravity-gateway
```

#### Method 4: Docker / Docker Compose
Run via Docker CLI:
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

Or via `docker-compose.yml`:
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

### 🔑 Dynamic Multi-Key Management API

The gateway includes an integrated SQLite-backed key management system. Administrators can manage downstream keys dynamically via `/admin/keys`.

#### 1. Create Downstream Key
```bash
curl -X POST http://127.0.0.1:8080/admin/keys \
  -H "Authorization: Bearer admin-secret-key-12345" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "User-Alice",
    "allowed_models": ["gemini-3.5-flash-low", "gpt-4o"]
  }'
```

#### 2. List All Keys
```bash
curl -s http://127.0.0.1:8080/admin/keys \
  -H "Authorization: Bearer admin-secret-key-12345"
```

#### 3. Revoke Key
```bash
curl -X POST http://127.0.0.1:8080/admin/keys/key_7f8a91b2c3d4/revoke \
  -H "Authorization: Bearer admin-secret-key-12345"
```

---

### 📱 Client Integration

The gateway is 100% compliant with standard OpenAI `/v1` endpoints.

#### 1. General Settings
- **Base URL**: `http://127.0.0.1:8080/v1`
- **API Key**: Configured `API_KEY` or a key from `DOWNSTREAM_KEYS_JSON`; `/v1/*` always requires a valid Bearer key
- **Model**: Original upstream model name (e.g. `gemini-3.5-flash-low`, `claude-sonnet-4-6`)

#### 2. SillyTavern Setup
1. Open API settings, set **API**: `Chat Completion (OpenAI Compatible)`.
2. **Custom Endpoint**: `http://127.0.0.1:8080/v1`.
3. **API Key**: Input your `API_KEY`.
4. Click **Connect**, select model, and enjoy truncation-free roleplaying!

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

- **Liveness Probe**: `GET /healthz` → returns `{"status":"ok","version":"1.0.7"}` (200 OK)
- **Readiness Probe**: `GET /readyz` → returns `{"status":"ready","version":"1.0.7"}` (200 OK)
- **Prometheus Metrics**: `GET /metrics` → exports standard metrics including request totals, latencies, active connections, and synthetic wrapper counts.
- **Emergency Fallback**: Set `WRAPPER_MODE=off` in `.env` and restart the gateway to revert to transparent raw passthrough.

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

#### Q4: Why are my changes in `.env` not taking effect?
**A**: Starting from `v1.0.7`, native `.env` loading is fully supported. Check:
1. Whether an environment variable with the same name exists in your system (system environment variables take precedence over `.env`);
2. Check the first log output on startup `loaded configuration from .env file (path: ...)`;
3. On Windows, use `run.bat` which guarantees loading `.env` from the script directory.

---

<a name="license"></a>
## 📄 License

This project is open-sourced under the [MIT License](LICENSE).  
Copyright (c) 2026 Xeltra233.

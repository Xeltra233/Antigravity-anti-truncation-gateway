# Antigravity Gateway (Experimental Branch)

本分支是基于主线 `main` 分支构建的 **实验性多模式图片数据流与模型 3 倍扩展分支 (`experimental`)**。

> ⚠️ **声明：本评测报告与测试数据仅供参考 (For Reference Only)**。  
> 实际生产环境中的延迟、Token 消耗及推理表现会随上游基座模型（如 Gemini、Claude、GPT 等）、视觉 OCR 能力、网络环境以及 Prompt 复杂度的不同而存在差异。

---

## 一、 三大模型变体对应说明 (模型 × 3)

网关对上游 `/v1/models` 返回的所有文本模型自动扩展为 3 个不同特性的模型入口（生图/非文本模型保持单例原样透传）：

| 模型前缀格式 | 变体名称 | 运行模式与行为 | 适用场景与 Token 特征 |
| :--- | :--- | :--- | :--- |
| **`[抗截断] 模型id`**<br>*(或原模型ID)* | **正式标准版** | **纯文本传输**：应用抗截断合成工具注入与智能恢复，无图片转换。 | **1.0x 基准**：最节省 Token，适合超长上下文对话与日常代码开发。 |
| **`[实验性] 模型id`** | **全量图片流版** | **全上下文图片流**：将上下文中的所有会话文本（全局 System + 全部历史 User/Assistant + 插件注入记忆 + 当前提问）均栅格化为无损 1024px PNG 图像流发送给上游。 | **~2.52x 膨胀**：提供最彻底的上下文抗审查、防内容过滤与抗截断隔离。 |
| **`[混合实验性] 模型id`** | **混合当前轮版** | **当前整轮图片流**：将当前最新一整轮的所有输入（包含当前用户提问、酒馆/RAG 插件动态注入的记忆与设定等）转换为 PNG 图像流，历史所有轮次对话保持轻量纯文本。 | **~1.56x~2.25x**：大幅降低多轮历史的 Token 膨胀，兼顾当前轮抗审查与上下文容量。 |

> 💡 **格式兼容性**：前缀支持带空格 `[实验性] gemini-3.7-flash-high` 与紧凑格式 `[实验性]gemini-3.7-flash-high`。

---

## 二、 核心架构特性与协议保护

1. **原生图片透传不分裂**：
   - 历史或当前消息中自带的原生图片（`type: image_url`）**100% 保持原始字节与位置原样透传**，不拆分、不分裂、不二次重采样。
2. **生图模型 100% 单例透传**：
   - 生图/图像编辑模型（如 `gemini-3.1-flash-image`、`dall-e-3`、`flux`）以及语音/嵌入模型严格保持原始单例透传，不加前缀、不重复乘以 3。
3. **Role 角色感知与酒馆/RAG 插件完美兼容**：
   - **当前轮完整判定**：以最后一条历史助手回复为界，所有在最后一条助手回复之后的消息（无论插件塞入的是 `system` 记忆、`user` 提问还是自定义 role），**全部作为当前轮 100% 完整进行图片数据流转换，绝不遗漏**。
   - **Role 缺失自动补齐**：遇到缺失 `role` 字段的消息自动补齐为 `"user"` 兜底。
   - **Role 画布标头**：文本栅格化时，在 PNG 画布顶部清晰嵌入 `[Role: User] [Part 1/2]`、`[Role: System]` 或 `[Role: Assistant]`，使视觉模型无需额外标签即可精确识别人设与多轮对话。
   - **Tool / Function 原生保护**：`role: "tool"` 和 `role: "function"` 消息作为函数调用的机器契约，绝对不转为图片，必须保持原生 JSON/文本字符串，确保工具调用闭环。
   - **下行抗截断一致性**：无论上行是文本还是转换后的图片流，下行回复依然全量通过合成 Tool (`antitruncation_response`) 封装传输，实现上行抗审查 + 下行抗截断的双向防护。
4. **智能兜底机制 (Smart Fallback)**：
   - 当单请求图片页数超限（默认 >100 页）、单图字节超限（默认 >4MB）、总字节超限（默认 >12MB）或上游多模态报错时，网关自动触发智能回退，安全无损降级至标准纯文本模式，并在响应头注入 `X-Image-Context-Fallback: true`，绝不丢失上下文或中断请求。

---

## 三、 权威基准实测数据 (仅供参考)

### 1. 评测环境与权威数据集
- **评测基座模型**：`gemini-3.7-flash-high`
- **评测涵盖领域**：
  1. `InterCode` (Linux 终端 Bash 管道流与 awk/sed/grep 复合日志分析)
  2. `ANSI VT100 / XTerm` (终端色彩转义字符与 SGR 控制序列保真解析)
  3. `SWE-bench` (终端 Git Unified Diff 补丁审查与冲突修复)
  4. `Linux Syscall Tracing` (strace 系统调用跟踪日志与阻塞瓶颈定位)
  5. `Raven's APM` (瑞文高级推理测验 3x3 矩阵布尔异或流体智商)
  6. `Mensa High-IQ` (门萨高智商高阶双重差分数列归纳)
  7. `Standard Multimodal VQA` (原生图像色度与 CIE 1931 / Display P3 色域分析)
  8. `Berkeley Function-Calling Leaderboard (BFCL)` (多轮历史 + 复杂工具链协同调用)

### 2. 三变体核心指标汇总

| 变体模式 | 平均 Prompt Tokens | Token 开销倍率 | 准确率 (Score) | 平均耗时 (Latency) | 429/截断率 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`[抗截断]` (纯文本标准版)** | **691.5** | **1.00x** *(基准)* | **100.0%** | 3.16s | 0.0% |
| **`[实验性]` (全量图片流)** | **1741.5** | **2.52x** | **100.0%** | 4.24s | 0.0% |
| **`[混合实验性]` (当前轮图片流)** | **1547.4** | **2.24x** *(多轮历史大幅节省 ~43%)* | **100.0%** | 4.51s | 0.0% |

### 3. 8 大权威用例分项实测明细

| 评测用例 / 权威来源 | 考查场景 | [抗截断] Tokens | [实验性] Tokens | [混合实验性] Tokens | 各模式准确率 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `InterCode Bash Pipeline` | 终端日志管道与多命令聚合 | 415 | 1148 (2.77x) | 1149 (2.77x) | 100% / 100% / 100% |
| `ANSI Terminal Sequences` | 终端色彩转义字符保真解析 | 382 | 1191 (3.12x) | 1193 (3.12x) | 100% / 100% / 100% |
| `SWE-bench Git Patch` | Git Unified Diff 补丁审查 | 388 | 1172 (3.02x) | 1172 (3.02x) | 100% / 100% / 100% |
| `Linux strace Tracing` | 系统调用跟踪与阻塞定位 | 731 | 1184 (1.62x) | 1185 (1.62x) | 100% / 100% / 100% |
| `Raven's APM Matrix` | 3x3 矩阵布尔异或流体智力 | 306 | 1154 (3.77x) | 1157 (3.78x) | 100% / 100% / 100% |
| `Mensa Sequence Induction` | 门萨高阶双重差分数列归纳 | 290 | 1143 (3.94x) | 1147 (3.96x) | 100% / 100% / 100% |
| `Multimodal VQA` | 原生图像色度与色域分析 | 1337 | 2240 (1.68x) | 2246 (1.68x) | 100% / 100% / 100% |
| `Berkeley BFCL Tool Chain` | 多轮历史 + 复杂工具链协同调用 | 1687 | 4691 (2.78x) | **2639 (1.56x)** | 100% / 100% / 100% |

*(注：以上数据由 `benchmarks/runner/run_benchmark.py` 直连真实上游评测得出，完整 JSON/MD 报告保存在 `benchmarks/reports/` 目录下。)*

---

## 四、 快速调用示例

### Python 客户端示例

```python
import json
import urllib.request

# 可选前缀："[抗截断] gemini-3.7-flash-high"、"[实验性] gemini-3.7-flash-high"、"[混合实验性] gemini-3.7-flash-high"
payload = {
    "model": "[混合实验性] gemini-3.7-flash-high",
    "messages": [
        {"role": "system", "content": "你是一个资深架构师。"},
        {"role": "user", "content": "请简要说明微服务架构的核心优势。"}
    ]
}

req = urllib.request.Request(
    "http://127.0.0.1:8080/v1/chat/completions",
    data=json.dumps(payload).encode("utf-8"),
    headers={
        "Content-Type": "application/json",
        "Authorization": "Bearer sk-your-gateway-key"
    }
)

with urllib.request.urlopen(req) as resp:
    print(resp.read().decode("utf-8"))
```

### cURL 示例

```bash
curl -X POST http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-your-gateway-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "[实验性] gemini-3.7-flash-high",
    "messages": [{"role": "user", "content": "你好，请自我介绍一下。"}]
  }'
```

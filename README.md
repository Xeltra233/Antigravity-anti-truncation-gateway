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
| **`[实验性] 模型id`** | **全量图片流版** | **全上下文图片流**：将上下文中的所有会话文本（System/User/Assistant）均栅格化为无损 1024px PNG 图像流发送给上游。 | **~2.98x 膨胀**：提供最彻底的上下文抗审查、防内容过滤与抗截断隔离。 |
| **`[混合实验性] 模型id`** | **混合当前轮版** | **当前轮图片流**：仅将最新一轮用户的提问转换为 PNG 图像流，历史对话保持纯文本。 | **~1.61x~2.67x**：大幅降低多轮历史的 Token 膨胀，兼顾抗截断与上下文容量。 |

> 💡 **格式兼容性**：前缀支持带空格 `[实验性] gemini-3.7-flash-high` 与紧凑格式 `[实验性]gemini-3.7-flash-high`。

---

## 二、 核心架构特性与协议保护

1. **原生图片透传不分裂**：
   - 历史或当前消息中自带的原生图片（`type: image_url`）**100% 保持原始字节与位置原样透传**，不拆分、不分裂、不二次重采样。
2. **生图模型 100% 单例透传**：
   - 生图/图像编辑模型（如 `gemini-3.1-flash-image`、`dall-e-3`、`flux`）以及语音/嵌入模型严格保持原始单例透传，不加前缀、不重复乘以 3。
3. **Role 角色感知与 Tool 调用闭环**：
   - **Role 画布标头**：文本栅格化时，在 PNG 画布顶部清晰嵌入 `[Role: User] [Part 1/2]`、`[Role: System]` 或 `[Role: Assistant]`，使视觉模型无需额外标签即可精确识别人设与多轮对话。
   - **Tool / Function 原生保护**：`role: "tool"` 和 `role: "function"` 消息作为函数调用的机器契约，绝对不转为图片，必须保持原生 JSON/文本字符串，确保工具调用闭环。
4. **智能兜底机制 (Smart Fallback)**：
   - 当单请求图片页数超限（默认 >100 页）、单图字节超限（默认 >4MB）、总字节超限（默认 >12MB）或上游多模态报错时，网关自动触发智能回退，安全无损降级至标准纯文本模式，并在响应头注入 `X-Image-Context-Fallback: true`，绝不丢失上下文或中断请求。

---

## 三、 权威基准实测数据 (仅供参考)

### 1. 评测环境与权威数据集
- **评测基座模型**：`gemini-3.7-flash-high`
- **评测权威数据集**：
  1. `MMLU-Pro` (Computer Science & Database B+Tree Analysis)
  2. `OpenAI GSM8K` (Grade School Math Multi-Step Reasoning)
  3. `UC Berkeley Hendrycks MATH` (Level 5 Number Theory)
  4. `OpenAI HumanEval` (Task 137 `compare_one` Multi-Type Handling)
  5. `NVIDIA RULER` (Long Context Multi-Needle in a Haystack)
  6. `ChartQA / DocVQA` (Multi-Column Financial Reasoning)
  7. `Standard Multimodal VQA` (Colorimetry & Chromaticity Analysis)
  8. `Berkeley Function-Calling Leaderboard (BFCL)` (Multi-Turn Tool Orchestration)

### 2. 三变体核心指标汇总

| 变体模式 | 平均 Prompt Tokens | Token 开销倍率 | 平均耗时 (Latency) | 429/截断率 |
| :--- | :--- | :--- | :--- | :--- |
| **`[抗截断]` (纯文本标准版)** | **674.9** | **1.00x** *(基准)* | 4.28s | 0.0% |
| **`[实验性]` (全量图片流)** | **1792.8** | **2.66x** | 5.92s | 0.0% |
| **`[混合实验性]` (当前轮图片流)** | **1568.0** | **2.32x** *(多轮历史大幅节省)* | 5.34s | 0.0% |

### 3. 8 大权威用例分项实测明细

| 评测用例 / 权威来源 | 考查领域 | [抗截断] Tokens | [实验性] Tokens | [混合实验性] Tokens |
| :--- | :--- | :--- | :--- | :--- |
| `MMLU-Pro (CS)` | 数据库 B+ 树分裂与 I/O 层高 | 371 | 1173 (3.16x) | 1264 (3.41x) |
| `OpenAI GSM8K` | 多步链式算术与业务推理 | 309 | 1195 (3.87x) | 1276 (4.13x) |
| `MATH Level 5` | 完全平方数论方程求解 | 254 | 1193 (4.70x) | 1277 (5.03x) |
| `OpenAI HumanEval #137` | 复杂多类型边界与代码生成 | 379 | 1168 (3.08x) | 1237 (3.26x) |
| `NVIDIA RULER` | 超长文本多 Needle 事实检索 | 554 | 1183 (2.14x) | 1256 (2.27x) |
| `ChartQA / DocVQA` | 复杂财务指标跨行加权推导 | 462 | 1186 (2.57x) | 1246 (2.70x) |
| `Multimodal VQA` | 原生图像色度与色域分析 | 1376 | 2233 (1.62x) | 2305 (1.68x) |
| `Berkeley BFCL` | 多轮历史 + 复杂工具链协同调用 | 1694 | 4693 (2.77x) | **2697 (1.59x)** |

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

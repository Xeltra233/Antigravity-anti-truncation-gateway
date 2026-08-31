# Antigravity Gateway (Experimental Branch)

本分支是基于主线 `main` 分支构建的 **实验性多模式图片数据流与模型 3 倍扩展分支 (`experimental`)**。

---

## 一、 三大模型变体对应说明 (模型 × 3)

网关对上游 `/v1/models` 返回的所有文本模型自动扩展为 3 个不同特性的模型入口（生图/非文本模型保持单例原样透传）：

| 模型前缀格式 | 变体名称 | 运行模式与行为 | 适用场景与 Token 开销 |
| :--- | :--- | :--- | :--- |
| **`[抗截断] 模型id`**<br>*(或原模型ID)* | **正式标准版** | **纯文本传输**：应用抗截断合成工具注入与智能恢复，无图片转换。 | **1.0x 基准**：最节省 Token，适合超长对话与日常代码开发。 |
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

## 三、 全量实测基准报告 (基于 `gemini-3.7-flash-high`)

以下数据基于网关真实直连上游 `gemini-3.7-flash-high`，对 8 大跨领域复杂场景（含多轮历史、原生图片、双工具调用）实测得出：

### 1. 核心指标汇总

| 变体模式 | 平均 Prompt Tokens | Token 开销倍率 | 准确率 (Score) | 平均耗时 | 429/截断率 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`[抗截断]` (纯文本标准版)** | **582.9** | **1.00x** *(基准)* | **100.0%** | 4.03s | 0.0% |
| **`[实验性]` (全量图片流)** | **1737.9** | **2.98x** | **100.0%** | 4.64s | 0.0% |
| **`[混合实验性]` (当前轮图片流)** | **1555.1** | **2.67x** *(多轮多省 ~40%)* | **100.0%** | 4.52s | 0.0% |

### 2. 8 大场景分项评测明细

| ID | 评测场景 | [抗截断] Tokens | [实验性] Tokens | [混合实验性] Tokens | 各模式准确率 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `fact_retrieval_01` | 跨多段落事实定位与长文本精准提取 | 286 | 1178 (4.12x) | 1254 (4.38x) | 100% / 100% / 100% |
| `reasoning_logic_02` | 多约束条件逻辑推理与算术推导 | 273 | 1180 (4.32x) | 1254 (4.59x) | 100% / 100% / 100% |
| `code_analysis_03` | Python 算法边界条件分析与代码修复 | 270 | 1162 (4.30x) | 1227 (4.54x) | 100% / 100% / 100% |
| `json_schema_04` | 严格 JSON Schema 实体提取 | 284 | 1179 (4.15x) | 1269 (4.47x) | 100% / 100% / 100% |
| `chinese_nuance_05` | 中文成语语义辨析与高级英文对照 | 203 | 1145 (5.64x) | 1217 (6.00x) | 100% / 100% / 100% |
| `ocr_ascii_table_06` | 复杂 ASCII 表格与数学公式符号解析 | 359 | 1156 (3.22x) | 1222 (3.40x) | 100% / 100% / 100% |
| `native_image_interleaved_07` | 原生图片与上下文文本交错理解 | 1324 | 2248 (1.70x) | 2315 (1.75x) | 100% / 100% / 100% |
| `complex_tools_history_image_08` | 工具调用与历史原生图片混合多轮复杂场景 | 1664 | 4655 (2.80x) | **2683 (1.61x)** | 100% / 100% / 100% |

---

## 四、 快速调用示例

### Python 客户端示例

```python
import json
import urllib.request

# 可选用："[抗截断] gemini-3.7-flash-high"、"[实验性] gemini-3.7-flash-high"、"[混合实验性] gemini-3.7-flash-high"
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

# Python 扩展

开发环境执行 `make setup`；安装版需要 Python 3.11。
SDK 导入名是 `seven_evaluate`。每个入口只放一个公开注册函数，按
`adapters/`、`evaluators/`、`processors/`、`conclusions/` 发现。
`_` 开头的 helper 不单独注册。

```python
from seven_evaluate import processor

@processor(id="team-summary", version="2.0.0")
def summarize(ctx):
    if ctx["phase"] == "initialize":
        return {"count": 0}
    if ctx["phase"] == "update":
        return {"count": ctx["state"]["count"] + len(ctx["cases"])}
    return {
        "processor": "team-summary@2.0.0",
        "input_digest": ctx["input_digest"],
        "data": ctx["state"],
    }
```

保存为 `processors/team_summary.py`，计划设置 `processors: [processor:team-summary]`。
此例计数的是处理的 Case，不冒充已评分量，所以没有首页 scored KPI 映射。
需要 KPI 时由处理器实际计算，再使用 [语言无关 JSON Pointer 映射](typescript-extensions.md)。

四类注册与 session hooks 的准确签名见 [SDK](../sdk/python/src/seven_evaluate/__init__.py)；
完整对照示例见 [examples/typescript](../examples/typescript)。
Subject 默认只收到 inputs、目标参数和 session，不收到 reference；Evaluator 收到 Case
与结果；Processor 分批接收 initialize/update/finalize；Conclusion 接收处理器输出与计数。

每次操作启动独立 worker；不要依赖模块内存跨 Turn 保存状态。返回值遵循
[Schema](../protocol/schemas/exchange.json)，保留 null、false、0 的区别。
超时/崩溃是执行错误，不补零，不自动重试业务请求。

扩展是可信代码：发现时加载模块会执行顶层代码，进程隔离不是沙箱。
`SEVEN_EVAL_PYTHON` 可选择明确准备的环境。容量及 state 限制见 [有界执行](bounded-evaluation.md)。

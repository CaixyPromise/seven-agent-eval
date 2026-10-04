# 同数据候选参数比较

T10 第一轮仅比较明确提供的候选配置，不自动搜索、不重置外部环境，不并行执行副作用请求。

本轮交付为冻结数据、串行候选调用和同案例配对比较。交错运行、repeat 与 holdout 提示保留为后续增量；最新 checkpoint 验证记录见 `phase4-progress.md`。

## 演示

```sh
make setup
make acceptance-compare
python3 scripts/compare_target.py --port 7455
```

另一终端：

```sh
./bin/seven-eval --package examples/compare-rag --addr 127.0.0.1:7431 serve
```

在发现页运行 `compare`，报告查看候选参数、确认计数、质量／耗时／成本、同案例不同回答与检索证据、配对调优依据，并导出 `.seven.eval`。也可 CLI：

```sh
./bin/seven-eval --package examples/compare-rag run compare
./bin/seven-eval verify /absolute/path/result.seven.eval
SEVEN_EVAL_PYTHON=/does-not-exist ./bin/seven-eval --package /another/empty/directory serve /absolute/path/result.seven.eval
```

示例故意包含两次目标错误，因此 run CLI exit 1、Run partial 是正确结果；校验仍应 exit 0。默认 7431 若已有服务请选择另一个空闲回环端口。示例成本是明确标记 `source: controlled-synthetic` 的合成事实，不是实际模型费用。

## 计划与参数映射

```yaml
schema_version: evaluate.plan/v1
id: compare
mode: invoke
preset: builtin.rag-basic@1
source: source:questions
subject: adapter:rag
parameters:
  k: [1, 5]          # 评测截断；各候选共享。
candidates:
  - id: baseline    # 首个候选为基线。
    target_parameters: {top_k: 1, strategy: ranked}
  - id: expanded
    target_parameters: {top_k: 3, strategy: ranked}
```

HTTP Adapter 显式传递目标参数：

```yaml
id: rag
type: http
endpoint: http://127.0.0.1:7455
request:
  query: $.query
  parameters: $parameters
response_path: $.answer
retrieval_path: $.retrieval
usage_path: $.usage
applied_parameters_path: $.effective_parameters
```

`$.field` 仍只选择 Case inputs；`$parameters`／`$parameters.field` 只选择候选目标参数，不运行表达式。reference 不进入请求映射上下文。所有候选请求字段在调用前校验；HTTP 无显式参数映射的比较计划无效。普通单配置 invoke 计划也可用 `target_parameters`；与 candidates 互斥，不隐式合并。

Python Subject 从 `ctx["target_parameters"]` 读取参数。可返回：

```python
return {
    "response": answer,
    "parameter_receipt": {
        "requested": {},  # 执行器按计划覆盖，不能由插件冒充请求。
        "applied": actual_parameters,
        "state": "unconfirmed",  # 执行器重新核对。
    },
}
```

目标返回对象覆盖所有请求键且值相同为 target_confirmed，值不同／缺键为 mismatch，未返回对象为 unconfirmed。JSON 数字按精确数值比较（3 与 3.0 相同，大整数不丢精度；布尔值不等于数字）。目标可返回额外默认字段；请求键内部对象按完整值核对，不把部分嵌套回执当作确认。

这只能证明目标声明了有效配置；要独立证明参数执行，需要目标 trace 或行为验收。受控样例通过不同检索数量和顺序验证确实执行，不能替代用户真实目标验收。凭据经 Adapter／judge 环境变量提供，不放在 target_parameters 中。

## 统计与证据

- 同一份规范化输入冻结后执行所有候选。输入摘要、源文件版本、执行配置与 SDK 快照沿原结果机制保留。
- 每个候选的指标按 `metric_id + evaluator + parameters` 完整身份独立统计。评测 K、judge 模型／prompt 配置、成本 currency/source 不混合。
- 有效均值只使用该候选 observed；有效／计划案例数明确显示。图表展示各自有效均值，不伪装成相同有效分母。
- 配对统计只用双方都 observed 的同一原案例；同时输出基线均值、候选均值、候选减基线的差值、数量与 case_ids。没有配对则 null，不是零。
- F1 越高、耗时／同币种成本越低是单指标方向，不代表综合最优或显著性结论。调优依据可下钻对应案例；没有参考或价格事实时不猜测正确率／费用。
- `processors: []` 显式关闭统计；自定义 Processor 通过 `ctx.comparison`、`ctx.candidates`、带候选身份的 cases 实现替换。默认比较统计不会在 Go 或 React 补造。
- 一个 journal 记录整次比较。取消或进程中断保留已经完成的候选案例，恢复不自动重放不确定调用；未执行案例不是零分。

## 边界

2–8 个候选，每份参数最多 64 KiB，累计最多 1000 次 Case 调用。当前只支持 HTTP 和独立 Python Subject；Episode／共享 session、native Seven 目标参数比较、跨运行随机重复试验、自动搜索和 TypeScript SDK 不在本轮。复用默认 Processor 只是一种开箱配方，扩展可自行定义统计与决策规则。

`.seven.eval` 1.0 增加可选 Run.comparison 与 SubjectResult.parameter_receipt，旧包仍可导入。结果包包含候选、输入与证据、指标、配对统计、结论、码表和自包含 HTML；新查看器不启动 Python、不执行包内扩展代码。

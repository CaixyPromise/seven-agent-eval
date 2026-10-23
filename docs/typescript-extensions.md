# TypeScript / JavaScript 扩展（T11）

开发环境先执行 `make setup`。本轮验证 Node 20.19.5，`SEVEN_EVAL_NODE` 可覆盖 PATH 中的 `node`；内置配方和码表仍依赖 Python。结果包查看不依赖两种运行时。

## 约定目录与注册

`adapters/*.ts|js` 放 Subject，`evaluators/`、`processors/`、`conclusions/` 分别放对应扩展。每个入口恰好导出一个注册函数；同步返回和 Promise 均可。`_` 开头的 helper、`.d.ts`、`.test.ts|js`、`.spec.ts|js` 不单独发现，但本地 helper 可以被相对导入。

```ts
import { processor } from "@seven-agent/eval";

export default processor({ id: "team-summary", version: "2.0.0" }, ctx => {
  if (ctx.phase === "initialize") return { count: 0 };
  if (ctx.phase === "update") return { count: ctx.state.count + ctx.cases!.length };
  return {
    processor: "team-summary@2.0.0",
    input_digest: ctx.input_digest,
    data: ctx.state,
  };
});
```

此示例只计算处理过的 Case 数，因此不声明 scored KPI。已评分量应统计具有
`status: observed` 指标的 Case；完整 KPI 实现见 `examples/typescript/processors/team.ts`。

在计划中设置 `processors: [processor:team-summary]` 替换默认统计；省略继承，`[]` 关闭。完整可运行示例是 `examples/typescript`：`typescript`、`python` 对照计划，`missing` 关闭统计，`conflict` 演示两个首页 KPI 来源。

`subject` 返回 SubjectResult（回答字段为 `response`）；`evaluator` 返回 Metric 数组；`processor` 在 initialize/update 阶段返回可序列化 state，在 finalize 阶段返回 ProcessorOutput，update 每次最多接收 32 个 CaseResult；`conclusion` 接收统计输出与计数而非完整案例，返回 Conclusion 数组。共享字段使用 snake_case，权威结构为 `protocol/schemas/exchange.json`，SDK 构建生成 `dist/exchange.d.ts`。context 类型由 SDK 导出。旧的整份 Run Processor 接口已移除，详见 `docs/bounded-evaluation.md`。

Subject 只接收 `inputs`、`target_parameters`、`session`，默认不含 references：

```ts
import { subject } from "@seven-agent/eval";
import { randomUUID } from "node:crypto";

export default subject({ id: "session-target" }, async ctx => ({
  response: { inputs: ctx.inputs, session: ctx.session },
}), {
  create: () => ({ id: randomUUID() }),
  cleanup: async ctx => { /* 仅清理 ctx.session 标识的本轮资源 */ return null; },
});
```

Go 传递可序列化 session；每次操作都是新进程，不应依赖模块内存跨 Turn 保存状态。Episode 内共享、Episode 间隔离，开启 `parameters.cleanup_sessions: true` 后在运行结束清理创建的会话。副作用调用不自动重试。

## 模块、执行和错误边界

- 使用 ESM 和 `.ts/.js`，无需手动编译。Go esbuild 转译/打包，SDK 和示例通过独立 tsc 检查；esbuild 本身不做类型检查，参见 [官方说明](https://esbuild.github.io/content-types/#typescript)。不读取用户自定义构建配置。
- 支持 `@seven-agent/eval`、`node:` 内置模块、Package 内的相对 TS/JS/JSON 模块。第三方裸包、越出 Package 的路径、符号链接及 CommonJS require 不受支持。npm 发布和第三方依赖在后续阶段。
- 发现只运行 describe，不执行注册的业务函数；**模块顶层代码会执行**。这是可信扩展进程，不是安全沙箱；Node 内置模块仍有进程用户权限。
- 运行准备冻结源文件、本地 helper、SDK/worker、锁文件和编译入口，记录摘要及 Node 版本；执行校验冻结 worker 与入口摘要。修改原 helper 不影响已准备运行。输入图限制 2,000 文件/64 MiB；编译入口 8 MiB；worker 输出受 Go 统一上限约束。
- `console.*` 写 stderr；stdout 专供 JSON-RPC。直接写 stdout、拒绝 Promise、退出、超时、取消和无效 Schema 均成为显式错误，不补零，不自动重放 Subject。
- null、false、0 分别保留；安全整数为 number，超出安全范围的整数为 bigint，并以 JSON 数字往返。不要把大整数先转为 number。undefined、NaN、Infinity、循环对象及非普通 JSON 对象拒绝输出。

## 首页 KPI 映射

`ProcessorOutput.report` 可选，版本 `1`，`kpis.scored/coverage/anomalies` 使用相对于 `data` 的 JSON Pointer（含标准 `~0`、`~1` 转义），没有表达式或脚本执行。

- scored/anomalies 为非负安全整数；coverage 为 0–1。字段省略或显式 null 显示 `—`。
- 声明的路径缺失、类型错误或越界：处理器输出无效，不能借用旧值。计数大于 JS 安全整数范围被拒绝，避免页面静默舍入。
- 多个输出声明 report：保留所有原始输出，首页指标显示缺失并提示来源冲突。
- 没有显式映射就不提供首页 KPI；内置 Processor 同样必须声明映射，不按 ID 或同名字段猜测。
- 总样本数仍由 Run 提供；结论仍由 Conclusion 产生。RAG、stream、比较图表的数据契约保持原样，不存在通用图表脚本。

## 验证入口

```sh
make test
make acceptance-typescript
./bin/seven-eval --package examples/typescript discover --json
./bin/seven-eval --package examples/typescript run typescript
./bin/seven-eval verify artifacts/t11/typescript.seven.eval
```

验收脚本执行 TS/Python 对照、关闭/冲突计划、导出、删除临时 Package、双运行时不可用的独立查看器完整 Run 比较。浏览器实际操作记录在 `docs/phase4-progress.md`；脚本成功不等于浏览器验收。

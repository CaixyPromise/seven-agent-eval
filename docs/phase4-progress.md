# 第四阶段进度：T10 参数比较

沿用 `codex/agent-rag-phase3` 和单一工作线，不创建新工作树、独立任务或远程仓库。

## 基线

- 第三阶段受控闭环：`8d5b1f4`。
- F1 checkpoint：`88e2211`，本地提交；收 checkpoint 前 `make test` 退出 0。
- T10 第一轮已交付：明确候选、冻结数据、串行调用、配对比较、样本下钻与结果包迁移。本机受控验收于 2026-09-28 通过，2026-10-05 审查并刷新验收后收本地 checkpoint。第四阶段整体尚未完成，T11 未启动。
- 本阶段仅候选配置比较；没有自动搜索，也没有同时实施 T11 TypeScript SDK。

## 与原任务卡对齐

父仓库 `docs/seven-agent-evaluate-v1-task-breakdown.md` 已改为“T10 第一轮已交付”。原卡片中的交错运行、repeat、holdout 提示保留为后续增量；共享 Episode／native Seven 参数比较与真实目标验证同样留待后续。它们没有计入本轮交付，也不因收 checkpoint 而标记完成。父目录文档独立保留在父仓库，本提交不包含它。

## 公共契约与执行

计划仍为 `mode: invoke`，增加 `candidates: [{id, target_parameters}]`；2–8 个候选，首个为基线。Evaluator 参数只使用共享 `parameters`，目标调用只使用 `target_parameters`。候选参数每份最多 64 KiB，总调用数最多 1000。

输入在第一次目标调用前规范化并冻结；每个候选使用完全相同的案例列表，串行执行，不自动重试。结果保留 `candidate/id`、原案例 ID、目标参数回执。Episode 和 native Seven 的目标参数比较暂不支持；共享环境不并行，也不做 prepare/reset。独立 Python Subject 可从 `ctx.target_parameters` 读取候选参数。

HTTP Adapter 请求映射增加 `$parameters` 和 `$parameters.field`；旧 `$.field` 仍仅选择 inputs，reference 不进入调用上下文。要求显式映射，所有候选的请求字段先校验。`applied_parameters_path` 可映射目标返回对象；没有返回时为 unconfirmed，返回值与请求值不一致为 mismatch，一致为 target_confirmed。确认仅是目标自报，不是独立证明。凭据只能走既有环境变量引用，不允许在目标参数中携带会被脱敏的凭据。

运行用一份 append-only journal；候选调用中断后保留已完成结果，不重新执行。Run.comparison 冻结基线、原案例 ID、规范化输入摘要；原 DatasetRevision 继续保留。新字段均可选，旧离线与 stream 包仍沿用 1.0。

## 统计与页面

默认 Processor 根据完整指标身份分候选统计。有效均值各有 observed/total；配对均值和差值只用基线与候选均可计算的同一案例。成本按 currency/source 隔离；没有价格或用量事实时不估算。生成 F1、耗时、成本的配对调优依据和对应 case_ids，不宣称统计显著性、唯一最优或通用业务通过。

`processors: []` 关闭比较统计；自定义 Processor 不补造默认值，原始候选案例和参数证据仍可查看。现有 Ant Design 报告页新增比较卡片、指标表、ECharts 对照、同案例候选 Tabs 和调优依据；没有独立的新工作台。页面只消费 Processor 输出，不计算评分或配对差值。

## 验证记录

### T10 checkpoint 复核（2026-10-05，Asia/Singapore）

基线 `88e2211`；本地 checkpoint 提交标题为 `feat: deliver candidate parameter comparison checkpoint`，准确 SHA 可通过 `git log -1 --format=%h -- docs/phase4-progress.md` 查询。本轮只收 T10 第一轮与回归发现的 judge 传输错误修复；不启动 T11，不推送。

审查按需求符合度与实现质量分别进行：

- 需求：候选参数与评测参数独立；输入冻结后串行调用；参数回执区分一致／不一致／未确认；配对差值使用同案例有效交集；缺失保留 null；Processor 关闭／替换及旧包导入有效。交错、repeat、holdout 明确移为后续增量，未计入第一轮验收。
- 实现：复用现有执行、持久化与报告机制，统计由 Processor 产生；凭据使用环境变量，结果包不执行扩展。新增负向断言确认后续候选映射失败时业务调用为零，以及包中候选输入不同会被拒绝。
- 回归首次发现 judge 响应读取阶段的 ConnectionResetError 未归类；已补 OSError → `judge_transport_error`，保留空值且不重试。测试目标先读完请求体，避免非法 JSON 场景偶发变成连接重置；新增模拟响应读取重置的确定性测试。首次 `make test` 退出 2 的失败已修复，以下为最终结果。

| 命令 / 验证 | 最终结果 |
|---|---|
| `make test` | 0；Go race 全通过，Python 26 项、前端领域 10 项、TypeScript/Vite 构建通过 |
| `go vet ./...` | 0 |
| `make acceptance-compare` | 0；包含最终产品源码构建与 15 次真实回环 HTTP 调用 |
| `python3 scripts/acceptance.py` | 0；原离线 replay/HTTP/参考隔离与包迁移回归 |
| `uv run --project sdk/python python scripts/live_acceptance.py` | 0；ACK/重启/去重、预算、修订、重算、exporter 回归 |
| 加强后的 `uv run --project sdk/python python scripts/comparison_acceptance.py` | 0；原 Package 删除后，在另一个新目录启动无 Python 查看器，完整 Run JSON 一致，HTML 报告与页面资源可读 |
| `./bin/seven-eval verify artifacts/phase4/browser-comparison.seven.eval` | 0；9 月 28 日浏览器生成包仍能导入 |
| `git diff --check` | 0 |

最终产物是 `artifacts/phase4/20261004T174433-029b484a20cfa324.seven.eval`、`artifacts/phase4/acceptance.json`、`artifacts/phase4/report.html`。Run ID 使用 UTC，所以日期前缀为 10 月 4 日，对应本地 10 月 5 日验收。示例两次故意失败保持 partial；15 次已处理不等于全部成功。

本轮没有改变前端展示逻辑，浏览器交互与截图沿用下方 9 月 28 日证据；本轮新增的是可重复的独立查看器 HTTP/API 验证，不把它记成新的浏览器操作验收。Vite 大 chunk 警告仍为已记录非阻断项。生成的结果包和截图保持忽略，提交包含生成入口、示例和验证记录。

### 首轮实现验收（2026-09-28）

2026-09-28 从当前源码执行，以下命令均退出 0：

```sh
make test
go vet ./...
make build
make acceptance-compare
python3 scripts/acceptance.py
uv run --project sdk/python python scripts/phase3_acceptance.py
uv run --project sdk/python python scripts/seven_acceptance.py
uv run --project sdk/python python scripts/live_acceptance.py
./bin/seven-eval verify artifacts/phase4/browser-comparison.seven.eval
git diff --check
```

- Go race：core/server 全通过；Python 25 项、前端领域 10 项通过；TypeScript 检查及 Vite 构建通过。
- 比较专项：冻结输入（首个调用后修改源文件不影响后续候选）、真实 HTTP 参数映射、Python Subject、Processor 关闭与替换、取消保留部分案例、配对分母反例、币种隔离、重复指标身份拒绝及精确 JSON 数值回执。
- 可重复验收脚本实际记录 15 次 HTTP 调用，3 个候选 × 5 个案例；两次故意失败，所以运行是 partial，不能当成全成功。正常案例返回 1/3 个检索文档；未确认、不一致与证据不足均独立保留。
- 离线、RAG、受控 Seven SSE/Episode 与在线恢复、预算、修订、重算回归均通过。真实 Seven 和真实模型质量仍 NOT RUN。
- 结果包：`artifacts/phase4/20260928T070339-1a4e0afc4b578d28.seven.eval`；同目录 `acceptance.json` 和 `report.html`。这些为忽略的可重建产物。

### 浏览器闭环

独立 Playwright Chromium，本轮会话 `t10`，使用真实 Vite 构建与 Go server。隔离 Package 启动在随机回环端口，不修改原有 7431 服务。

1. 从发现页点击“运行评测”，实际创建 `20260928T065724-d22083b5a8d54dce`；页面更新至 15/15、partial，显示 2 次异常。
2. 查看固定评测 K=1/5 的 Precision/Recall/F1 候选对照；横轴为目标配置，不是评测 K。比较卡和详情来自 Processor/CaseResult。
3. 打开同案例 Drawer，切换 top-1/top-3，确认后者有 d1/noise/d2 三个文档；不一致案例显示请求与返回参数，Escape 可关闭 Drawer。
4. 实际浏览器下载为 `artifacts/phase4/browser-comparison.seven.eval`，CLI verify 通过。
5. 在另一空目录、`SEVEN_EVAL_PYTHON=/does-not-exist` 的独立查看器通过页面导入该下载文件。原包 run.json 与查看器 API 经 `jq -S` 规范化后 `diff` 退出 0：输入、证据、指标、配置、结论、码表及统计一致。
6. 1536×1024 桌面及 390×844 窄屏查看，窄屏 body scrollWidth=390；宽表由容器横向滚动保留所有列。reduced-motion 媒体设置实际为 reduce，重新加载仍可展示图表；共享 ECharts 封装在初始化、更新及媒体变更时遵循设置，卸载清理实例、Observer 与监听器。没有把代码检查当作运行时资源泄漏压力测试。
7. 独立查看器浏览器 console：0 errors / 0 warnings。本轮没有重复宣称第二阶段的完整 200% 缩放、网络故障、长 ID 或全键盘 Gate；T10 记录覆盖本轮新增流程与布局。

截图在 `output/playwright/phase4/`：`quality-desktop.png`、`same-case.png`、`mismatch.png`、`narrow.png`、`portable-same-case.png`、`portable-report.png`。增量视觉与文案复核见 `docs/design/parameter-comparison-review.md`。

## 当前边界

- 参数回执是目标自报；受控目标额外通过真实检索文档数量证明参数改变了输出，不能据此宣称任意外部目标已经独立验证。
- 示例费用是 `controlled-synthetic` USD 数据，不是真实账单；无成本事实时不估算。质量均值分母与配对比较分母分别保留，缺失和错误不是零分。
- 调优依据只是同案例有效观测差值，不是显著性检验、自动搜索或唯一最优建议；未确认／不一致的参数证据应一并查看。
- 本轮复用已批准的 Ant Design 报告壳，没有重做整页图像原型；新增组件分目录、图表惰性加载，统计计算不进入 React。
- 已知非阻断风险：Vite 主 chunk 约 843 KiB、共享 ECharts 约 536 KiB；比较卡约 6 KiB、图表约 0.87 KiB，为独立 lazy chunk。没有将全局分包重构扩入 T10。
- 后续仍为 T11、多参数实际平台验证、T12 容量/故障门槛、T13 安装发行与 T14 真实端到端验收；不扩大本轮为这些能力的交付。

# 第二阶段：持续评分与窗口报告

## 当前状态（2026-09-22 集中收尾）

**T05–T06 已完成本机受控范围的实现和验收。** 当前源码通过自动回归、真实 Go server 页面流程、窗口导出与独立重开。验收只证明本机回环 EEP、明确支持的 OTLP JSON 子集和受控事件源，不代表真实 Seven Agent 生产接入或容量 SLA。

普通 Chrome 的旧扩展会话访问 `127.0.0.1:17431` 曾返回 `ERR_BLOCKED_BY_CLIENT`；本轮重新在普通 Chrome 新标签页打开当前服务成功，并用浏览器原生缩放控件实际达到 `Zoom: 200%`，检查后恢复为 `Zoom: 100%`。独立 Playwright Chromium 继续承担确定性业务流程和故障注入。

## 本轮实现与口径

- 按持久化接收序号串行调度；暂停在每次 dispatch 前检查，允许已经执行中的 Case 完成，后续 Case 留在积压中。暂停本身持久化，重启仍暂停。
- judge 必须配置采样率及预算。采样按单位、证据版本、Evaluator 版本和配置确定；每个计划窗口的预算由所有 judge **共享**，一次被选中的 Evaluator 调用占一个名额；证据修订会占新名额。决策在调用前持久化。
- 重启遇到已开始但无持久化结果的 judge，保留预算占用、记录 `evaluation_interrupted`，不重试不确定的付费调用；确定性评测允许重算。扩展上下文提供稳定 `idempotency_key`。
- journal 写入与同步决定 ACK；SQLite 失败仍更新内存事实并标记 `projection_degraded`，重启重建投影。未完成的无换行尾记录恢复时截掉，完整但损坏的记录拒绝启动。写入或同步失败后停止继续追加，等待恢复。
- OTLP span 集合和 quiet/timeout 起点跨批持久化。完整证据等待 quiet period；缺根或缺输出到超时后记证据不足，迟到的新 span 产生新 revision。相同 span ID 的不同内容拒绝，不静默覆盖。
- OTLP JSON turn 接入要求 resource 上的 `seven.eval.turn.id`，一个 resource/trace 组装单位对应一个 turn；并非任意跨 turn span 图的通用映射器。支持字符串、整数/浮点 token 属性，优先根用量聚合，否则汇总 LLM 子 span。
- 单位隔离使用 plan/source/project/trace，turn 再包含 turn ID。源 Case ID 不要求全局唯一；窗口使用命名空间 Case ID，`scope.facts.origin` 保留源事件、trace、turn 和原 Case ID。
- 同一单位固定首次归属的事件时间窗口，更高证据版本替换旧贡献；同版本不同事件安全冲突。重复不改变分母。窗口修订使用相关接收/评分 journal 序号；它是单调审计版本，不是连续的第几次修改。
- 展示过的正常窗口快照保存在 `live/windows/`；ID 包含修订、窗口状态及有效配方摘要。详情不会因新 revision 失效；导出原快照，Drawer 可另选“查看最新”。
- Processor 接收样本及接收/开始/完成时间事实，输出窗口统计、原因、异常及 series。输入摘要覆盖样本、窗口范围和时间事实。处理器关闭或未输出统计时，不在 Go/React 中补默认评分数值。
- “已评分”是至少有 observed 且无执行错误的单位；失败、完全不可计算、其他无有效评分和 pending 构成互斥主分区。“涉及未采样”“涉及预算耗尽”按单位计数，可与确定性已评分重叠，不作为互斥分区相加。原因分布按指标计数。
- 评分延迟为接收证据到完成评分计算，包含排队，不包含随后 journal Sync 的时间；业务调用耗时仍是 `latency.ms`。OTLP 等待组装的时间不包含在归一化事件的评分延迟中。
- 首屏评分统计覆盖所选事件时间范围；计划待处理量覆盖计划全部积压。积压 series 由 Processor 按接收/完成时钟和包括历史证据版本的事实重建，而不是把事件时间窗口 pending 当历史积压。
- exporter 保留 in-flight 事件重试、不回队尾；有界队列立即返回，202 的逐项 rejected 不误记 sent。sent/retried/dropped/rejected 可核对；它是内存旁路队列，不保证业务进程退出后的本地队列恢复。

## 可重复验收入口

```sh
make test
go vet ./...
make build
python3 scripts/acceptance.py
make acceptance-live
```

`scripts/live_acceptance.py` 创建临时 Package，以真实 HTTP、SIGKILL、重启和 SDK exporter 验证：暂停接收 → ACK 后终止 → 重启仍暂停 → 重传 duplicate → 恢复评分与预算耗尽 → 证据修订 → 冻结旧快照 → daemon 断线与 exporter 恢复 → 同一封闭窗口 Processor/Conclusion 精确重算比较 → 导出 → 不存在 Python 路径的干净查看器重开。

产物：`artifacts/phase2-closeout/acceptance.json`、`window.seven.eval`、`report.html`。这些是本机可重建产物；脚本随源码保存，产物不强制加入 Git。

2026-09-22 最终回归命令：`make setup`、`make test`、`go vet ./...`、`make build`、`python3 scripts/acceptance.py`、`make acceptance-live`、结果包 `verify` 和 `git diff --check` 均为 exit 0。测试覆盖 Go race、8 项前端领域测试、10 项 Python 测试及 TypeScript/Vite 构建。Vite 仍有 chunk 大于 500 kB 的非阻断警告；本阶段没有扩大为分包重构。最后一轮 exporter 断线验收：2 个事件成功发送、2 次重试、0 丢弃，emit 约 0.068 ms，仅为本机受控测量，不是容量 SLA。

Go 专项回归另覆盖投影失败、未 ACK 尾记录、暂停中的在途边界、零采样和缺预算计划、judge 中断不重放、OTLP 跨批恢复、缺根超时及迟到补全、同源 Case ID 冲突、窗口修订分母、禁用 Processor 和在线/离线一致性。Python 测试覆盖 outage/FIFO/队列满、逐项 ACK、关闭边界及统计分区/延迟/积压。

## 浏览器与视觉 Gate

- 独立 Chromium 使用真实构建产物和 Go server 验证 SSE、暂停期间继续接收、恢复清空积压、窗口下钻、打开旧快照后出现新 revision 提示、窗口下载与 CLI 校验。
- 固定 1536×1024、DPR 1 桌面截图：`output/playwright/phase2-closeout/desktop.png`；Drawer：`drawer.png`；390×844：`mobile-top.png`；200% 等效布局修复后：`zoom-200-layout-fixed.png`。
- 390×844 下页面壳改为顶部导航，计划选择和控制区换行，KPI 与图表单列，关键操作可达。长样本 ID 在 Drawer 中允许换行，不挤出操作。
- 键盘焦点顺序覆盖跳转链接、导航、导入、计划、范围、刷新、暂停和窗口详情；Escape 关闭 Drawer。reduced-motion 浏览器上下文返回 true；本轮进一步验证图表初始化后画布在 250 ms 内保持一致，切换到空计划后 ECharts 实例从 4 个归零，覆盖初始化、更新稳定和卸载清理。
- 受控 500/SSE 断线响应含模拟 token、绝对路径和 stack 文本；页面只显示稳定的“实时更新已断开”及重试动作，敏感标记未出现在正文或可访问树。
- 原型和实现按 1536×1024、DPR 1 桌面视口复核；原型是静态方向稿，使用了与验收 fixture 不同的示例数值，因此不声明像素级 1:1 或“同数据”。实现保留信息优先级，但采用两列图表和“图表 + 精确表格”纵向结构。Copy gate 与视觉 Gate 已通过本机受控范围。

## 浏览器补充验收（2026-09-22）

新增可重复 fixture：`scripts/browser_fixture.py` 与 `testdata/browser-acceptance/`。`prepare` 可生成无 stream 计划或完整场景 Package，`seed` 使用当前时间附近的确定性事件，覆盖空计划、长 plan/trace/case ID、Processor 关闭/替换、judge 未采样、judge 预算耗尽和 Evaluator 执行错误。fixture 只调用本机 EEP 接口，不调用 Subject。

本轮从当前源码完成以下真实浏览器检查：

- 空计划在 SSE 快照先于初始报告返回时曾持续显示“正在读取窗口”；已修复为收到快照即结束加载，并复测显示“所选时间内还没有事件”。无 stream 计划单独显示“还没有可用的持续评分计划”。
- Processor 关闭时统计 KPI 为 `—` 且图表明确说明未提供窗口统计；替换计划只消费自定义 Processor 输出。judge 未采样、预算耗尽和 Evaluator 错误在 API/Drawer 中保持不同原因，不转成零分。
- 详情延迟响应显示局部骨架；受控 404 和导出 503 只显示审核过的中文提示，模拟 token、绝对路径、stack 和上游响应未进入正文；解除拦截后局部“重试”恢复。
- 延迟第一个详情请求、关闭后切换计划并打开第二个窗口，最终 Drawer 保持 `processor-replaced`，旧响应没有覆盖新选择。
- revision 13 Drawer 在 revision 42 到达后保留旧快照并提示“已有更新”；在切换前后分别由浏览器导出，两个包内 `run.json` 的窗口 revision 分别为 13 和 42，且 CLI `verify` 均通过。
- revision 42 包在干净目录、`SEVEN_EVAL_PYTHON=/nonexistent/seven-eval-python` 下启动查看器，页面显示“窗口快照 · 修订 42”、窗口范围、4 条样本、指标汇总、处理器输出/码表/执行配置入口；未启动 Python worker。
- 普通 Chrome 原生 200% 缩放下，计划、时间范围、刷新/暂停及 KPI 仍可访问；随后恢复 100%。该项取代此前仅用 768×512 CSS 视口模拟真实缩放的记录。

本地证据位于 `output/playwright/phase2-supplement/`：桌面长 ID、无计划、独立查看器截图及 revision 13/42 结果包。目录继续忽略，不提交可重建二进制。

补充提交前从最终源码重新执行：`make test`、`go vet ./...`、`python3 scripts/acceptance.py`、`make acceptance-live`、两个浏览器导出包的 `seven-eval verify` 以及 `git diff --check`，退出码均为 0。`make test` 包含 Go race、8 项前端领域测试、10 项 Python SDK 测试和前端生产构建；`make acceptance-live` 重新构建 CLI，并再次通过 ACK/SIGKILL/重启、顺序与预算、revision 替换、exporter 断线恢复、在线/离线精确重算、结果包完整性和无 Python 查看器检查。Vite 的大 chunk 警告仍是已知非阻断风险。

仍属于后续范围：真实 Seven Agent 生产来源（T07/T14）、容量与保留故障矩阵（T12）、跨机器安装发行（T13）。OTLP 只支持 JSON：OpenInference root／AGENT／CHAIN，RETRIEVER 上下文，LLM token 属性，trace/resource-level turn ID，以及 quiet period、timeout 和 revision；不支持 protobuf 或任意跨 turn span 图。

## 目标

在不调用 Subject、不重放生产请求的前提下，持久化已完成的事件，异步运行既有评测配方，并把每个事件时间窗口导出为可离线查看的 `.seven.eval` 快照。

## 实施边界

- 支持 EEP `POST /api/ingest/events` 和本机回环 OTLP/HTTP JSON 的 OpenInference 子集。
- 支持 `mode: stream` 的 `trace`、`turn` 计划；本阶段没有跨请求 session 生命周期。
- journal 是确认后的事实来源；SQLite 是可重建查询投影。暂停只停评分，不停接收与持久化。
- 仅使用完成事件中的 `Case`、`SubjectResult`、检索上下文和既有 Evaluator/Processor/Conclusion。线上路径没有 Subject、prepare/reset 或重放调用。
- 默认窗口为 60 秒、迟到水位线为 60 秒。窗口快照在水位线前标识为 `partial` / `provisional`，导出后保持不可变。

## 当前交付物

- `examples/live`：受控 stream Package；`scripts/live_fixture.py`：无凭据事件源。
- `internal/core/live.go`：journal、去重、重启重建、暂停/恢复、异步评分、修订选择、窗口结果和 OTLP 映射。
- `/api/live/*`、`/api/ingest/events`、`/v1/traces`：本地 server 接口与 SSE。
- `web/src/features/live`：Ant Design 工作台、ECharts 图表、窗口表、异常 Drawer、导出与安全中文错误提示。
- `protocol/examples/otlp-openinference.json`：受控 OpenInference JSON fixture。

## 尚未声明为本阶段完成的能力

- 真正生产 Seven Agent 来源、外部身份鉴权、持久保留/清理和容量阈值属于后续 T07/T12/T14 验收。
- 通用 LLM judge、RAG 专项指标、自动参数搜索和 TypeScript SDK 不在本阶段范围。
- OTLP 仅支持 JSON 和明确的 OpenInference 属性子集；protobuf 会被安全拒绝。

## 旧产物说明

`artifacts/phase2/` 与旧 `output/playwright/live-*.png` 保留用于历史对照，不作为本轮最终语义和浏览器验收证据。当前结果以 `phase2-closeout` 及上述可重复脚本为准。

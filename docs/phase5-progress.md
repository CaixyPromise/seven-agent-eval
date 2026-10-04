# 第五阶段进度：T12

分支：`codex/agent-rag-phase3`。本轮从 `f204ca2` 继续，日期 2026-10-05（Asia/Singapore）。**本轮约定的离线大规模硬切闭环已完成：输入校验 → 执行 → 分批聚合 → 分页报告 → 分块包 → 无运行时重开，10,000 Case / 1,000,000 Observation 实测通过。** 不保留旧接口、旧状态或 1.0 包兼容层。T13/T14 与在线长期容量 SLA 不因此自动通过。

## 本轮交付

- 输入在 Subject 调用前全部校验并冻结；SQLite 检查重复/关联/展开身份，JSONL/CSV 顺序读取，BEIR/TREC 磁盘关联。冻结摘要损坏在调用前拒绝。
- Run 元数据不含全体 CaseResult；逐 Case journal + SQLite 投影，可重建；报告分页摘要与单案例详情分离，Episode/比较下钻同步改用分页。
- Python/Node 统一 `processor/reduce` 三阶段，批次最多 32 Case/8 MiB；内置统计 SQLite 精确聚合，不近似或截断分母；Conclusion 接收统计与计数。
- 协议/worker/结果包硬切 2.0；旧包明确拒绝。ZIP 输入/结果分块 16 MiB，单记录、元数据、条目数和累计 4 GiB 各自限制；HTML 为独立汇总，完整样本由查看器分页。
- 页面沿用 Ant Design，原生浏览器下载避免整包 Blob；单次下载领取后删除，最多 4 个待领取，10 分钟过期，正常停服清理。
- 导入保留未执行输入；后续读分块重新核对摘要；同 Run 导入串行检查；失败留下 partial，不伪装完整成功。
- 详细公共契约及支持上限：`docs/bounded-evaluation.md`。自定义处理器是可信代码，不承诺其任意实现的内存上限。

## 百万指标实测与删除确认

命令：`uv run --project sdk/python python scripts/large_acceptance.py`，退出 0。真实 Node Evaluator 每 Case 100 指标，Python 内置磁盘 Processor；不是预先生成统计文件。

| 项目 | 本轮实测 |
|---|---:|
| Case / Observation | 10,000 / 1,000,000 |
| run 命令（含默认导出） | 1,298.18 秒 |
| 整体验收（含 verify/独立导入查询） | 1,644.452 秒 |
| time -l maximum resident set size | 104,513,536 B，约 99.7 MiB |
| ZIP / 累计解压 | 6,275,122 / 126,862,091 B |
| Run 元数据 / 第一页摘要 | 48,033 / 3,333 B |
| 精确统计 | 100 个指标均值/中位数 49.5、p95 94，全部核对 |
| 无 Node/Python 独立查看器 | 元数据、首尾分页/完整详情、HTML 摘要通过 |
| 大数据清理 | PASS；finally 清理后再次检查专属临时目录不存在 |

资源数值来自 CLI 的 `time -l`，不是所有同时运行子进程 RSS 总和，不外推生产 SLA。执行二进制 SHA256：`eeb65997a94a305436e43c3750d720282b76fb793eb6489117691db31bed951c`；测量期间后续修改为报告安全文案、下载/导入防护与额外边界测试，最终二进制另跑同链路小规模回归，不冒称百万负载重新跑过。

测量仅保留 `artifacts/phase5-large/acceptance-10000.json`。本轮百万输入、SQLite、journal、代码快照、结果包已删除；结果包摘要 `bc49adc717d86a6de8ff6715ad1fb1f49ca5f6dd9167b0e6aec9853ac1f6f739` 可对账。脚本成功/失败/SIGTERM 都清理所属临时目录，SIGKILL/断电不属于 finally 保证。

另外按 manifest 实际版本精确删除了本仓库 artifacts 下 30 个旧 1.0 生成包（其中一份原为 Git 跟踪文件，本次提交记录删除）；没有删除源数据或扩展代码。浏览器 fixture 与无运行时查看器的专属临时数据在精确停止所属进程后删除。百万测试和最终 100 Case 回归的 `large_data_removed` 均为 true；没有留下百万包副本。

## 本轮验证记录

- 最终 `make test` 退出 0：Go race（core 211.639s / server 18.400s）、TS SDK 5 项、前端 11 项、Python 28 项及 TypeScript/Vite 构建。包含 Processor 预算与 CSV 多行字段上限测试。随后增加的并发重复导入测试与已有下载领取/停服清理测试单独 `go test -race ... -count=1` 通过（core 3.769s / server 5.658s）；无额外产品代码变更。
- `go vet ./...`、`make build`、`git diff --check` 均退出 0；Vite >500 KiB chunk 警告保留为非阻断风险。
- 离线 acceptance、TS/Python、T10 comparison、HTTP RAG、Seven SSE、live acceptance 均在新协议下退出 0；Seven 仍为受控环境，非真实生产。
- 新增 V2 真实累计 >64 MiB 分块测试、取消时未执行输入导入/再导出、校验后源包替换拒绝、元数据无全体 Cases、单次下载及正常停服删除测试。
- 新格式恢复/清理集成：`uv run --project sdk/python python scripts/resilience_acceptance.py --cases 3 --seconds 2 --output artifacts/phase5-hardcut-resilience`，退出 0；64 积压恢复 27.722 秒，66 received、pending=0，清理 66 记录/3 Run。此为格式切换集成检查；前 checkpoint 的千例/60 秒记录保留为历史，不冒充本轮重跑。
- 小型可打开新包：`artifacts/phase5-hardcut-resilience/window.seven.eval` 与 `report.html`；百万包按用户要求不保留。
- 最终二进制同链路复测：100 Case / 10,000 Observation，run 23.822 秒、全链路 27.569 秒、maximum RSS 43,614,208 B，精确统计与独立查看器通过并清理，退出 0；二进制摘要 `e5890c8f13c5ef908e916684d21e1c01a74b94c010cb529ad45f80fef7c0f733`。随后 `python3 scripts/acceptance.py`、窗口包 verify、`go vet ./...`、`git diff --check` 均退出 0。

## 本轮真实页面复核

独立 Playwright 会话 `t12-bounded`，真实 Go 服务与 Vite 产物；65 Case、长 ID、7 异常。实际完成：25 条分页→第三页→第 65 个完整详情、异常筛选、Escape 关闭、下载 .seven.eval、CLI verify、无 Node/Python 查看器页面重开及最后一例证据。

- 原服务端口 63270，独立查看器 60949，均为本轮临时隔离实例。
- 本轮窄屏明确 override 390×844 / DPR 1；最终桌面 override 1536×1024。最初独立查看器截图采用浏览器原生 DPR 2，不作为 DPR 1 原型对照。
- 受控详情 500 含模拟敏感文本；页面未出现原始异常，局部重试后可读实际证据。修正详情错误不再误说“报告读取失败”。
- 截图在 `output/playwright/t12-bounded/`：`drawer-desktop.png`、`anomalies-desktop.png`、`samples-mobile.png`、`independent-viewer.png`、`detail-error-final.png`。
- 本轮是分页与详情/下载局部验收，不声称重做全产品 200% 缩放、reduced-motion 或全量原型对照。
- 原型/React 技能在本轮用于复用已有 Ant Design 布局、拆分分页/异步详情组件、检查安全文案与取消请求；没有额外创造图表协议或整页视觉重构。
- 浏览器安全错误截图等待 CSS 过渡结束后重拍；`detail-mobile.png` 实际显示 390×844 长 ID、完整回答和指标，关闭操作可达。局部重试首次自动脚本等待了不存在的标题而超时，重新按真实“逐项指标”标题确认成功，未把脚本超时隐瞒为首次全通过。两个预期 500 留在浏览器控制台；无上游敏感标记进入正文。

| Before | After | Why |
|---|---|---|
| 页面依赖完整 Run.cases，全部样本常驻 | 分页摘要；打开后才读单条完整证据 | 百万指标不进入初始页面内存 |
| Episode/候选直接索引全数组 | 按 Episode/原始 Case 身份查询 | 分页后仍有跨 Turn/配置的访问路径 |
| 导出 fetch Blob 持有全包 | 单次原生下载；领取/停服清理 | 避免浏览器整包缓冲与临时包滞留 |
| 详情局部失败沿用报告文案 | 详情专用安全文案与就地重试 | 背景统计仍有效，不暗示整个报告失败 |
| 旧整份 Run Processor / 1.0 包 | reduce ABI / 2.0 分块包 | 按用户要求硬切，不保留兼容分支 |

## 历史记录（f204ca2 及输入切换中间态）

以下为前一 checkpoint 的测量、恢复与清理策略。出现的“当前”“尚未支持万级”等表述只描述该历史时间点，不能覆盖上面的新格式与本轮通过结果。

## 测试机器与冻结测量口径

- macOS 26.5.2 (25F84)，Mac15,6，Apple M3 Pro，12 CPU，36 GiB RAM。
- Go 1.25.1 darwin/arm64，Node 20.19.5；Python 采用现有 uv 锁定环境。
- 本机受控数据、回环 HTTP、串行执行；不代表生产 SLA 或真实 judge 吞吐。
- 首先测量 100 / 1,000 Case，每 Case 10 个 Observation、1 KiB 回答的持久化路径；记录墙钟、分配量、journal 和投影大小。
- 同时保留 10,000 Case / 1,000,000 Observation 目标的差距检查；未达到不得记录为通过，不通过调大常量掩盖限制。
- 在线接收、评分和恢复分别测量；全量默认 Python、Node 及混合扩展均须说明运行组合。持续运行时长和峰值资源必须随验收结果记录，不以短测外推全天稳定性。

## 实施前硬限制与全量内存路径

| 环节 | 当前实现 / 限制 | 影响 |
|---|---|---|
| 输入 | 1,000 Case；普通文件 32 MiB；先规范化全部输入 | 原万级目标尚不满足 |
| 执行 | Run 保留全部 CaseResult；worker 单操作进程 | 峰值与证据、指标数量有关 |
| 聚合 | Processor 接收整份 Run | 无分批聚合 ABI |
| 持久化 | 每 Case 重写完整结果和 SQLite body；单记录/结果 64 MiB | 写放大近似平方增长 |
| 查询 | Store.List 读取所有完整 Run；live Report 读取历史记录并聚合完整窗口 | 历史总量未有界 |
| 在线 | 待评分 10,000；单事件 1 MiB；单批 100；历史记录常驻内存 | pending 限制不等于历史容量限制 |
| 结果包 | 累计解压 64 MiB；生成 Run、HTML 等多份缓冲 | Case 数不是唯一容量标准 |

本轮已将持久化改为逐 Case SQLite 投影及运行元数据更新，运行列表不再读取全部 Case；终态结果仍为完整文件。输入、Processor、结果包及在线历史的其余限制未因此消失。

## 恢复与清理安全条件

1. ACK 以 journal 同步成功为依据；投影损坏可以重建。
2. 追加失败后禁止继续沿同一不确定 journal 写入；重启修复不完整尾行。
3. 不重放不确定 Subject。已完成 Case 保留；失败、缺失、采样和预算状态不转为零分。
4. 默认保留。清理先预览，再按明确预览执行；工作区必须停止服务、持有独占锁。
5. 待处理/评分中数据与被引用执行快照不可删除；删除证据不能删除去重语义。
6. 保留清理审计，过期查询明确；已有 `.seven.eval` 不依赖被清理目录。

## 本轮冻结的开发环境通过条件

这些是首轮测量门槛，不是生产 SLA；原万级 / 百万指标仍单列缺口。

- 持久化微基准：100 / 1,000 Case，10 指标和 1 KiB 回答；同机前后比较，指标为总时间和累计分配（不冒充峰值内存）。
- 完整离线：Python、Node 各 1,000 Case / 10,000 Observation；混合 1,000 Case / 20,000 Observation。Python/Node 各 600 秒、混合 900 秒以内；CLI 命令最大 RSS < 2 GiB。`time -l` 不代表同一时刻所有子进程 RSS 之和。
- 在线：Python + Node 混合确定性 Evaluator，1 KiB 回答；暂停后一次尝试 65 条，测试上限 64，必须接收 64 / 拒绝 1；ACK 后 SIGKILL、重启、duplicate 64，恢复积压 < 120 秒。
- 持续输入：1 event/s，至少 60 秒，全部已接收单位最终可评分；记录接收耗时、pending、daemon RSS（< 2 GiB）、journal 大小及含排队的 p95。不是 24 小时 soak，不外推长时稳定性。
- 超上限输入在执行目标前明确拒绝；未把 10,000 / 1,000,000 标记为支持。
- 清理、过期查询、去重、导出和无语言运行时的 API/HTML 重开必须通过；本轮不新增浏览器视觉能力。

## 保留策略与操作方式

默认不清理；先停止使用同一状态目录的服务：

```bash
./bin/seven-eval --package /path/to/package retention preview 2026-10-01T00:00:00Z
./bin/seven-eval --package /path/to/package retention apply <返回的 digest>
```

- 预览落盘在状态目录 `retention/previews/`，不会删除数据。执行会重新计算权威文件摘要，数据变化即拒绝旧预览。
- 首版只选择已物化、watermark 早于截止时间、且所有相关证据均已评分并超过截止时间的窗口。未查询成窗口的证据、存在 pending 的整个窗口、partial 离线运行和来源不明的孤立快照保留。
- 已结束离线运行按 journal 最后修改时间选择；保留运行及窗口引用到的 Node 执行快照受保护，包含传递引用。
- 在线 journal 通过临时文件 → Sync → 原子替换 → 目录 Sync 压缩。去重摘要、评价单位身份、版本及过期窗口范围保留；旧证据正文删除。重复事件仍为 duplicate，过期范围的新事件 / revision 返回 `DATA_EXPIRED`，不会重新评分。
- OTLP 保留 span ID / 摘要，原样重传去重、冲突拒绝；过期 span 正文不保留。
- 先发布过期凭据再删除文件；中断后查询仍明确过期，重新 preview/apply 可继续回收残留文件。审计保留在 journal 和 `retention/audit/`。
- 删除不影响已导出包；`exports/` 不在清理范围，需单独管理所占空间。清理不可恢复原始证据，执行前应导出需要保留的窗口或运行。这不是备份或底层介质的取证擦除功能。
- **限制**：去重/过期元数据仍随历史数量增长，不是无限运行的固定内存承诺；压缩需要额外临时磁盘空间，不支持边接收边清理。

## 当前测量

### 本次局部文案契约

| 事实 | 主位置 | 生命周期 | 下一步 |
|---|---|---|---|
| 时段包含已清理的窗口版本，剩余统计不代表完整历史 | 持续评分内容区的一条 warning | 仅 `expired_windows > 0` 时 | 切换时段或打开已有导出包 |
| 已清理范围没有保留窗口，不等于从未收到事件 | 该范围的空状态 | 无保留窗口且有过期版本 | 调整时间范围 |
| 指定版本已清理，重试不能恢复正文 | 详情/导出的安全错误 `DATA_EXPIRED` | 本次读取失败 | 打开清理前导出的结果包 |

沿用 Ant Design Alert 和原有 ErrorNotice，无新布局、无新的统计计算。局部渲染验收通过；不声称重新完成全部第二阶段视觉矩阵。

| 持久化基准 | 修改前 | 逐 Case 查询投影后 |
|---|---:|---:|
| 100 Case | 3.809 s / 累计分配 201,928,104 B | 1.656 s / 14,134,816 B |
| 1,000 Case | 143.188 s / 累计分配 20,787,718,256 B | 16.970 s / 136,353,864 B |

命令：`go test ./internal/core -run '^$' -bench BenchmarkCapacityPersistence -benchtime=1x -benchmem`，两次退出码 0。单次样本，不构成统计置信区间。基准仅包含持久化，没有 Subject、Evaluator、Processor 或浏览器。

### 完整工作负载与最终复测

| 实际执行 | Case / Observation | 墙钟 | 命令最大 RSS |
|---|---:|---:|---:|
| Python replay | 1,000 / 10,000 | 210.119 s | 113,491,968 B |
| Node replay | 1,000 / 10,000 | 62.041 s | 120,586,240 B |
| Python + Node replay | 1,000 / 20,000 | 397.666 s | 178,290,688 B |

数据为合成 1 KiB 重复文本、确定性布尔指标，不代表真实模型生成成本或压缩率。开发机器并非独占，部分时间并行执行了回归。计时包含准备、执行、聚合和导出。

- `uv run --project sdk/python python scripts/resilience_acceptance.py`：退出 0，`artifacts/t12/acceptance.json`。该规模测量使用二进制 SHA-256 `d60f13cf0f2142fa67e882113f324092d35a91f5613ca1f8273887744104d7c4`。
- 在线：暂停时尝试 65 条，持久化 64 / 队列满拒绝 1；SIGKILL 后 64 条原样重传全部 duplicate；恢复用时 22.803 s，无 Subject。随后按名义 1 event/s 输入 60 条，连同收尾查询计 63.750 s，总计 124 个单位；最终 pending 0，连续输入期间观察 pending 最大 1、daemon RSS 最大 42,024,960 B。
- 窗口 p95 23,739.962 ms 包含前面的暂停、重启和积压，**不是稳态每次评分约 24 秒**。
- 124 个证据单位及 3 个已完成离线运行被显式清理；重启后 duplicate 仍成立，窗口查询为 410，原 Package 删除后结果包在无 Node/Python 查看器中完整 Run 相等，HTML 可读。
- 完整规模测量后补了清理投影保留和局部页面反馈。最新 Go 二进制 SHA-256 `4be8c8ca4520f6701575f3484e90b0afce5e805934c260c7f9a9c26593d8e60f` 已执行最终集成复测：`uv run --project sdk/python python scripts/resilience_acceptance.py --cases 3 --seconds 2 --output artifacts/t12/final-smoke`，退出 0。该复测不是第二次千例容量测试。
- 最终复测清理 66 个在线单位 / 3 个离线运行，journal 855,339 → 43,681 B；Node/Python 路径设为不存在仍可执行维护和独立查看。当前源码对应的新包为 `artifacts/t12/final-smoke/window.seven.eval`，报告为同目录 `report.html`；CLI verify 退出 0。

## 当前源码回归记录

以下均在本轮执行，退出码 0：

- `make build`（SDK、TypeScript、Vite、Go）；局部 Drawer 修复后另外重新执行 `pnpm --dir web build` 和 `pnpm --dir web test`。
- `make test`：Go race 全包、SDK 5 项、Web 11 项、Python 26 项、类型检查和前端构建。
- `go vet ./...`，`git diff --check`。
- `python3 scripts/acceptance.py`：历史 replay、真实受控 HTTP、参考不泄漏、移除原 Package 后验证。
- `uv run --project sdk/python python scripts/live_acceptance.py`：ACK/SIGKILL、预算、revision 固定快照、在线/离线精确重算、exporter 断线重试、无 Python 重开。exporter 本次 emit 0.098 ms，sent 2 / retried 4 / dropped 0；队列溢出 dropped 的独立 SDK 测试也通过。
- `scripts/typescript_acceptance.py`、`scripts/comparison_acceptance.py`、`scripts/phase3_acceptance.py`、`scripts/seven_acceptance.py`（均使用 `uv run --project sdk/python python`）。
- 新增定向测试：短磁盘写入返回 ENOSPC 不 ACK、写失败禁止后续追加、残尾截断后恢复、超长尾行有界、离线 SQLite 增量投影失败后重建、重建不刷新保留年龄、清理取消/发布失败、过期去重与禁止复活、pending revision 保护、执行快照引用保护、OTLP span 摘要去重与安全 410。

磁盘故障为受控写入/发布失败注入，没有填满用户磁盘，也没有做物理断电验证。Vite 的 >500 KiB chunk 警告仍在，非本轮容量结论的替代指标。

## 验收台账

| 项目 | 状态 | 证据 |
|---|---|---|
| 当前源码与容量路径检查 | PASS | 上表，工作区开始时干净 |
| 持久化基线测量 | PASS | 上表，固定合成负载 |
| 故障/恢复集中验收 | PASS | Go race、SDK、live acceptance 及 T12 脚本 |
| 显式清理与引用保护 | PASS | 新增 retention tests、独立 CLI 预览/执行 |
| 在线持续运行与离线规模 | PASS（限定负载） | 千例离线和 60 秒在线；不替代万级 / 长期 soak |
| 导出及独立重开 | PASS | 完整 Run 比较、HTML、verify |
| 局部页面提示 | PASS（本轮局部范围） | 清理前、部分历史已清理、旧快照导出 410、390px 窄屏 |
| T12 本地 checkpoint | 本轮归档 | 以包含本文档的本地提交为准；不声明完整容量目标完成 |

## 局部浏览器复核

独立 Playwright Chrome 会话 `t12-retention`，真实 Go server + Vite 构建产物，回环端口 54991（由 fixture 动态分配），中文、浅色、桌面 1536×1024 和窄屏 390×844、截图按 CSS 像素、未改变浏览器缩放。准备命令为 `uv run --project sdk/python python scripts/retention_browser_fixture.py`。

实际先加载窗口详情，再停止本轮服务、预览并清理、重启同一状态目录。旧详情导出真实返回 410：安全提示出现，无无效重试，导出按钮禁用，已加载统计保留；不再提示此快照仍可接收迟到修订。Escape 可关闭 Drawer。两轮仅清理 fixture 中的两条合成证据及对应四个窗口版本，未清理其他工作区。

| Before | After | Why |
|---|---|---|
| 清理后的空列表与“从未收到事件”混淆 | 范围提示历史已清理；空状态为“没有保留的窗口” | 不能把剩余样本覆盖率当作完整历史 |
| 过期对象仍可点重试/导出，且旧绿色提示说可修订 | `DATA_EXPIRED` 不显示重试，导出禁用，移除冲突状态提示 | 已删除正文不会因重试恢复，迟到请求也已被拒绝 |
| 新范围告知与 KPI 需要保持原页面节奏 | 复用 `.feedback` 的 18px 间距；手机宽度下自然换行 | 桌面和窄屏都能完整阅读，不遮挡原操作 |

逐张查看过的截图：

- `output/playwright/t12/before-cleanup.png`
- `output/playwright/t12/retained-subset.png`
- `output/playwright/t12/expired-drawer.png`
- `output/playwright/t12/expired-desktop.png`
- `output/playwright/t12/expired-mobile.png`

控制台记录为本轮主动停服导致的 SSE 中断/连接拒绝，以及两个预期 410；未观察到应用脚本异常。不将这次局部检查称为重新完成 200% 缩放、全部键盘或 reduced-motion 矩阵。临时 browser 会话及本轮服务在归档前关闭；截图、临时 Package 和结果包保持忽略，不进入提交。

## 仍需在 T12 内继续的边界

1. 万级 Case / 百万 Observation 仍不支持：整批规范化、整份 Run/Processor 输入、64 MiB 结果包仍需针对大规模重新设计和验证，不能只调大常量。
2. 元数据与未清理历史仍随时间增长；没有声明全局存储配额、长期固定内存或全天候容量 SLA。60 秒受控输入不是长期 soak。
3. 首版维护要停服；未物化窗口、partial 运行、来源不明的孤立快照保守保留。真实生产、跨平台安装仍归 T14/T13。

因此本次提交应称为 **T12 首轮基线与恢复/清理 checkpoint**，不能将整个第五阶段或原 T12 完整目标标成完成。

# 有界离线与在线执行：协议 2.0

这是迭代硬切，不提供旧 Processor、旧状态目录或 1.0 结果包转换。旧测试产物重新生成；源数据与用户扩展源码不属于可删除的运行产物。

## 输入 → 执行 → 聚合

1. 全量输入先逐条校验。重复 ID、关联数据和展开后的 Episode/候选身份在临时 SQLite 检查；所有输入通过后，发布规范化 JSONL 和摘要。Subject 尚未调用。
2. 执行前再流式核对冻结文件摘要与数量，然后按序读取。默认串行，不自动重放 Subject。一个 Case 结果同步写入 journal 后更新查询投影；Run 只保存元数据。
3. Processor 使用 `processor/reduce`，`phase` 为 `initialize`、`update`、`finalize`。初始化与更新返回可序列化 `state`；结束返回标准 ProcessorOutput。Go 保存和传递 state；一次操作一个进程，不依赖 worker 内存。
4. `update` 的 `cases` 最多 32 条且编码合计最多 8 MiB；初始化/结束的 `cases` 为 null。共同上下文为 total、completed、input_digest、parameters、episodes、comparison、candidates、window；`workspace` 是本次处理器独享的临时目录，结束/错误后清理。
5. 内置 Processor 将观测放在该临时目录的 SQLite 中，精确计算均值、中位数、最近秩 p95 和配对均值；不将近似统计伪装成精确值。Python/TS 自定义 Processor 必须采用同一分批契约，可以自行使用 workspace。
6. Conclusion 只接收 outputs、total、completed 和 parameters，不再接收全体 Case。异常总数由 Processor 计算；默认报告最多附 100 个异常 ID 预览，`failure_ids_truncated` 明示截断；全部异常通过样本分页查询，不截断统计分母。

内置统计使用保守的 4 MiB 报告内容预算；超限返回处理器错误，不返回悄悄截断的统计。通用 worker 输出上限为 8 MiB。扩展是可信代码，进程隔离不是内存/权限沙箱；不能承诺任意自定义 Processor 的内存上限。

## 查询与下载

- `GET /api/runs/{id}`：元数据、统计、结论、码表和配置，不含 cases。
- `GET /api/runs/{id}/cases?offset=0&limit=25`：稳定执行序号分页摘要，limit 1–100；支持 `anomalies=true`、`episode`、`candidate`、`original_id` 筛选。
- `GET /api/runs/{id}/cases/detail?case_id=...`：一个完整 CaseResult。详情请求取消/切换不覆盖新选择。
- `POST /api/runs/{id}/export`：在磁盘生成包后返回单次下载 URL；前端交给浏览器原生下载，不生成全包 Blob。最多 4 个待领取下载，10 分钟过期；领取后及服务正常退出时清理临时包。窗口导出采用同样流程。
- 原有 GET export 直接流式发送磁盘文件，仍用于脚本。导入先落临时文件再校验，不把请求体全部读入内存。

## `.seven.eval` 2.0

ZIP 包包含 `manifest.json`、`run.json`、`catalogs.json`、`effective.json`、`schemas/exchange.json`、`report.html`，以及顺序编号的 `inputs/part-000001.jsonl` 与 `results/part-000001.jsonl`。manifest 保存所有内容摘要、输入数、结果数和窗口范围。

`report.html` 是独立汇总报告；逐条输入、证据和观测在分块文件中，使用查看器分页访问。取消运行仍带上全部已冻结输入，不能把“未执行”与“没有输入”混淆。查看器不执行包内脚本，也不启动 Python/Node；导入后的重新导出保留未执行输入。

导入验证固定文件集合、路径、条目数、单块和累计解压量、摘要、JSON/schema、记录数量、引用与输入/结果一致性。读取已校验文件的后续分块时重新核对摘要，文件被替换则拒绝。旧格式直接拒绝。

| 边界 | 当前限制 |
|---|---:|
| 展开后的 Case/调用 | 10,000 |
| 每个原始数据文件 | 1 GiB |
| JSONL/CSV 规范化记录 | 2 MiB |
| Case ID | 1,024 字符 |
| 单条 CaseResult | 4 MiB |
| 元数据文件、worker 输出 | 8 MiB |
| ZIP JSONL 分块 | 16 MiB |
| ZIP 条目数 | 4,096 |
| ZIP 文件与累计解压量 | 4 GiB |

CSV 使用可重置读取预算约束多行字段的分配，再校验准确的逻辑记录大小。ZIP 大于旧 64 MiB 限制的验收使用真实分块路径，不通过把全量内存上限放大来实现。

## 可重复验收及清理

`make acceptance-large` 执行 10,000 Case × 100 指标，覆盖真实 Node Evaluator、Python Processor、精确统计、分块包、校验及双运行时不可用的分页查看器。`python3 scripts/large_acceptance.py --cases 100` 是快速同链路预检。

所有规模输入、journal、SQLite、执行快照和大结果包放在本次 `TemporaryDirectory`，成功/失败/终止时先停本轮进程再清理。只保留 `artifacts/phase5-large/acceptance-<规模>.json` 小型测量与摘要，记录 `large_data_removed`。异常退出如 SIGKILL/断电无法运行 finally，不能将这种情形声称为自动清理保证。

规模结果和浏览器记录见同一份 `phase5-progress.md`。有界处理不等于无限历史、长期运行 SLA 或跨平台发行通过；这些边界保持明确。

## 在线历史、调度与窗口

- journal 仍是 ACK 与恢复依据；每条记录同步后，事务更新可重建 SQLite 索引。历史正文、接收去重、单位的首次事件时间、最新 revision、评分状态和 judge 决策均从索引读取，不放进历史内存 map。
- pending 按持久化序号分批查询，最多 32 条且编码合计 8 MiB；暂停只停止新的评分。judge 预算按计划、窗口和完整评测身份索引查询。OTLP 组装在磁盘保存，按 quiet/timeout 到期时间分批读取。
- SQLite COMMIT 被读锁阻挡时，清理驱动遗留的底层事务；已落盘事件保持 ACK，后续操作重建投影。无法重建时返回安全不可用状态，不把缺失记录算作评分成功。启动仍需顺序重放 journal；内存有界不等于重启时间与历史无关。
- 报告在磁盘冻结查询范围内的记录及 latest 标记，再从同一快照生成窗口与 backlog。评分或迟到 revision 不污染正在生成的快照；已经物化的旧版本仍可查询与导出。每次复用现有 `processor/reduce`，不在 Go/React 补造统计。
- 范围接口不返回 CaseResult 或逐单位 facts；窗口详情返回统计和范围，样本使用与离线报告相同的分页摘要/单条证据接口。窗口输入、证据和事实在持久化前沿用脱敏入口。
- 范围缓存仅保留最近一个汇总；准备好的在线计划最多 128 个；Run journal 序号缓存最多 128 个，淘汰后读取最后一个有界 journal 记录。运行列表最多 1,024 个元数据项及 8 MiB，超限明确失败；指定 Run 查询不受列表条目限制影响。

| 在线边界 | 当前限制 / 行为 |
|---|---|
| 接收 | 单事件 1 MiB、单批 100；pending 沿用计划上限（默认 10,000） |
| 查询时间 | 最多 24 小时；最多 1,440 个窗口 |
| 范围内记录 | 最多 10,000 条，包含旧 revision；边缘对齐窗口的记录也计入预算 |
| 窗口事实 | 编码最多 4 MiB；范围事实最多 8 MiB |
| 范围结果 | 窗口元数据累计 4 MiB，整个响应最多 8 MiB |
| 查询超限 | `WINDOW_CAPACITY_EXCEEDED`，提示缩短范围；不截断分母、不显示假 0 |
| 停服清理 | 元数据预览：每类身份最多 50,000、合计 64 MiB；窗口/Run 列表也受各自限制 |

默认仍保留数据，清理必须停服、preview、apply 并留审计。pending/在途记录及受其保护的窗口不能删除。压缩按批读取正文，清理后保留去重/过期身份，既有结果包仍独立可用；这些身份与 journal/索引仍占磁盘，不承诺无限磁盘或在线无停机清理。

`make acceptance-online-capacity` 建立 10,000 条无评测器的历史占用，最近事件执行真实 Python+Node 指标，持续 60 秒输入；覆盖 ACK 后 SIGKILL/重启、暂停/恢复、queue full、duplicate、revision、exporter 断线、停服清理、导出和无语言运行时重开。历史占用与评分吞吐分开声明，不声称 10,000 条全部执行混合 worker。每 100 ms 采样 daemon 及其可观察后代 RSS；短寿命 worker 峰值可能漏采，不是操作系统精确峰值或生产 SLA。

脚本冻结二进制到专属临时目录；finally 先停止所属 daemon/viewer，再删除历史、证据、数据库和快照。保留小型 `artifacts/t12-online/acceptance.json`、`window.seven.eval`、`report.html`。`scripts/online_browser_fixture.py` 提供小型真实页面数据；截图保持忽略，不提交规模数据。

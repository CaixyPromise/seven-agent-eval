# 有界离线执行：协议 2.0

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

规模结果和浏览器记录见同一份 `phase5-progress.md`。离线有界处理不等于在线无限历史固定内存、长期运行 SLA 或跨平台发行通过；这些边界保持明确。

# 第二阶段：持续评分与窗口报告

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

## 验收记录

已在本机完成以下验收：

- `make test`：Go race 测试、server 测试、前端单元测试、锁定 Python 3.11 SDK 测试和 Vite 构建通过。
- `go vet ./...` 与 `python3 scripts/acceptance.py` 通过；后者重新验证了第一阶段 replay、HTTP 调用、脱敏与结果包独立打开。
- 受控事件源向 `examples/live` 的 EEP 接口注入 12 条完成事件：均在 journal ACK 后评分，窗口显示 12/12、100% 覆盖率、p95 131 ms；异常 Drawer 显示 4 条精确匹配失败样本。
- 浏览器在 1536×1024 与 390×844 下验证了实时 SSE 刷新、趋势图、窗口表、Drawer 和“导出窗口”操作；浏览器控制台无错误。截图保留在本机忽略目录 `output/playwright/live-desktop.png` 和 `output/playwright/live-mobile.png`。
- 从浏览器导出的 `artifacts/phase2/window.seven.eval` 经 `seven-eval verify` 校验；在独立的 `artifacts/phase2/viewer` 目录重新打开后，窗口范围、12 条样本、结论和三份码表仍可读取，不需要原 Package 或 Python worker。

本阶段交付提交会在所有变更核对后建立；没有创建远程仓库或推送。

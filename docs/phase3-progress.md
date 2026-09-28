# 第三阶段进度

基线：`f23e6d1`，分支：`codex/agent-rag-phase3`。本阶段本地提交，不推送。

## 范围与状态

- Checkpoint 1（已提交 `a20939b`）：Source/Mapping、JSONL HTTP RAG、确定性指标、页面及独立结果包。
- Checkpoint 2（已提交 `6bbe982`）：CSV/TREC/BEIR、可选 HTTP judge。
- Checkpoint 3（本次最终交付提交）：Seven Workspace SSE、历史导入、Episode、页面复核与统一回归。

状态：本轮计划的受控交付完成；真实 Seven 环境调用和真实模型评分质量待提供环境后验收，不以本机 fixture 替代。

共享 schema、Subject lifecycle 和执行核心在同一次连续实施中完成；checkpoint 是可演示交付节点，不声称各节点的公共代码彼此独立。本轮用户确认先完成受控验收，真实 Seven 环境稍后提供。

第一、二阶段本机离线与受控持续评分已在基线验收。真实生产接入、真实 judge 评分质量、容量与安装发行不在该证据范围。

## 接口检查

当前 Seven 路由来源：`seven-agent-server/internal/management/workspace/handler/routes.go` 与 `internal/orchestrator/chat/handler/routes.go`。会话创建 POST `/workspaces/:workspaceId/sessions`；对话 POST `/workspaces/:workspaceId/sessions/:sessionId/conversation/stream`，挂载前缀由部署配置决定，不能硬编码只使用 `/v1`。终止事件为 `conversation.completed`。

## 验证记录

2026-09-28，从当前源码验证：

- `make test`：Go race（core/server）、React 9 项领域测试、Python 19 项测试、TypeScript 与 Vite 构建通过，退出 0。
- `go vet ./...`、`make build`、`git diff --check`：通过。
- `python3 scripts/acceptance.py`：原离线 replay / HTTP / 参考不泄漏 / 结果包回归通过。
- `uv run --project sdk/python python scripts/live_acceptance.py`：ACK 后 SIGKILL、恢复、去重、预算、revision、exporter 断线和窗口重算通过。
- `uv run --project sdk/python python scripts/phase3_acceptance.py`：真实回环 HTTP，移动 Package、5 个 Case、K=1/5 分开、缺参考不变零、删除源目录后无 Python 校验通过。
- `uv run --project sdk/python python scripts/seven_acceptance.py`：2 个会话、3 次调用、4 个计划 Turns；失败后不调用后续 Turn，生成 partial 包，源目录删除后无 Python 校验通过。
- `TestAllFormatFixturesNormalizeEqually`：JSONL/CSV/BEIR/TREC 的 Case 与确定性指标规范化摘要一致。

`make acceptance-rag acceptance-seven acceptance-live` 统一入口再次通过，退出 0。历史样例 `./bin/seven-eval --package examples/seven-history run history` 完成 1/1，退出 0。有意失败的 Episode 样例为 3/4 partial，run CLI 退出 1 是预期状态；其 `verify` 退出 0。

## 浏览器与可迁移验收

独立 Chromium、真实 Go server 和构建资源；1536×1024、DPR 1、100% 缩放。另验证 390×844 和 `prefers-reduced-motion: reduce`，页面 scrollWidth 为 390。

- 发现页启动真实 HTTP RAG 计划，5/5；指标按 K 区分，检索排名显示 qrels 等级，引用可下钻。
- 发现页 Source/Mapping 筛选与摘要；修复共享内置实现路径导致的表格行标识冲突，增加领域测试并复测筛选无残留。
- Episode 页面显示 2/2 完成与 1/2 部分结果，后续 Turn 明确未执行；Enter 可打开第一轮工具证据，Escape 可关闭。
- 浏览器实际下载 `browser-rag.seven.eval`（Run `20260928T051654-ee6f85f87ff3bbf3`）与 `browser-episode.seven.eval`（Run `20260928T051655-39677173914f1bf8`），两者 verify 退出 0。
- 新目录 `artifacts/phase3/final-browser-viewer`、`SEVEN_EVAL_PYTHON=/does-not-exist`：CLI 打开 RAG 包；通过页面导入 Episode 包，查看 Turn 与工具证据。无需原 Package、SQLite 或用户处理器代码。
- 分别比较两个包的 `run.json` 与独立查看器 `GET /api/runs/{id}`，经 `jq -S` 规范化后 `diff` 退出 0；包含指标、结论、码表、有效配置、数据版本和 Episode。
- 页面文案与局部视觉复核见 `docs/design/phase3-review.md`，不宣称全页原型像素级一致，也未重复整个工作台的 200% zoom 或全面无障碍审计。

## 产物与复现

- `artifacts/phase3/acceptance.json`、`seven-acceptance.json`：受控范围与断言结果。
- `artifacts/phase3/browser-rag.seven.eval`、`browser-episode.seven.eval`：浏览器下载包。
- `artifacts/phase3/episode.seven.eval`：可重复脚本生成的 partial 包。
- `artifacts/phase3/report.html`：自包含 RAG 报告。
- `output/playwright/phase3/`：RAG 桌面、检索/引用 Drawer、390px、Episode 导航、工具证据、独立重开截图。
- 运行命令及 YAML/JSONL 约定见 `docs/phase3-usage.md`。示例 `rag-http`、`rag-formats`、`seven-fixture`、`seven-history` 均无凭据。

产物、缓存与运行数据库保持忽略；提交包含生成脚本与准确记录，不提交二进制截图。父仓库任务拆分文档只更新本项目状态，其未跟踪文件保持在父仓库，未混入子仓库提交。本轮仅关闭自己启动的验收服务，不影响原有 7431 工作台。

## 接口真实性与后续边界

- Seven 创建会话使用当前服务端 `scopeType: private`；Episode 内串行共享，Episode 间隔离，失败/超时停止当前 Episode 剩余 Turns，取消保留部分结果。不执行旧 runner 环境重置、不自动重试副作用请求。
- 当前 SSE 支持 started/patch/completed/failed/cancelled/paused、message replace/append、工具 Part；未知事件只保留可审计类型，不猜测成功。最后一个 message Part 作为回答；超时保留部分回答。
- 未确认通用 Retrieval/Citation Part 时保持缺失；context-window usedTokens 不冒充计费 tokens。真实环境需要核对可用证据。
- 当前公开路由未确认逐会话归档，native Seven 不提供自动清理，不用删除或全局归档替代；Python Subject 可显式提供 create/cleanup，验收只清理本轮返回的 session。
- 历史导入按旧 runner 实际 `tool_result_json`、`tool_arguments_parsed_json`、duration_ms 归一化；`ok/success/completed` 映射为工具完成，旧 checker 保留审计，不直接转成新指标。受控历史格式测试通过，真实旧产物差分尚未运行。
- judge 为显式启用的受控 HTTP bridge；成功、非法结构、HTTP 错误、超时和未配置分别验收。模型、版本化 prompt、配置摘要和参数保持身份；本轮不声明真实模型评价准确性。
- T09 本轮仅 JSONL/CSV/TREC/BEIR；Parquet/HF、OpenAI Evals/Ragas、正式生产来源、T10 参数比较、TS SDK、T12 容量/保留、T13 跨平台发行、T14 真实端到端继续后续。
- 非阻断风险：Vite 主 chunk 约 843 KiB，Live chunk 约 559 KiB，保留 500 KiB 警告；本轮未扩展分包重构。

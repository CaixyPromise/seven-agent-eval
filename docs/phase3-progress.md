# 第三阶段进度

基线：`f23e6d1`，分支：`codex/agent-rag-phase3`。本阶段本地提交，不推送。

## 范围与状态

- Checkpoint 1（已验证，准备提交）：Source/Mapping、JSONL HTTP RAG、确定性指标、页面及独立结果包。
- Checkpoint 2（自动验收通过，准备归档）：CSV/TREC/BEIR、可选 HTTP judge。
- Checkpoint 3（受控协议与页面流程通过，视觉归档中）：Seven Workspace SSE、历史导入、Episode。

共享 schema、Subject lifecycle 和执行核心在同一次连续实施中完成；checkpoint 是可演示交付节点，不声称各节点的公共代码彼此独立。本轮用户确认先完成受控验收，真实 Seven 环境稍后提供。

第一、二阶段本机离线与受控持续评分已在基线验收。真实生产接入、真实 judge 评分质量、容量与安装发行不在该证据范围。

## 接口检查

当前 Seven 路由来源：`seven-agent-server/internal/management/workspace/handler/routes.go` 与 `internal/orchestrator/chat/handler/routes.go`。会话创建 POST `/workspaces/:workspaceId/sessions`；对话 POST `/workspaces/:workspaceId/sessions/:sessionId/conversation/stream`，挂载前缀由部署配置决定，不能硬编码只使用 `/v1`。终止事件为 `conversation.completed`。

## 验证记录

2026-09-28，从当前源码验证：

- `make test`：Go race（core/server）、React 8 项领域测试、Python 18 项测试、TypeScript 与 Vite 构建通过。
- `go vet ./...`、`make build`、`git diff --check`：通过。
- `python3 scripts/acceptance.py`：原离线 replay / HTTP / 参考不泄漏 / 结果包回归通过。
- `uv run --project sdk/python python scripts/live_acceptance.py`：ACK 后 SIGKILL、恢复、去重、预算、revision、exporter 断线和窗口重算通过。
- `uv run --project sdk/python python scripts/phase3_acceptance.py`：真实回环 HTTP，移动 Package、5 个 Case、K=1/5 分开、缺参考不变零、删除源目录后无 Python 校验通过。
- `uv run --project sdk/python python scripts/seven_acceptance.py`：2 个会话、3 次调用、4 个计划 Turns；失败后不调用后续 Turn，生成 partial 包，源目录删除后无 Python 校验通过。
- `TestAllFormatFixturesNormalizeEqually`：JSONL/CSV/BEIR/TREC 的 Case 与确定性指标规范化摘要一致。

浏览器使用独立 Chromium、真实 Go server 和构建资源。已通过发现页启动 RAG、检索/引用下钻、浏览器导出、独立目录无 Python 查看器重开、Episode Turn 导航；正在归档截图和最终比较记录。结果包与截图在忽略的 `artifacts/phase3/`、`output/playwright/phase3/`，不进入源码提交。

# v0.1.0-alpha.1 Alpha 发布说明

版本采用 GitHub prerelease 发布，见 [Release 页面](https://github.com/CaixyPromise/seven-agent-eval/releases/tag/v0.1.0-alpha.1)。
SDK 当前版本为 0.1.0；尚未发布 SDK 注册表包。
许可证：Apache-2.0，Copyright 2026 CaixyPromise，见根 LICENSE 和 NOTICE。
已确认公开范围、保留两张原型图和历史作者与邮箱，清单见 [public-scope.md](public-scope.md)。
仓库确定为 `CaixyPromise/seven-agent-eval`，gh 登录与账户 API 检查成功。
GitHub private vulnerability reporting 已启用，并经 API 确认。

## 本次提供

- 离线 HTTP/Python/TS Subject 和历史回答评测；可替换 Evaluator、Processor、Conclusion。
- JSONL、CSV、TREC/BEIR 输入，RAG 确定性指标及显式启用的 judge bridge。
- 生产完成事件持续评分、窗口报告、显式候选参数比较。
- 本地 Ant Design/ECharts 工作台，2.0 格式 `.seven.eval` 可携带结果。
- Python 与 TS/JS 扩展、安装目录运行，以及无 Python/Node 的结果查看器。
- 随仓库和安装包提供 `seven-eval-sdk` Skill，指导 AI 助手开发、验证和交付扩展。

## 安装与边界

安装步骤见 [installation.md](installation.md)，自建示例见根 README。
本轮最终产物验收以 macOS arm64 为准；历史 Linux arm64 容器记录见第五阶段文档。
其他平台、签名、公证和自动更新不在本次验证声明中。
结果格式采用 2.0，旧格式不提供兼容导入。

工具服务于可信本地使用；用户扩展不是沙箱，不将服务直接暴露公网。
真实 Seven、真实模型评分质量和生产 T14 验收仍待完成。
运行代码的 Linux/macOS 完整 CI 已通过；最后的发布状态更新为文档改动，
tag 对应提交的检查见仓库 Actions。Release 附 macOS arm64 安装包及 SHA-256。

## 发布候选证据

最新 Logo 安装产物验收见 `artifacts/brand-checkpoint/`；本轮文档/Skill 候选验收见
`artifacts/alpha1-docs/`（本地产物，不提交生成数据）。
源代码、许可证、SDK 元数据和本文纳入本地 checkpoint。
最终命令、退出码及历史审查见 [open-source-readiness.md](open-source-readiness.md)。

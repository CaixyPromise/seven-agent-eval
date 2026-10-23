# 0.1.0-alpha.1 首发候选说明

状态：本地候选，尚未发布。SDK 当前版本为 0.1.0；候选发行名称不表示已经发布 SDK。
许可证：Apache-2.0，Copyright 2026 CaixyPromise，见根 LICENSE 和 NOTICE。
已获准保留历史作者与邮箱。材料公开权利及安全反馈信息待确认，
清单见 [public-scope.md](public-scope.md)。拟使用 gh 本机配置的 CaixyPromise 账户，
其令牌当前无效；公开仓库尚未创建。

## 本次提供

- 离线 HTTP/Python/TS Subject 和历史回答评测；可替换 Evaluator、Processor、Conclusion。
- JSONL、CSV、TREC/BEIR 输入，RAG 确定性指标及显式启用的 judge bridge。
- 生产完成事件持续评分、窗口报告、显式候选参数比较。
- 本地 Ant Design/ECharts 工作台，2.0 格式 `.seven.eval` 可携带结果。
- Python 与 TS/JS 扩展、安装目录运行，以及无 Python/Node 的结果查看器。

## 安装与边界

安装步骤见 [installation.md](installation.md)，自建示例见根 README。
本轮最终产物验收以 macOS arm64 为准；历史 Linux arm64 容器记录见第五阶段文档。
其他平台、签名、公证和自动更新不在本次验证声明中。
结果格式采用 2.0，旧格式不提供兼容导入。

工具服务于可信本地使用；用户扩展不是沙箱，不将服务直接暴露公网。
真实 Seven、真实模型评分质量和生产 T14 验收仍待完成。
CI 已配置，GitHub 托管执行待仓库就绪后验证；公开下载地址尚未建立。

## 发布候选证据

安装产物与重开验收见 `artifacts/apache-release/`（本地产物，不提交生成数据）。
源代码、许可证、SDK 元数据和本文纳入本地 checkpoint。
最终命令、退出码及历史审查见 [open-source-readiness.md](open-source-readiness.md)。

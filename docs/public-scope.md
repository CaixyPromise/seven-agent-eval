# 首发公开范围审阅

2026-10-24：Apache-2.0 和署名 CaixyPromise 已确认，允许保留历史作者与邮箱。
以下是拟公开范围；未执行公开。请所有者确认材料权利，或指出需要排除的路径。

| 内容 | 路径 | 用途与需确认事项 |
|---|---|---|
| 产品实现 | `cmd/`、`internal/`、`builtin/`、`web/` | CLI、执行与实时核心、默认配方及 UI；确认拥有公开授权 |
| 扩展与协议 | `sdk/`、`protocol/` | Python/TS SDK、schemas、合成 conformance；保留第三方各自许可证 |
| 示例与测试 | `examples/`、`testdata/`、测试源码 | 合成问答、工具与 SSE/OTLP fixtures；确认不含真实客户数据 |
| 文档与开发入口 | `docs/`、`scripts/`、根文档、Makefile、CI、锁文件 | 用法、设计与历史验收记录；包含受控验证的性能记录 |
| 两张设计图 | `docs/design/blue-workspace-prototype.png`、`docs/design/live-workspace-prototype.png` | 原型而非实际产品截图；确认图像来源与可发布权利 |
| Git 历史 | 当前本地分支可达提交 | 保留作者/邮箱；包含已删除的旧合成结果包，详情见下文 |

用户运行状态、venv、node_modules、凭据、下载产物、百万测试数据和安装验收生成包不纳入当前树。

## 历史检查

本轮修改前 15 个提交，作者为 CaixyPromise 的既有 Git 身份（历史大小写保持不变）。
Gitleaks 全历史扫描无命中；历史 ZIP 另行展开扫描无命中。
逐个检查 528 个可达 blob，未命中个人绝对路径；可疑生成/私有路径只有下述旧结果包。
历史仍有 `artifacts/phase2/window.seven.eval`：11,750 字节、6 个成员、合成问答，
当前树已删除。保留历史意味着该文件仍可从旧提交读取；未重写历史。

`git fsck --full` 无损坏，但存在不可达 blob/tree（本地暂存/验收树等对象）。
它们不属于分支可达历史；常规 Git 推送不传输这些不可达对象。
本轮不执行垃圾回收，也不删除本地对象。文本扫描不证明任意隐私或权属问题不存在。

## 账户与反馈

本机 gh 配置账户 CaixyPromise，当前认证无效（API 401）。后续可由所有者重新登录；
本轮不创建仓库或推送。仓库名建议 `seven-agent-eval`，实际远程地址待创建后验证。
私密漏洞渠道待所有者指定邮箱，或仓库创建后启用并验证 private vulnerability reporting。
现有提交邮箱不自动充当安全联系地址。

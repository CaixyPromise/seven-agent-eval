# 已确认的首发公开范围

2026-10-24：Apache-2.0 和署名 CaixyPromise 已确认，允许保留历史作者与邮箱。
所有者已确认下列公开范围，两张原型图保留；尚未执行公开。

| 内容 | 路径 | 用途与范围 |
|---|---|---|
| 产品实现 | `cmd/`、`internal/`、`builtin/`、`web/` | CLI、执行与实时核心、默认配方及 UI；已获准公开 |
| 扩展与协议 | `sdk/`、`protocol/` | Python/TS SDK、schemas、合成 conformance；保留第三方各自许可证 |
| 示例与测试 | `examples/`、`testdata/`、测试源码 | 合成问答、工具与 SSE/OTLP fixtures；按已确认范围公开 |
| 文档与开发入口 | `docs/`、`scripts/`、`skills/`、根文档、Makefile、CI、锁文件 | 用法、SDK 开发 Skill、设计与历史验收记录 |
| 两张设计图 | `docs/design/blue-workspace-prototype.png`、`docs/design/live-workspace-prototype.png` | 所有者确认保留；原型而非实际产品截图 |
| 品牌 SVG | `web/src/assets/seven-agent-symbol.svg`、`web/public/favicon.svg` | 按所有者要求接入品牌库 v1.1.0；保留图形与配色，不授予商标使用权 |
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

2026-10-24 重新检查 gh 登录成功，API 返回账户 CaixyPromise。目标仓库已确定为
`CaixyPromise/seven-agent-eval`；当日查询该仓库返回 404，本地未配置 remote。
此前 API 401 是历史状态，现已恢复。当前只进行本地准备。
私密漏洞渠道已选 GitHub private vulnerability reporting；仓库创建后需启用并验证，
尚未宣称渠道可用。现有提交邮箱不充当安全联系地址。

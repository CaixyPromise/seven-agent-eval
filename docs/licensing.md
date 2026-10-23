# 许可证与公开决定

项目所有者已于 2026-10-24 选择 **Apache-2.0**。根目录 [LICENSE](../LICENSE)
保存官方英文全文；Python、TypeScript SDK 和 Web 元数据使用 SPDX 标识 Apache-2.0。
公开发布尚未执行。以下对比保留选择依据。

| 选择 | 适用目标 | 取舍 |
|---|---|---|
| Apache-2.0（建议） | 企业集成、商业使用、扩展生态和明确专利授权 | 允许闭源衍生和商业服务；遵守许可证及适用通知要求 |
| MIT | 极简、宽松再使用 | 没有 Apache-2.0 那样明确单列的专利授权条款；不要求公开修改 |
| AGPL-3.0 | 希望修改后的网络服务承担相应源码提供义务 | 增加部分企业的采用审查成本；需核对组合和分发方式 |

本项目的平台无关内核、Python/TS 扩展和可携带结果适合 Apache-2.0 的集成方向。
它不会阻止竞品提供 SaaS，也不会自动取得外来代码或依赖的权利。
权威文本：[Apache-2.0](https://www.apache.org/licenses/LICENSE-2.0)、
[MIT](https://opensource.org/license/mit)、[AGPL-3.0](https://www.gnu.org/licenses/agpl-3.0.html)。
这是选型建议，不是法律兼容性认证。

已确认署名 **CaixyPromise**，允许保留 Git 历史中的作者姓名与邮箱。
所有者已确认代码、文档、示例与两张原型图的公开范围，见 [公开清单](public-scope.md)。
两张原型图保留为设计过程材料；第三方依赖仍遵循各自许可证。

发行包生成 `third-party/`，保留 Go/npm/Python 元数据及找到的 LICENSE/NOTICE
全文，见 [第三方通知](../THIRD_PARTY_NOTICES.md)。清单辅助审查，不等于法律结论。
根 LICENSE 已加入，并由打包入口带入；第三方材料继续遵循各自许可证。
根 NOTICE 使用已确认的 Copyright 2026 CaixyPromise；不重写既有作者归属。

目标仓库为 `CaixyPromise/seven-agent-eval`，漏洞反馈选 GitHub private vulnerability
reporting。2026-10-24 重新检查 gh/API 登录成功；旧 401 记录为历史状态。
渠道需在仓库创建后启用验证，当前没有创建仓库或推送，候选为 `v0.1.0-alpha.1`。

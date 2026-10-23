# 许可证与待确认事项

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
代码/文档/原型图公开范围供所有者审阅，见 [公开清单](public-scope.md)。两张 `docs/design/*prototype.png` 是设计过程保留图像，
应单独确认来源与可发布权利；依赖许可证不覆盖它们。

发行包生成 `third-party/`，保留 Go/npm/Python 元数据及找到的 LICENSE/NOTICE
全文，见 [第三方通知](../THIRD_PARTY_NOTICES.md)。清单辅助审查，不等于法律结论。
根 LICENSE 已加入，并由打包入口带入；第三方材料继续遵循各自许可证。
根 NOTICE 使用已确认的 Copyright 2026 CaixyPromise；不重写既有作者归属。

公开前请一次确认：

1. 当前代码、文档、合成示例及两张原型图的公开权利，按公开清单审阅。
2. 私密漏洞反馈渠道（邮箱或未来启用 GitHub private vulnerability reporting）。

拟使用本机 gh 配置的 CaixyPromise 账户；2026-10-24 检查令牌无效、API 返回 401，
未创建仓库。账户信息来自本机配置，不代表本轮已验证远程身份。

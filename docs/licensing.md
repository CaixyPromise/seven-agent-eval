# 许可证建议与待确认事项

**建议 Apache-2.0；目前尚未选定或授予项目许可证。** 本轮不创建冒充已批准的 LICENSE，
不将 package metadata 改成已授权状态，也不公开仓库。

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

公开前由权利人确认许可证及署名主体、代码/文档/原型图公开授权，以及提交作者邮箱
随 Git 历史公开是否符合意愿。两张 `docs/design/*prototype.png` 是设计过程保留图像，
应单独确认来源与可发布权利；依赖许可证不覆盖它们。

发行包生成 `third-party/`，保留 Go/npm/Python 元数据及找到的 LICENSE/NOTICE
全文，见 [第三方通知](../THIRD_PARTY_NOTICES.md)。清单辅助审查，不等于法律结论。
许可证获批后加入根 LICENSE、必要的 NOTICE，并由打包入口带入。

# 使用与开发文档

当前为 Alpha 候选，结果格式 **2.0**，不兼容 1.0 包或旧运行状态。

| 目标 | 入口 |
|---|---|
| 安装、默认报告、卸载、独立查看器 | [安装说明](installation.md) |
| 数据与 RAG、Seven SSE 和历史导入 | [数据与评测配方](phase3-usage.md) |
| 生产完成事件、窗口与旁路边界 | [持续评分](continuous-scoring.md) |
| 指定候选参数并配对比较 | [参数比较](parameter-comparison.md) |
| Python 扩展 | [Python SDK](python-extensions.md) |
| TypeScript/JavaScript 扩展 | [TS/JS SDK](typescript-extensions.md) |
| 分批 Processor、大小与查询上限、结果包 | [当前执行契约](bounded-evaluation.md) |
| 结构定义 | [JSON Schema](../protocol/schemas/exchange.json) |
| 贡献、测试与本地构建 | [贡献指南](../CONTRIBUTING.md) |
| 安全边界及漏洞报告状态 | [安全说明](../SECURITY.md) |
| 版本范围和未完成项目 | [变更记录](../CHANGELOG.md) |
| 首发候选与公开清单 | [首发说明](release-alpha.md)、[公开范围](public-scope.md) |

指标由 Evaluator 产出，统计由 Processor 计算，结论由 Conclusion 扩展提供。
覆盖率不是业务正确率；缺失、跳过、预算耗尽、不可计算与错误不得转成零分。
省略阶段列表继承预设，显式列表替换，`[]` 关闭；不使用旧统计兜底。

`phase*-progress.md`、`phase1-contract.md`、`verification.md` 和 `design/*review.md`
是历史实施记录，不代表全部当前状态。首发验证见 [开源准备记录](open-source-readiness.md)。
真实 Seven/模型/生产 T14 尚未验收。

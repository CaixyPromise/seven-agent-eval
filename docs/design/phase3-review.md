# 第三阶段页面与文案复核

## F1 图表局部复核（2026-09-28）

沿现有批准的蓝色 Ant Design 工作台增加局部图表，不重新生成全屏原型。标题承载“随 K 对照”目的，卡片说明承载横轴、macro 均值与缺失语义；精确值和有效分母在图表下方表格。配置仅在展开区展示。无有效观测时原位提示查看参考与检索证据，不伪造 0% 曲线。

| Before | After | Why |
| --- | --- | --- |
| RAG 只有 Precision/Recall 数值表，缺 F1 | 逐样本 F1、Processor macro 均值，三项同图及精确值表 | 直接对照召回与精确率的权衡，避免对平均值再算 F1 的错误 |
| 图表缺少明确横轴语义 | 标题“检索质量随 K 的变化”，K 按数值排序 | 不把截断对照冒称为生产时间或参数优化实验 |
| 初次渲染图例靠近横轴标签 | 固定图例在上方，390px 再截图 | 防止图例遮挡 K；精确表格内部横向滚动保留 F1 |
| Live 独有 ECharts 封装 | 共用 EvaluationChart，报告图表 lazy 加载 | 复用 reduced-motion、resize 与 dispose 逻辑，避免复制组件与扩大入口 |
| 不可计算值可能被误读为零 | null 不连线、全缺失显示 Empty、表格用“—” | 区分真正零分与证据不足，保留各指标分母 |

真实独立查看器使用新结果包，无可用 Python；Chromium 1536×1024 与 390×844，reduced-motion 实际媒体状态为 reduce。局部默认、窄屏、全不可计算（临时拦截后恢复）均读取文案并截图复核；表格提供图像的精确数据替代。证据在 `output/playwright/rag-f1/`。未重新验收全工作台 200% 缩放、全部键盘路径或仪器化动画/泄漏测试。

2026-09-28。复用既有蓝色 Ant Design 工作台，不重新设计全站。新增局部领域组件 `RetrievalEvidence`、`EpisodeTable`，保留发现、报告和样本 Drawer 的原入口。使用 frontend-product-design 的事实文案与渲染复核、React best practices 的直接组件导入和派生状态约束。

## Copy gate

| 用户事实 | 承载位置 | 生命周期 | 文案 / 下一步 |
| --- | --- | --- | --- |
| Source 与旧 dataset 是不同输入方式 | 发现筛选、计划摘要 | 发现后、执行前 | “数据源”“字段映射”；计划显示实际 Source ID |
| 不同 K 的指标不可混合 | 指标标题与配置展开 | 每次查看指标 | `@1` / `@5`，展开 Evaluator 与 parameters |
| 检索分数、参考等级可能缺失 | 检索排名表 | 样本 Drawer 打开时 | 缺失显示 `—`，不补零或伪造等级 |
| 引用定位未提供时不能判定范围合法 | 逐项指标 | 评分与报告 | “证据不足”；有定位才检查范围 |
| 调用完成不等于任务效果 | Agent 结论、独立指标 | Agent 报告存在时 | 分开显示调用完成、工具执行、已声明效果匹配 |
| Episode 可能停止后续轮次 | Episode 导航、partial 提示 | 部分结果存在时 | “已处理 / 计划轮次”“未执行”，只允许打开真实结果 |
| 评分覆盖率不是业务正确率 | 原有 KPI 说明 | 报告查看时 | 保留原有单一说明，不在每个 RAG 卡片重复 |

## Before | After | Why

| Before | After | Why |
| --- | --- | --- |
| 发现页仅列出 dataset / adapter 等资产 | 增加 Source、Mapping 筛选，Source 计划显示真实逻辑引用 | 不把已经接入的数据源误报为“未指定数据集” |
| 检索上下文只以通用 JSON 折叠展示 | 有序表格展示排名、文档 ID、检索分数、qrels 等级和文本 | 错误排名样例可直接对照参考，定位具体样本 |
| 相同指标标题不含 K，数值一位小数 | 标题显示 `@K`，RAG 均值保留四位小数；完整配置仍可展开 | 防止截断参数混淆和过度舍入掩盖差异 |
| Episode 摘要是原始 JSON | Ant Design 表格按序提供 Turn 按钮，未执行 Turn 为文字标签 | 能跳转已保留证据，并明确区别没有执行与执行失败 |
| 样本仅展示回答和 contexts | 增加检索、工具、引用定位与 Episode/Turn 身份 | 数据沿原样本入口下钻，不增加独立页面或重复状态 |
| 结果包只有通用字段视图 | 独立查看器沿同一组件展示新增证据，HTML 也保留结构与 Episode | 换目录、缺 Python 或源代码时仍可审计 |
| RAG 和 judge 共用实现路径，表格行标识冲突，筛选后有残留行 | 行标识包含类型、来源、ID、版本和相对路径，并增加领域测试 | 同一实现路径不等于同一资产；列表与筛选计数必须一致 |

## 实际渲染证据

- 独立 Chromium、1536×1024、DPR 1、100% 缩放：发现 → RAG 运行 → 报告 → wrong-rank 检索/引用 Drawer → 浏览器下载。
- 同一 RAG 样本在 390×844、reduced-motion: reduce 下复核；document scrollWidth 为 390。检索表保留横向滚动，未静默删除参考列。
- Enter 打开 Episode 的 first Turn，Escape 关闭 Drawer；失败 Episode 显示 1/2 和后续 Turn 未执行。
- 浏览器导出包在新查看目录、无可用 Python 路径下重新打开；规范化 Run JSON 与包内数据完全相同。
- 证据：`output/playwright/phase3/rag-report-desktop.png`、`rag-evidence-desktop.png`、`rag-evidence-mobile.png`、`episode-navigation.png`、`episode-tool-evidence.png`、`reopened-rag-desktop.png`。

本阶段没有生成新的全页面原型，也不声称与第二阶段静态原型同数据或像素级一致。复核范围为本次局部增量。未重做整个工作台的真实 200% zoom、全部错误注入、ECharts 生命周期或全面无障碍审计；这些不能从本次窄屏截图推断通过。Copy gate 与局部视觉复核通过，证据保留在忽略目录。

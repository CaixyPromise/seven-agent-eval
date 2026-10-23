# 用 AI 助手开发评测扩展

随仓库和安装包提供一个 Skill：`skills/seven-eval-sdk/`。
它指导 AI 找到当前工具与 Evaluation Package，选择扩展类型，生成代码、运行合成 fixture，
检查统计口径和 KPI，再导出可验证结果。Skill 不是后台代理，也不会自动执行生产请求。

## 使用方式

最直接的方式是让助手读取当前工具目录下的 `skills/seven-eval-sdk/SKILL.md`。
也可以把整个 `seven-eval-sdk` 文件夹复制到你的 AI 工具支持的项目级 Skill 目录。
例如支持 `.agents/skills/` 的工具可使用：

```sh
mkdir -p /path/to/your-project/.agents/skills
cp -R /path/to/seven-agent-eval/skills/seven-eval-sdk /path/to/your-project/.agents/skills/
```

按工具的发现规则重新加载。复制整个文件夹，保留 references 和 assets；不要只复制 SKILL.md。
不同 AI 工具的 Skill 安装方式不同，不宣称全部自动兼容。该项目不自动写入全局助手配置。
Skill 独立复制后仍需找到已安装的 Seven Eval；它不附带运行时或完整 SDK。

示例请求：

> 使用 seven-eval-sdk Skill，为我的已有回答数据创建一个独立 Package。用 Python Processor
> 统计已评分量、评分覆盖率和异常量，显示首页 KPI。先用合成数据跑通，导出报告并给出验证命令。

> 使用 seven-eval-sdk Skill，把同一统计实现为 TypeScript。保持 null/false/0 与错误状态，
> 比较 Python 和 TS 的 Processor 输出，并在查看器里检查报告。

## Skill 包含什么

- 主入口：开发步骤、四类扩展选择、发现/运行/导出流程、交付要求。
- `references/sdk-contract.md`：跨语言上下文、session、指标身份、分批状态、KPI 与错误语义。
- `assets/team_summary.py`、`assets/team_summary.ts`：等价的有界 Processor 模板。

模板统计至少有一个 observed 指标的 Case 为已评分，显式 false 规则或错误为异常；
同一个 Case 可以同时已评分且异常。coverage 不是业务正确率。数值零不会自动成为异常。
复制其中一种到 `processors/`，设置 `processors: [processor:team-summary]`。
两种模板具有相同 ID，不能同时注册到同一 Package。

先从 replay 数据验证新扩展；接入真实目标或 judge 时使用已授权环境和预算。
更多权威字段见 [Schema](../protocol/schemas/exchange.json)、[Python SDK](python-extensions.md)
和 [TS/JS SDK](typescript-extensions.md)。协议版本为 2.0，不使用旧整份 Run Processor API。

## 验证与声明边界

Skill 的两个模板通过真实 Go 调度、Python/Node worker 在临时 Package 执行，并比较输出；
安装验收会检查 Skill 文件随包分发。验证记录见 [首发准备](open-source-readiness.md)。
这验证了所附开发路径与模板，不等于所有 AI 工具已经自动发现或成功调用该 Skill。
SDK 当前不在 PyPI/npm 发布；安装与源码准备方式见 [安装说明](installation.md)。

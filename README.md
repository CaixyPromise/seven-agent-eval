# Seven Agent Eval

面向 Agent 与 RAG 的评测工具：把数据、目标调用、证据、指标、统计和结论组织成可复现的评测计划，
在本地工作台查看结果，再导出一份可以带走的 `.seven.eval` 报告。

采用 **Go 执行核心 + Python／TypeScript 扩展 + React 工作台**。平台无关，
Seven Agent 通过适配器接入；其他 HTTP Agent、RAG 服务和已有回答也可评测。

**当前版本：[v0.1.0-alpha.1](https://github.com/CaixyPromise/seven-agent-eval/releases/tag/v0.1.0-alpha.1)，Alpha 预发布。**
仓库为 [CaixyPromise/seven-agent-eval](https://github.com/CaixyPromise/seven-agent-eval)，许可证 [Apache-2.0](LICENSE)，
Copyright 2026 CaixyPromise。GitHub private vulnerability reporting 已启用。
SDK 当前版本为 0.1.0，尚未发布到 PyPI／npm。

## 你可以用它做什么

| 使用场景 | 当前能力 | 入口 |
|---|---|---|
| 离线评测 | 调用 HTTP/Python/TS Subject，或评分已有回答；默认报告开箱可用 | [快速开始](#快速开始) |
| RAG 评测 | JSONL/CSV/TREC/BEIR；Precision、Recall、F1、MRR、NDCG、MAP；引用检查和可选 judge | [RAG 指南](docs/phase3-usage.md) |
| 持续评分 | 接收已完成请求的 EEP／OTLP JSON 事件；去重、证据修订、采样、预算和窗口报告 | [持续评分](docs/continuous-scoring.md) |
| 参数比较 | 冻结同一数据集，运行明确候选配置，配对比较质量、时延和可用成本 | [比较指南](docs/parameter-comparison.md) |
| 自定义评测 | Python／TS／JS Subject、Evaluator、Processor、Conclusion | [Python SDK](docs/python-extensions.md)、[TS/JS SDK](docs/typescript-extensions.md) |
| AI 辅助开发 | 用内置 Skill 指导 AI 编写扩展、发现计划、执行验证和导出 | [SDK 开发 Skill](skills/seven-eval-sdk/SKILL.md) |
| 分享结果 | 单文件 `.seven.eval`；无原项目、Python、Node 也能用查看器重开 | [安装与查看器](docs/installation.md) |

评测 K 与目标检索 top_k 是两件事：前者对已取得的排名计算不同截断指标，后者改变目标参数并重新调用。
基础报告的评分覆盖率表示有可用评分的比例，不代表业务正确率。
缺失、不可计算、未采样、预算耗尽和执行错误都有独立状态，不会补成零分。

## 快速开始

源码开发环境：Go 1.25、uv 管理的 Python 3.11、Node >=20.19、pnpm 10.24.0。
在仓库根目录执行：

```sh
make setup
make build
./bin/seven-eval --package examples/history discover --json
./bin/seven-eval --package examples/history plan inspect smoke
./bin/seven-eval --package examples/history run smoke
./bin/seven-eval --package examples/history serve
```

打开 `http://127.0.0.1:7431`。从评测计划进入运行，查看样本证据、指标、处理器统计与结论，然后导出结果。
示例使用合成数据，无需模型凭据。每个工作区只允许一个协调器；server 运行期间从页面启动任务，
停止 server 后再在同一工作区执行 CLI run。

安装包用户无需 Go、uv、pnpm 或 npm；执行评测需要 Python 3.11，使用 TS/JS 扩展时需要 Node。
环境在最终安装目录创建，不复制开发 `.venv`。完整步骤见 [安装说明](docs/installation.md)。
macOS arm64 用户可从 [Release 下载安装包与 SHA-256](https://github.com/CaixyPromise/seven-agent-eval/releases/tag/v0.1.0-alpha.1)，
校验后按 [安装说明](docs/installation.md) 安装；其他环境可从源码构建。

### 调用一个本地 HTTP 目标

终端一：

```sh
python3 scripts/http_fixture.py
```

终端二：

```sh
./bin/seven-eval --package examples/http run smoke
./bin/seven-eval --package examples/http serve
```

受控目标监听 `127.0.0.1:7432`。默认请求只使用 Case inputs，不发送 reference。
自己的 HTTP 目标通过 adapter 配置 endpoint、请求/响应映射、超时及凭据环境变量名；
请求字段映射不执行任意表达式。RAG 响应可映射检索、引用和工具证据，见 [数据与适配器](docs/phase3-usage.md)。

### 创建自己的 Evaluation Package

```sh
./bin/seven-eval init ./my-evaluation
./bin/seven-eval --package ./my-evaluation discover --json
./bin/seven-eval --package ./my-evaluation run smoke
```

```text
my-evaluation/
├── evaluation.yaml
├── plans/                   # 评测计划
├── datasets/                # 规范化 JSONL Case
├── sources/                 # JSONL/CSV/TREC/BEIR 来源
├── mappings/                # 字段映射和受限转换
├── adapters/                # HTTP/Seven 或 Python/TS/JS Subject
├── evaluators/              # 逐 Case 指标
├── processors/              # 分批聚合、KPI 与图表数据
├── conclusions/             # 基于统计和证据的结论
├── catalogs/                # 指标、原因、结论码表
└── tests/                   # 测试，不作为运行资产发现
```

运行状态放在 Package 的 `.seven-evaluate/`。逻辑引用按 Package 解析，搬迁目录不需要改绝对路径。
发现会报告无效引用、重复 ID 和不可用运行时；不会调用被测目标，但脚本模块顶层代码会加载。

```yaml
schema_version: evaluate.plan/v1
id: smoke
mode: replay
dataset: dataset:sample
# preset 默认为 builtin.basic@1
# processors: [processor:team-summary]  # 替换该阶段
# conclusions: []                       # 显式关闭
```

一个已有回答 Case：

```json
{"id":"refund","inputs":{"question":"退款多久到账？"},"response":"3天","contexts":["退款通常需要3天。"],"references":{"exact":"3天"}}
```

replay 计划要求 response 字段，`null` 表示缺失；invoke 计划配置 subject 后调用目标。
只有严格相等确实有意义时才提供 `references.exact`；JSON 约束可使用 `references.schema`。
自由文本不会自动得到整体业务正确率。

## 扩展与 AI 辅助开发

| 扩展 | 职责 |
|---|---|
| Subject | 调用目标、收集回答与证据；可提供可序列化 session hooks |
| Evaluator | 对 Case 与实际结果生成带状态、身份和参数的指标 |
| Processor | 通过 initialize/update/finalize 分批计算统计、KPI 与领域图表 |
| Conclusion | 根据处理器输出和计数给出结论，说明证据不足 |

每个脚本入口注册一个扩展。Python 导入名为 `seven_evaluate`，TS/JS 导入名为 `@seven-agent/eval`。
省略阶段列表继承预设，显式列表替换，`[]` 关闭。Processor 关闭或替换后不会偷偷补旧统计。
Worker 每次操作使用独立进程，不依赖模块内存跨 Turn 保存状态。
TS/JS 支持 ESM、Node 内置模块和 Package 内相对导入；第三方 npm 裸包与 CommonJS 暂不支持。

试用现有跨语言扩展：

```sh
./bin/seven-eval --package examples/typescript run typescript
./bin/seven-eval --package examples/typescript run python
```

想让 AI 帮你开发，可以直接提供这个任务：

> 阅读 `skills/seven-eval-sdk/SKILL.md`，在一个独立 Evaluation Package 中实现 TS Processor，
> 统计已评分样本、覆盖率和异常；使用合成数据运行，核对报告并导出 `.seven.eval`。

Skill 提供 Python/TS 可运行模板、指标与 KPI 语义、错误/取消/session 规则和交付检查。
它随源码与安装包分发，可复制到 AI 工具的项目级 Skill 目录；
不会自动安装或改变助手配置。[接入与使用](docs/ai-development.md)说明了可携带方式。

## 导出与独立查看

先从 run 输出取得 `run_id`，导出到尚不存在的文件：

```sh
./bin/seven-eval --package ./my-evaluation export RUN_ID ./demo.seven.eval
./bin/seven-eval verify ./demo.seven.eval
mkdir -p ./viewer-workspace
./bin/seven-eval --package ./viewer-workspace serve ./demo.seven.eval
```

`.seven.eval` 为 **2.0** 格式的有限 ZIP 快照：包含有效配置、输入与结果分片、Evidence、指标、
Processor 输出、Conclusion、码表、schema 与自包含摘要 HTML。
查看器无需原 Package、用户扩展或 Python/Node；导入不会执行包内脚本。
旧 1.0 包和旧运行状态已硬切，不提供迁移层。分享前请检查证据是否包含敏感内容。

## 持续评分与有状态评测

stream 计划只评分已发生的请求，不调用 Subject。通过 EEP 或支持的 OTLP/OpenInference JSON 子集接入，
接收持久化后 ACK；暂停期间继续接收，恢复后处理积压。窗口按事件时间聚合，迟到证据产生新修订；
导出固定快照。[持续评分快速开始](docs/continuous-scoring.md)提供无凭据事件源。

Episode 内 Turn 顺序执行并共享新建 session，Episode 之间隔离；失败、超时或取消保留部分结果。
Seven Workspace SSE 和历史导入已经过受控协议验收，真实 Seven 环境与生产流量验收仍待完成。

## 执行边界与支持环境

- 默认配方生成基础结果、统计和结论；judge 显式启用，默认不调用付费模型。
- 离线默认串行，当前上限 10,000 个展开 Case、单来源文件 1 GiB、规范化记录 2 MiB。
  调用 Subject 前完成全量输入校验；其他输出/结果包限制见 [有界执行契约](docs/bounded-evaluation.md)。
- 无业务请求自动重试；取消或中断保留部分结果，重启不自动重放不确定的副作用调用。
- 默认保留数据。清理是停服后的显式维护，先 preview 再 apply；先导出需要保留的证据。
- 本地可信用户工具：仅回环服务，无公网认证、多租户隔离或不可信代码沙箱。
- 最近原生安装验收为 macOS arm64；另有 Debian arm64 容器验收记录。
  Windows、x86_64 安装、独立物理第二机器、签名/公证尚未验收，见 [支持矩阵](docs/installation.md)。
- 当前没有自动参数搜索、OTLP protobuf、第三方 npm 依赖支持或生产 SLA。

源码运行、受控协议、浏览器验收和真实生产验收分别记录；历史阶段文档不覆盖当前协议。

## 开发、贡献与安全反馈

```sh
make test
go vet ./...
python3 scripts/acceptance.py
make acceptance-typescript
make acceptance-live
```

完整开发与贡献规则见 [CONTRIBUTING.md](CONTRIBUTING.md)，
全部指南见 [文档索引](docs/README.md)，候选内容见 [Release 说明](docs/release-alpha.md)。
托管 CI 包含 Linux 回归和手动触发的 macOS 安装验收；实际结果见
[GitHub Actions](https://github.com/CaixyPromise/seven-agent-eval/actions/workflows/ci.yml)。

漏洞反馈采用目标仓库的 **GitHub private vulnerability reporting**：
从 Security → Advisories → Report a vulnerability 提交，或直接使用
[私密漏洞报告入口](https://github.com/CaixyPromise/seven-agent-eval/security/advisories/new)。
不要在公开 Issue 中附凭据或真实生产 trace。[安全边界与渠道状态](SECURITY.md)。

项目按 [Apache-2.0](LICENSE) 授权，署名见 [NOTICE](NOTICE)，第三方通知见
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。品牌使用不授予商标权。

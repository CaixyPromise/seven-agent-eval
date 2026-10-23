# 开源首发准备记录

## 当前决定（2026-10-24）

- 当前分支 main，首发候选 **v0.1.0-alpha.1**；SDK 元数据为 0.1.0，未发布到注册表。
- Apache-2.0、Copyright 2026 CaixyPromise、公开范围与保留两张原型图均已确认。
- 保留 Git 历史及作者邮箱；目标仓库 **CaixyPromise/seven-agent-eval**。
- gh 登录和账户 API 已重新核对成功；此前 401 属历史状态。
- 漏洞反馈确定采用 GitHub private vulnerability reporting，仓库创建后需启用验证。
- 目前仅本地 checkpoint；未创建远程仓库、推送或发布 tag；托管 CI 尚未执行。
- SDK 开发 Skill 随源码和目录包提供，见 [AI 辅助开发](ai-development.md)。

下文保留此前各轮验证的实际时点。最初基线为 `56d3631`，开发分支
`codex/agent-rag-phase3`，后已建立本地 main；历史中的待确认事项以本节为准。
许可证见 [已确认决定](licensing.md)。

## 公开范围

- 当前源码、协议、内置配方、合成 fixtures、SDK 和文档；不包含状态、venv、缓存及结果包。
- Git 历史 14 个基线提交已使用 Gitleaks 8.30.1 全历史扫描，无命中；暂存新增内容也已扫描无命中。
- `public_content_check.py` 检查受跟踪的私有/生成路径、个人绝对路径与 Markdown 本地链接。
- 两张原型 PNG 已查看：为示例客服界面，无可见凭据；所有者已确认保留公开。
- 历史曾提交 11,750 字节的 `artifacts/phase2/window.seven.eval`，当前树已删除。
  已只读展开检查 6 个成员、12 个合成退款问答；无个人绝对路径，解包内容 Gitleaks 扫描无命中。
  `scripts/history_payload_check.py` 可重复该检查，临时展开内容已删除；未重写历史。
- Git 提交作者邮箱会随历史公开；未重写历史或更改作者归属。
- 自动扫描不能证明任意隐私文本不存在，也不证明拥有全部材料权利。

## 当前文档

README 面向用户引导安装与各使用场景；新索引将当前契约和历史记录分开。
Python、持续评分、安全边界、贡献指南、首发变更记录已补充。
结果为 2.0 硬切；旧阶段记录保留原时点事实。真实 Seven/模型/生产 T14 待验证。

## 可重复验收

先精确暂存准备纳入 checkpoint 的文件，再运行：

```sh
gitleaks git --redact --no-banner --log-opts=--all .
gitleaks git --staged --redact --no-banner .
python3 scripts/history_payload_check.py
python3 scripts/public_content_check.py
python3 scripts/clean_acceptance.py --python /absolute/external/python3.11
git diff --check
```

clean_acceptance 从 Git index tree 导出到临时独立仓库，只使用受跟踪文件。
重新准备 Python/Node 依赖、构建、完整测试、vet、离线/RAG/TS/比较/live/Seven
受控验收，然后生成目录包并用安装版执行/重开。输出在 `artifacts/open-source/`。
临时源码、依赖环境和运行状态在 finally 清理；不运行百万测试。

这证明干净源码检出，不是新操作系统：复用已安装工具链与下载缓存。
安装阶段由既有 harness 验证无原 Package、无 Python/Node 查看器。
不把 API/HTML 检查冒称新浏览器交互验收；既有浏览器证据见第五阶段记录。

## CI

依赖检查发现旧 Vite 6.0.7 与 source-map-js 的开发链公告命中，已在 Vite 6 线升级至
6.4.4，锁定 source-map-js 1.2.2；TS SDK 构建 esbuild 升至 0.25.12。
没有改动 React/Ant Design/ECharts 或引入新前端框架。Go esbuild 只调用 Build API，
不运行其开发服务器；本轮未改变 Go 执行核心。

首轮干净树回归有一次 `TestLiveInterruptedJudgeNeverReplayed` 在 Ingest 准备时
`context deadline exceeded`，完整测试退出 2；临时目录已清理。
相同测试单独连续运行 3 次均通过（3.41/2.70/3.39 秒），未稳定复现，不能断言根因。
保留初次日志；最终树完整重跑另行记录，不扩大超时或屏蔽测试。

`.github/workflows/ci.yml` 使用只读权限、固定 action commit，不包含发布步骤。
常规 Linux 构建/回归与秘密扫描；手动 macOS job 完成安装链路。
尚未公开仓库，GitHub 托管运行状态为 NOT_RUN；本地执行和静态检查分别记录。
首次远程运行通过前不添加绿色 CI badge。

## 发布前剩余执行事项

1. 获得单独远程操作授权后，创建已选定的仓库并配置 remote。
2. 启用、验证 GitHub private vulnerability reporting，更新渠道可用状态。
3. 首次运行托管 CI，核对实际结果后再发布候选 tag/Release 与安装包。
4. 本地源码、Skill、文档与安装验收收 checkpoint；不重复询问已确定的署名与公开范围。

依赖 notices 随发行包生成；清单不等于法律兼容性或漏洞审计结论。
剩余真实生产验收和额外平台支持不是 Alpha 功能声明。

## 本轮最终验证（2026-10-24）

干净检出树 `f3a8ccf5519d6208209a9c0547eb0bb76928da49`，macOS 26.5.2 arm64，
Go 1.25.1、uv 0.10.2、Python 3.11.14、Node 20.19.5、pnpm 10.24.0。
最终 checkpoint 在该树之后仅补充本记录与 CI 的 uv 版本对齐；运行实现及依赖不变。

| 检查 | 本轮结果 |
|---|---|
| 干净目录 `make setup` | exit 0，33.581 秒；未复制原 venv/node_modules |
| `make test` | exit 0，372.656 秒；Go race core 328.939 秒、distribution/server 通过；发布工具 3、TS SDK 5、前端 11、Python 28 项通过 |
| `go vet ./...`、`make build` | 均 exit 0；Vite 大 chunk 警告保留 |
| 当前公开文件/本地链接检查 | exit 0，290 文件，无私有/生成路径混入或个人绝对路径命中 |
| 离线/HTTP、RAG、TS、比较、live、Seven 受控验收 | 全部 exit 0；比较 3 候选/15 次调用，live 封闭窗口重算一致 |
| 发行与安装 | 均 exit 0；488 文件，178 依赖/工具链条目，通知材料缺失 0 |
| 安装版迁移与查看器 | 真实 HTTP 1 次、reference 未泄漏；Package 搬迁/删除后，无 Python/Node 完整 Run 重开相等 |
| 安装版 TS KPI | scored 5、coverage 1、anomalies 2；重装/卸载保留用户内容 |
| 全部 npm 依赖在线审计（含开发依赖） | web 188、SDK 32 个依赖，均 exit 0，当前公告命中 0 |
| CI 静态与脚本检查 | actionlint 1.7.7、Ruff、暂存 diff 检查通过 |
| 秘密检查 | Gitleaks 8.30.1 历史/暂存内容、历史 ZIP 解包均无命中 |

178 条清单含 Go 模块 31、npm 140、Python wheels 6、Go runtime 1。
原文及摘要在包内 `third-party/`。npm 声明含 MIT、Apache-2.0、CC-BY-4.0、ISC、
BSD-3-Clause、0BSD；不把它们合并成项目许可证，也不声明法律兼容性已认证。

证据：`artifacts/open-source/final/acceptance.json`、各步骤日志、
`installed/{acceptance.json,demo.seven.eval,typescript.seven.eval,report.html}` 与 `releases/`。
初次失败日志仍在 `artifacts/open-source/test.log`；最终 JSON 明确 status=passed，
temporary_data_removed=true。临时安装/源码/执行状态均已删除；未跑百万数据测试。

收本地提交后再从该提交生成 `artifacts/open-source/checkpoint/` 发行包并复验安装，
其 manifest 的 commit/source_tree 与本地提交对应。上述完整回归的源码差异只限文档/CI。
当时公开发布前仍须完成许可证与公开范围决定；下节记录后续许可证确认。本轮没有新增浏览器交互、Linux 容器或
GitHub 托管 CI 运行证据，没有推送、tag 或真实生产调用。

## Apache-2.0 首发收口（2026-10-24）

在本地 main 完成，许可证 Apache-2.0 与版权署名 CaixyPromise 已由所有者确认。
根 LICENSE、NOTICE、SDK 独立 LICENSE 和 Python/TS/Web SPDX 元数据均已加入；
安装包携带 SDK 元数据、根许可证、署名及第三方通知。SDK 版本维持 0.1.0，
候选发行标识为 0.1.0-alpha.1，尚未发布。

本轮新执行检查（全部退出 0）：

- `make setup`、`make build`：Python 环境和 TS/UI 构建完成，Vite 既有大 chunk 警告保留。
- 发布工具 unittest 3 项、TS SDK 测试 5 项；`uv build --project sdk/python` 构建 sdist/wheel。
- 最终安装脚本：仅使用发行包安装，HTTP 实际调用 1 次，reference 未泄漏；
  Package 搬迁/删除，无 Node 时 Python 可用，无 Node/Python 查看器完整 Run 相等；
  TS KPI 为 scored 5 / coverage 1 / anomalies 2；重装/卸载保留用户内容。
- Gitleaks 修改前全历史 15 提交、历史 ZIP 展开扫描均无命中。
  检查 528 个可达 blob，个人绝对路径命中 0；历史中仍留有一份已从当前树删除的合成结果包。
- `git fsck --full` 无损坏，报告不可达 blob/tree；不属于分支可达历史，未进行 GC 或重写。

证据：`artifacts/apache-release/precommit/installed/acceptance.json`；checkpoint 提交后
从同一提交重新构建发行包并安装验证，最终证据固定为
`artifacts/apache-release/checkpoint/installed/acceptance.json` 与相邻 `releases/`。
打包时校验 LICENSE/NOTICE、SDK SPDX 和 wheel 元数据，结果包 CLI verify。
本轮未更改执行算法，未重跑完整 race/百万容量套件；未新增浏览器交互或跨 OS 验收。
临时安装环境由既有脚本 finally 清理，保留小型报告与结果包。

此轮时点所有者已允许历史姓名/邮箱公开；其后确认的完整范围见 [public-scope.md](public-scope.md)。
当时 gh API 返回 401，后续重新检查已成功。漏洞渠道现已选定 GitHub private vulnerability
reporting，启用仍待远程仓库创建。以上历史过程均未推送或公开。

## v0.1.0-alpha.1 文档与 Skill checkpoint

本轮完善中文 README，明确调用/历史/持续评分/参数比较、Python/TS 扩展、安装与查看器。
修正 TS 文档示例：处理的 Case 数不再冒充 scored KPI。首发范围、账户、安全反馈方式已同步。
新增 `skills/seven-eval-sdk`，含跨语言协议参考与等价 Processor 模板；安装包包含 Skill。
`scripts/clean_acceptance.py` 的许可证记录修正为已选定的 Apache-2.0。

可重复验证：

```sh
make build
pnpm --dir web test
python3 scripts/skill_acceptance.py
python3 -m unittest discover -s scripts -p 'test_release_tools.py' -v
python3 scripts/public_content_check.py
git diff --check
```

Skill frontmatter 另用 skill-creator 的 quick_validate.py 检查。
行为验收使用真实 Go CLI 与 Python/Node worker，包含 false、0、null 与 schema 执行错误；
比较完整 Case 与 Processor 输出，并校验导出包。不是仅检查 Markdown 语法。
输出在 `artifacts/alpha1-docs/skill/acceptance.json`。
从最终 checkpoint 生成的目录包与安装验收输出在 `artifacts/alpha1-docs/checkpoint/`，
记录 SDK Skill 安装、品牌资源提供、完整 Run 重开和临时目录清理状态。
本轮不新增浏览器/真实模型/生产验证；既有 Logo 视觉验收见品牌接入记录。

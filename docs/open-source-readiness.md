# 开源首发准备记录

本轮目标：本地 Alpha 候选准备，不创建公开仓库、不推送、不打发布 tag。
基线 `56d3631`，同一分支 `codex/agent-rag-phase3`。拟议版本 `0.1.0-alpha.1`，
不是已发布版本。许可证建议见 [选型](licensing.md)。

## 公开范围

- 当前源码、协议、内置配方、合成 fixtures、SDK 和文档；不包含状态、venv、缓存及结果包。
- Git 历史 14 个基线提交已使用 Gitleaks 8.30.1 全历史扫描，无命中；暂存新增内容也已扫描无命中。
- `public_content_check.py` 检查受跟踪的私有/生成路径、个人绝对路径与 Markdown 本地链接。
- 两张原型 PNG 为当前唯一受跟踪二进制，已查看：为示例客服界面，无可见凭据；公开授权由所有者确认。
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

## 发布前仍需所有者确定

1. 项目许可证、署名主体、原型图与代码公开权利。
2. 是否接受现有 Git 历史中的作者邮箱公开。
3. 实际公开仓库地址，以及可用的私密漏洞报告渠道。
4. 单独授权推送/发布；本 checkpoint 不进行这些动作。

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
公开发布前仍须完成许可证与公开范围决定；本轮没有新增浏览器交互、Linux 容器或
GitHub 托管 CI 运行证据，没有推送、tag 或真实生产调用。

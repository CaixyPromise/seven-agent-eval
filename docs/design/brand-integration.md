# Seven Agent 品牌接入

2026-10-24：按所有者要求，使用相邻 `seven-agent-brand/web-assets-v1.1.0` 已有资产。

- 工作台：`public/brand/light/svg/symbol.svg` → `web/src/assets/seven-agent-symbol.svg`。
- 浏览器图标：`public/favicon.svg` → `web/public/favicon.svg`，保留浅/深色自适应。
- SVG 几何、颜色与源资源相同，仅规范文件末尾换行；不重新绘制图案。
- 使用独立图案，保留 Seven Eval 产品名称；导入 Vite 构建产物，不依赖相邻仓库部署。
- 品牌标识不表示授予商标使用权，遵循 Apache-2.0 第 6 节。

## 验证

`pnpm --dir web build`（含 TypeScript）及 `pnpm --dir web test`（11 项）退出 0。
独立 Playwright Chromium，真实 Go server、合成 init Package，检查计划页和持续评分页。
桌面 1536×1024、窄屏 390×844；100% 缩放。Logo 实际加载成功，桌面 38×38，窄屏 32×32；
页面无水平溢出。品牌链接可聚焦，图片不重复朗读文字，SVG 使用 contain 保留比例。
favicon HTTP 200，XML 可解析为 SVG。既有 Vite 大 chunk 警告保留。
截图在 `output/playwright/eval-brand/{desktop,mobile}.png`，本地产物不提交。
首次访问碰到构建期间的目录页，构建完成后使用新查询参数重新访问并确认真实应用；
截图为真实工作台，不采用目录页。

| Before | After | Why |
|---|---|---|
| 蓝底文本数字 7 | 品牌库浅色独立 SVG 图案 | 采用统一品牌形状，保留产品文字识别 |
| 自绘 favicon | 品牌库自适应 favicon | 浏览器页签与工作台品牌一致 |
| 文本方块依赖字体和底色 | 固定尺寸、不可压缩、object-fit contain 图片 | 防止拉伸、挤压和文字基线偏移 |

本次只替换工作台与页签品牌；独立 HTML 报告内容和评测逻辑没有改动。

## 本地 checkpoint 与安装包验收

本轮补充安装验收脚本：从安装后的无 Python/Node 查看器请求 Logo 与 favicon，
比对磁盘实际字节，并检查编译后的 JS 引用带内容摘要的 Logo 文件。
验收 JSON 新增 `brand_assets_served` 和 `brand_symbol`；不是浏览器视觉验收的替代。

从本地 checkpoint 构建并执行：

```sh
python3 scripts/package_release.py --python python3.11 --output artifacts/brand-checkpoint/releases
python3 scripts/installation_acceptance.py --archive artifacts/brand-checkpoint/releases/seven-agent-eval-darwin-arm64.tar.gz --python python3.11 --output artifacts/brand-checkpoint/installed
./bin/seven-eval verify artifacts/brand-checkpoint/installed/typescript.seven.eval
```

结果记录在 `artifacts/brand-checkpoint/installed/acceptance.json`；安装包 manifest
记录实际 commit/source_tree，SHA-256 侧文件用于校验下载产物。
只保留小型结果包、HTML 和验收记录；临时安装、Package 与服务由 finally 清理。

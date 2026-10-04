import { conclusion } from "@seven-agent/eval";
export default conclusion(
  { id: "team-conclusion", version: "1.0.0" },
  (ctx) => [
    {
      handler: "team-conclusion@1.0.0",
      code: "TEAM_REVIEW",
      level: "info",
      summary: ctx.outputs.length
        ? "自定义统计已完成；异常样本需逐项复核，不代表整体业务正确率。"
        : "本次未生成处理器统计。",
    },
  ],
);

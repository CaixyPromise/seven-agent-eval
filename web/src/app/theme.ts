import type { ThemeConfig } from "antd";
export const evaluationTheme: ThemeConfig = {
  token: {
    colorPrimary: "#2563eb",
    colorText: "#17233b",
    colorTextSecondary: "#64748b",
    colorTextDescription: "#64748b",
    colorBgLayout: "#f6f8fc",
    colorBorder: "#e0e6ef",
    borderRadius: 8,
    controlHeight: 38,
    fontSize: 14,
    fontFamily:
      '-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif',
  },
  components: {
    Button: { primaryShadow: "none" },
    Table: {
      headerBg: "#fafbfe",
      headerColor: "#52627b",
      cellPaddingBlock: 12,
      cellPaddingInline: 18,
    },
    Card: { headerFontSize: 18 },
    Menu: {
      itemSelectedBg: "#eff6ff",
      itemSelectedColor: "#2563eb",
      itemHeight: 50,
      itemBorderRadius: 8,
    },
    Statistic: { contentFontSize: 36, titleFontSize: 14 },
    Tabs: { horizontalItemGutter: 24 },
  },
};

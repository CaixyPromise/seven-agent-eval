import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import ConfigProvider from "antd/es/config-provider";
import zhCN from "antd/es/locale/zh_CN";
import App from "./app/App";
import { PageBoundary } from "./app/PageBoundary";
import { evaluationTheme } from "./app/theme";
import "antd/dist/reset.css";
import "./styles/tokens.css";
import "./styles/app.css";
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ConfigProvider
      locale={zhCN}
      theme={evaluationTheme}
      button={{ autoInsertSpace: false }}
    >
      <PageBoundary>
        <App />
      </PageBoundary>
    </ConfigProvider>
  </StrictMode>,
);

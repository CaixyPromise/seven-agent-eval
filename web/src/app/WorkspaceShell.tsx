import { useRef, type ReactNode } from "react";
import Button from "antd/es/button";
import Menu from "antd/es/menu";
import Breadcrumb from "antd/es/breadcrumb";
import {
  ClipboardList,
  History,
  FileChartColumn,
  RadioTower,
  Upload,
  Folder,
} from "lucide-react";
import type { Route } from "./routes";
const navigation = [
  {
    key: "plans",
    label: <a href="#/plans">评测计划</a>,
    icon: <ClipboardList size={19} />,
  },
  {
    key: "runs",
    label: <a href="#/runs">运行记录</a>,
    icon: <History size={19} />,
  },
  {
    key: "report",
    label: <a href="#/reports">评测报告</a>,
    icon: <FileChartColumn size={19} />,
  },
  {
    key: "live",
    label: <a href="#/live">持续评分</a>,
    icon: <RadioTower size={19} />,
  },
];
export function WorkspaceShell({
  route,
  workspace,
  onImport,
  children,
}: {
  route: Route;
  workspace: string;
  onImport: () => void;
  children: ReactNode;
}) {
  const main = useRef<HTMLElement>(null);
  return (
    <div className="workspace-shell">
      <a
        className="skip-link"
        href="#main-content"
        onClick={(event) => {
          event.preventDefault();
          main.current?.focus();
        }}
      >
        跳到主内容
      </a>
      <aside className="sidebar">
        <a className="brand" href="#/plans">
          <span className="brand-mark">7</span>
          <span>Seven Eval</span>
        </a>
        <nav aria-label="主导航">
          <Menu mode="inline" selectedKeys={[route.page]} items={navigation} />
        </nav>
        <div className="sidebar-bottom">
          <Folder size={17} />
          <span>本地工作区</span>
        </div>
      </aside>
      <div className="workspace-main">
        <header className="topbar">
          <Breadcrumb items={[{ title: "工作区" }, { title: workspace }]} />
          <Button icon={<Upload size={16} />} onClick={onImport}>
            导入结果
          </Button>
        </header>
        <main
          id="main-content"
          ref={main}
          tabIndex={-1}
          className="page-content"
        >
          {children}
        </main>
      </div>
    </div>
  );
}

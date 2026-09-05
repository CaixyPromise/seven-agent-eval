import { Component, type ReactNode } from "react";
import Result from "antd/es/result";
import Button from "antd/es/button";
export class PageBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? (
      <Result
        status="warning"
        title="页面暂时无法打开"
        subTitle="请重新加载页面，再到运行记录中确认评测状态。"
        extra={
          <Button type="primary" onClick={() => window.location.reload()}>
            重新加载
          </Button>
        }
      />
    ) : (
      this.props.children
    );
  }
}

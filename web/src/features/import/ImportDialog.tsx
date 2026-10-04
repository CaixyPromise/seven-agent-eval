import { useEffect, useRef, useState } from "react";
import Button from "antd/es/button";
import Form from "antd/es/form";
import Modal from "antd/es/modal";
import Upload, { type UploadFile } from "antd/es/upload";
import { Upload as UploadIcon } from "lucide-react";
import { evaluationApi } from "../../api/client";
import { safeErrorCode, type ErrorCode } from "../../api/errors";
import { ErrorNotice } from "../../components/Feedback";
export function ImportDialog({
  onClose,
  onImported,
  onBackgroundImported,
}: {
  onClose: () => void;
  onImported: (id: string) => void;
  onBackgroundImported?: () => void;
}) {
  const [files, setFiles] = useState<UploadFile[]>([]),
    [error, setError] = useState<ErrorCode | null>(null),
    [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const active = useRef(true);
  const trigger = useRef(document.activeElement as HTMLElement | null);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
      const element = trigger.current;
      queueMicrotask(() => {
        if (element?.isConnected) element.focus();
      });
    };
  }, []);
  const close = () => {
    active.current = false;
    onClose();
  };
  async function submit() {
    const file = files[0]?.originFileObj;
    if (!file || lock.current) return;
    if (!file.name.endsWith(".seven.eval")) {
      setError("FILE_TYPE");
      return;
    }
    if (file.size > 4 * 1024 * 1024 * 1024) {
      setError("BUNDLE_TOO_LARGE");
      return;
    }
    lock.current = true;
    setBusy(true);
    setError(null);
    try {
      const run = await evaluationApi.import(file);
      if (active.current) onImported(run.id);
      else onBackgroundImported?.();
    } catch (cause) {
      if (active.current) setError(safeErrorCode(cause, "IMPORT_UNKNOWN"));
    } finally {
      lock.current = false;
      if (active.current) setBusy(false);
    }
  }
  return (
    <Modal
      title="导入评测结果"
      open
      onCancel={close}
      footer={null}
      destroyOnHidden
    >
      <Form layout="vertical" onFinish={() => void submit()}>
        <Form.Item label="结果文件" extra="单个文件不超过 4 GiB。">
          <Upload.Dragger
            accept=".seven.eval"
            multiple={false}
            maxCount={1}
            fileList={files}
            disabled={busy}
            beforeUpload={() => false}
            onChange={({ fileList }) => {
              setFiles(fileList);
              setError(null);
            }}
          >
            <p className="upload-icon">
              <UploadIcon size={30} />
            </p>
            <p className="ant-upload-text">点击或拖入 .seven.eval 文件</p>
          </Upload.Dragger>
        </Form.Item>
        {error ? <ErrorNotice code={error} /> : null}
        <div className="dialog-actions">
          <Button onClick={close}>关闭</Button>
          <Button
            type="primary"
            htmlType="submit"
            loading={busy}
            disabled={!files.length}
          >
            导入并查看
          </Button>
        </div>
      </Form>
    </Modal>
  );
}

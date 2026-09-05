import { useEffect, useRef, type ReactNode } from "react";
import Drawer from "antd/es/drawer";
import Modal from "antd/es/modal";
export function Dialog({
  title,
  children,
  onClose,
  drawer = false,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  drawer?: boolean;
}) {
  const trigger = useRef(document.activeElement as HTMLElement | null);
  useEffect(
    () => () => {
      const element = trigger.current;
      queueMicrotask(() => {
        if (element?.isConnected) element.focus();
      });
    },
    [],
  );
  return drawer ? (
    <Drawer
      title={title}
      open
      onClose={onClose}
      size={620}
      styles={{ wrapper: { maxWidth: "100vw" } }}
      focusable={{ trap: true, focusTriggerAfterClose: true }}
      destroyOnHidden
    >
      {children}
    </Drawer>
  ) : (
    <Modal
      title={title}
      open
      onCancel={onClose}
      footer={null}
      width={520}
      destroyOnHidden
    >
      {children}
    </Modal>
  );
}

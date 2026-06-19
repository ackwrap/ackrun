interface ConfirmDialogProps {
  open: boolean;
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  open,
  title,
  message,
  confirmText = '确认',
  cancelText = '取消',
  danger = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  if (!open) return null;

  return (
    <div className="aw-modal-backdrop z-[80]">
      <div className="aw-modal-panel max-w-md">
        <div className="border-b border-[var(--border-default)] px-5 py-4">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h3 className="text-base font-semibold text-white">{title}</h3>
              <p className="mt-2 text-sm leading-6 text-[var(--text-secondary)]">{message}</p>
            </div>
            <button onClick={onCancel} className="aw-modal-close" title="关闭">×</button>
          </div>
        </div>
        <div className="flex justify-end gap-2 px-5 py-4">
          <button onClick={onCancel} className="aw-confirm-cancel">
            {cancelText}
          </button>
          <button onClick={onConfirm} className={danger ? 'aw-confirm-danger' : 'aw-confirm-primary'}>
            {confirmText}
          </button>
        </div>
      </div>
    </div>
  );
}

import { AlertTriangle, Check, Info } from 'lucide-react';

interface ToastProps {
  message: string;
  type?: 'success' | 'error' | 'info';
}

export function Toast({ message, type = 'info' }: ToastProps) {
  if (!message) return null;

  const tone = type === 'success' ? 'aw-toast-success' : type === 'error' ? 'aw-toast-error' : 'aw-toast-info';
  const label = type === 'success' ? '成功' : type === 'error' ? '失败' : '提示';
  const Icon = type === 'success' ? Check : type === 'error' ? AlertTriangle : Info;

  return (
    <div className="pointer-events-none fixed bottom-[13vh] left-1/2 z-[70] w-full max-w-xl -translate-x-1/2 px-4">
      <div className={`aw-toast ${tone}`} role={type === 'error' ? 'alert' : 'status'} aria-live={type === 'error' ? 'assertive' : 'polite'}>
        <span className="aw-toast-icon"><Icon size={19} strokeWidth={2.3} /></span>
        <span className="min-w-0">
          <span className="aw-toast-label">{label}</span>
          <span className="aw-toast-message">{message}</span>
        </span>
      </div>
    </div>
  );
}

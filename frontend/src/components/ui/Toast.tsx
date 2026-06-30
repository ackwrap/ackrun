interface ToastProps {
  message: string;
  type?: 'success' | 'error' | 'info';
}

export function Toast({ message, type = 'info' }: ToastProps) {
  if (!message) return null;

  const tone = type === 'success' ? 'aw-toast-success' : type === 'error' ? 'aw-toast-error' : 'aw-toast-info';

	return (
		<div className="fixed left-1/2 top-6 z-[70] -translate-x-1/2 px-4 sm:left-auto sm:right-6 sm:translate-x-0">
			<div className={`aw-toast ${tone}`}>
        {message}
      </div>
    </div>
  );
}

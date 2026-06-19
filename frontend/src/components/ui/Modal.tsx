import React, { useState, useEffect, useCallback, useRef } from 'react';

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  width?: number;
  footer?: React.ReactNode;
  children: React.ReactNode;
  closable?: boolean;
  className?: string;
}

export function Modal({ open, onClose, title, width = 520, footer, children, closable = true, className = '' }: ModalProps) {
  const [visible, setVisible] = useState(false);
  const [exiting, setExiting] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const prevFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape' && closable) onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, closable, onClose]);

  useEffect(() => {
    if (open) {
      prevFocusRef.current = document.activeElement as HTMLElement;
      setVisible(true);
      setExiting(false);
      document.body.style.overflow = 'hidden';
    } else if (visible) {
      setExiting(true);
      const timer = setTimeout(() => { setVisible(false); setExiting(false); document.body.style.overflow = ''; prevFocusRef.current?.focus(); }, 200);
      return () => clearTimeout(timer);
    }
    return () => { document.body.style.overflow = ''; };
  }, [open]);

  const handleBackdrop = useCallback(() => { if (closable) onClose(); }, [closable, onClose]);

  if (!visible) return null;

  return (
    <div className="fixed inset-0 z-[var(--z-modal)] flex items-center justify-center p-4" onClick={handleBackdrop} role="dialog" aria-modal="true" aria-label={title}>
      <div className={`absolute inset-0 bg-[var(--bg-overlay)] ${exiting ? 'animate-modal-overlay-out' : 'animate-modal-overlay-in'}`} />
      <div ref={panelRef} className={`relative bg-[rgba(15,27,44,0.96)] backdrop-blur-xl rounded-[var(--radius-xl)] shadow-[var(--shadow-xl)] border border-[var(--border-default)] w-full mx-auto focus-ring ${exiting ? 'animate-modal-content-out' : 'animate-modal-content-in'} ${className}`} style={{ maxWidth: width }} onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-6 py-4 border-b border-[var(--border-light)]">
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">{title}</h3>
          {closable && (
            <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors focus-ring cursor-pointer" aria-label="Close">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M12 4L4 12M4 4l8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" /></svg>
            </button>
          )}
        </div>
        <div className="px-6 py-4">{children}</div>
        {footer && <div className="flex items-center justify-end gap-2 px-6 py-4 border-t border-[var(--border-light)]">{footer}</div>}
      </div>
    </div>
  );
}
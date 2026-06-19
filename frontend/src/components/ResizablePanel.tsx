import React, { useCallback, useRef, useState } from 'react';

interface ResizablePanelProps {
  left: React.ReactNode;
  right: React.ReactNode;
  defaultLeftWidth?: number;
  minLeftWidth?: number;
  maxLeftWidth?: number;
  className?: string;
}

export function ResizablePanel({
  left,
  right,
  defaultLeftWidth = 420,
  minLeftWidth = 280,
  maxLeftWidth = 700,
  className = '',
}: ResizablePanelProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [leftWidth, setLeftWidth] = useState(defaultLeftWidth);
  const [dragging, setDragging] = useState(false);

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    setDragging(true);

    const startX = e.clientX;
    const startWidth = leftWidth;

    const handleMouseMove = (moveEvent: MouseEvent) => {
      if (!containerRef.current) return;
      const delta = moveEvent.clientX - startX;
      const newWidth = Math.min(maxLeftWidth, Math.max(minLeftWidth, startWidth + delta));
      setLeftWidth(newWidth);
    };

    const handleMouseUp = () => {
      setDragging(false);
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
  }, [leftWidth, minLeftWidth, maxLeftWidth]);

  return (
    <div ref={containerRef} className={`flex h-full ${className}`} style={{ userSelect: dragging ? 'none' : 'auto' }}>
      <div className="shrink-0 overflow-y-auto" style={{ width: leftWidth }}>
        {left}
      </div>
      <div
        className={`w-1 shrink-0 cursor-col-resize bg-[var(--border-default)] transition-colors hover:bg-blue-500/60 ${dragging ? 'bg-blue-500' : ''}`}
        onMouseDown={handleMouseDown}
      />
      <div className="min-w-0 flex-1 overflow-y-auto">
        {right}
      </div>
    </div>
  );
}
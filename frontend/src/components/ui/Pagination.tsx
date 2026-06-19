import React from 'react';

interface PaginationProps {
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
  onPageChange: React.Dispatch<React.SetStateAction<number>>;
  onPageSizeChange: React.Dispatch<React.SetStateAction<number>>;
  pageSizeOptions?: number[];
}

export function Pagination({
  total,
  page,
  pageSize,
  totalPages,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = [10, 25, 50, 100],
}: PaginationProps) {
  const start = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const end = Math.min(total, page * pageSize);
  const buttonClass = (disabled: boolean) => `h-9 rounded-md border border-[var(--border-default)] px-3 ${disabled ? 'cursor-not-allowed bg-white/[0.02] text-[var(--text-tertiary)]' : 'bg-white/[0.04] text-white hover:bg-white/[0.08]'}`;

  return (
    <div className="mt-4 flex flex-col gap-3 rounded-md border border-[var(--border-default)] bg-white/[0.025] px-3 py-3 text-sm text-[var(--text-secondary)] md:flex-row md:items-center md:justify-between">
      <div>显示 {start}-{end} / 共 {total} 条</div>
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-[var(--text-tertiary)]">每页</span>
        <select
          value={pageSize}
          onChange={e => {
            onPageChange(1);
            onPageSizeChange(Number(e.target.value));
          }}
          className="h-9 rounded-md border border-[var(--border-default)] bg-[#152235] px-2 text-sm text-white outline-none focus:border-blue-400"
        >
          {pageSizeOptions.map(size => <option key={size} className="bg-[#152235] text-white" value={size}>{size}</option>)}
        </select>
        <button onClick={() => onPageChange(1)} disabled={page <= 1} className={buttonClass(page <= 1)}>首页</button>
        <button onClick={() => onPageChange(prev => Math.max(1, prev - 1))} disabled={page <= 1} className={buttonClass(page <= 1)}>上一页</button>
        <span className="px-2 text-white">第 {page} / {totalPages} 页</span>
        <button onClick={() => onPageChange(prev => Math.min(totalPages, prev + 1))} disabled={page >= totalPages} className={buttonClass(page >= totalPages)}>下一页</button>
        <button onClick={() => onPageChange(totalPages)} disabled={page >= totalPages} className={buttonClass(page >= totalPages)}>末页</button>
      </div>
    </div>
  );
}

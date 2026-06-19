interface PageSkeletonProps { title?: string; lines?: number }

export function PageSkeleton({ title, lines = 3 }: PageSkeletonProps) {
  return (
    <div className="space-y-5 animate-pulse">
      {title && <div className="h-7 w-40 bg-white/[0.06] rounded" />}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 backdrop-blur-xl">
            <div className="h-4 w-20 bg-white/[0.06] rounded mb-4" />
            <div className="flex items-center gap-4 mb-5">
              <div className="h-14 w-14 rounded-full bg-white/[0.06]" />
              <div className="flex-1 space-y-2">
                <div className="h-6 w-32 bg-white/[0.06] rounded" />
                <div className="h-4 w-24 bg-white/[0.04] rounded" />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="h-10 bg-white/[0.04] rounded" />
              <div className="h-10 bg-white/[0.04] rounded" />
            </div>
          </div>
        ))}
      </div>
      {Array.from({ length: lines }).map((_, i) => (
        <div key={i} className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.92),rgba(16,27,43,0.74))] p-5 backdrop-blur-xl">
          <div className="h-5 w-48 bg-white/[0.06] rounded mb-4" />
          <div className="space-y-3">
            {Array.from({ length: 4 }).map((_, j) => (
              <div key={j} className="h-4 bg-white/[0.04] rounded" style={{ width: `${70 + Math.random() * 30}%` }} />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
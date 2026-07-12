import React from 'react';

interface MiniSparklineProps {
  data: number[];
  color?: 'green' | 'blue' | 'purple';
  className?: string;
}

const colorMap = {
  green: { line: '#10b981', area: '#10b981' },
  blue: { line: '#3b82f6', area: '#3b82f6' },
  purple: { line: '#a855f7', area: '#a855f7' },
};

export function MiniSparkline({ data, color = 'blue', className = '' }: MiniSparklineProps) {
  const gradientID = React.useId().replace(/:/g, '');
  if (data.length < 2) return <div className={`h-14 w-full ${className}`} />;

  const width = 100;
  const height = 56;
  const max = Math.max(1, ...data);
  const points = data.map((value, index) => {
    const x = data.length === 1 ? width : index / (data.length - 1) * width;
    const y = height - Math.max(2, value / max * (height - 5));
    return [x, y] as const;
  });
  const linePath = points.map(([x, y], index) => `${index === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`).join(' ');
  const areaPath = `${linePath} L ${width} ${height} L 0 ${height} Z`;
  const colors = colorMap[color];

  return (
    <svg className={`h-14 w-full ${className}`} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" aria-hidden="true">
      <defs>
        <linearGradient id={gradientID} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={colors.area} stopOpacity="0.32" />
          <stop offset="100%" stopColor={colors.area} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={areaPath} fill={`url(#${gradientID})`} />
      <path d={linePath} fill="none" stroke={colors.line} strokeWidth="1.6" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

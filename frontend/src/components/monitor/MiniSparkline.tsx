import React from 'react';
import * as echarts from 'echarts';
import type { ECharts } from 'echarts';

interface MiniSparklineProps {
  data: number[];
  color?: 'green' | 'blue' | 'purple';
  className?: string;
}

const colorMap = {
  green: {
    line: '#10b981',
    areaStart: 'rgba(16, 185, 129, 0.4)',
    areaEnd: 'rgba(16, 185, 129, 0.05)',
  },
  blue: {
    line: '#3b82f6',
    areaStart: 'rgba(59, 130, 246, 0.4)',
    areaEnd: 'rgba(59, 130, 246, 0.05)',
  },
  purple: {
    line: '#a855f7',
    areaStart: 'rgba(168, 85, 247, 0.4)',
    areaEnd: 'rgba(168, 85, 247, 0.05)',
  },
};

export function MiniSparkline({ data, color = 'blue', className = '' }: MiniSparklineProps) {
  const chartRef = React.useRef<HTMLDivElement>(null);
  const chartInstanceRef = React.useRef<ECharts | null>(null);

  React.useEffect(() => {
    if (!chartRef.current) return;

    const chart = echarts.init(chartRef.current, 'dark');
    chartInstanceRef.current = chart;

    const colors = colorMap[color];
    const option: echarts.EChartsOption = {
      grid: { left: 0, top: 0, right: 0, bottom: 0 },
      xAxis: {
        type: 'category',
        show: false,
        boundaryGap: false,
      },
      yAxis: {
        type: 'value',
        show: false,
      },
      series: [
        {
          type: 'line',
          symbol: 'none',
          smooth: true,
          lineStyle: {
            width: 1.5,
            color: colors.line,
          },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: colors.areaStart },
              { offset: 1, color: colors.areaEnd },
            ]),
          },
          data: data,
        },
      ],
    };

    chart.setOption(option);

    const handleResize = () => {
      chart.resize();
    };
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      chart.dispose();
      chartInstanceRef.current = null;
    };
  }, [color]);

  // 更新数据
  React.useEffect(() => {
    if (chartInstanceRef.current && data.length > 0) {
      chartInstanceRef.current.setOption({
        series: [{ data }],
      });
    }
  }, [data]);

  return <div ref={chartRef} className={`w-full ${className}`} style={{ height: '56px' }} />;
}

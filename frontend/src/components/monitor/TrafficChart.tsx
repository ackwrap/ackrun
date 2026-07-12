import React from 'react';
import type { ECharts, EChartsOption } from 'echarts';

interface TrafficChartProps {
  className?: string;
}

interface TrafficPoint {
  time: number;
  upload: number;
  download: number;
}

export interface TrafficChartRef {
  addData: (upload: number, download: number) => void;
}

export const TrafficChart = React.forwardRef<TrafficChartRef, TrafficChartProps>(({ className = '' }, ref) => {
  const chartRef = React.useRef<HTMLDivElement>(null);
  const chartInstanceRef = React.useRef<ECharts | null>(null);
  const disposedRef = React.useRef(false);
  const dataRef = React.useRef<TrafficPoint[]>([]);
  const maxDataPoints = 60; // 保留 60 秒数据

  const updateChart = React.useCallback(() => {
    if (!chartInstanceRef.current) return;

    const times = dataRef.current.map((d) => d.time.toString());
    const uploads = dataRef.current.map((d) => d.upload);
    const downloads = dataRef.current.map((d) => d.download);

    chartInstanceRef.current.setOption({
      xAxis: {
        data: times,
      },
      series: [
        { data: uploads },
        { data: downloads },
      ],
    });
  }, []);

  // 初始化图表
  React.useEffect(() => {
    if (!chartRef.current) return;
    disposedRef.current = false;

    let handleResize: (() => void) | null = null;

    import('echarts').then((echarts) => {
      if (!chartRef.current || disposedRef.current) return;

      const chart = echarts.init(chartRef.current);
      chartInstanceRef.current = chart;

      const option: EChartsOption = {
        backgroundColor: 'transparent',
        grid: {
          left: 8,
          right: 12,
          bottom: 8,
          top: 12,
          containLabel: true,
        },
        tooltip: {
          trigger: 'axis',
          axisPointer: {
            type: 'cross',
          },
          formatter: (params: any) => {
            const upload = params[0];
            const download = params[1];
            return `${upload.axisValueLabel}<br/>
            上传: ${formatSpeed(upload.value)}<br/>
            下载: ${formatSpeed(download.value)}`;
          },
        },
        xAxis: {
          type: 'category',
          boundaryGap: false,
          data: [],
          axisLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.18)' } },
          axisTick: { show: false },
          axisLabel: {
            color: '#7B8AA2',
            fontSize: 10,
            formatter: (value: string) => {
              const date = new Date(parseInt(value));
              return `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}:${date.getSeconds().toString().padStart(2, '0')}`;
            },
          },
        },
        yAxis: {
          type: 'value',
          splitLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.10)' } },
          axisLabel: { color: '#7B8AA2', fontSize: 10, formatter: (value: number) => formatSpeed(value) },
        },
        series: [
          {
            name: '上传',
            type: 'line',
            smooth: true,
            symbol: 'none',
            lineStyle: {
              width: 2,
              color: '#10b981',
            },
            areaStyle: {
              color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: 'rgba(16, 185, 129, 0.3)' },
                { offset: 1, color: 'rgba(16, 185, 129, 0.05)' },
              ]),
            },
            data: [],
          },
          {
            name: '下载',
            type: 'line',
            smooth: true,
            symbol: 'none',
            lineStyle: {
              width: 2,
              color: '#3b82f6',
            },
            areaStyle: {
              color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: 'rgba(59, 130, 246, 0.3)' },
                { offset: 1, color: 'rgba(59, 130, 246, 0.05)' },
              ]),
            },
            data: [],
          },
        ],
      };

      chart.setOption(option);
      updateChart();

      handleResize = () => {
        chart.resize();
      };
      window.addEventListener('resize', handleResize);
    });

    return () => {
      disposedRef.current = true;
      if (handleResize) window.removeEventListener('resize', handleResize);
      chartInstanceRef.current?.dispose();
      chartInstanceRef.current = null;
    };
  }, [updateChart]);

  // 暴露方法给父组件
  React.useImperativeHandle(ref, () => ({
    addData: (upload: number, download: number) => {
      const now = Date.now();
      dataRef.current.push({ time: now, upload, download });

      // 保留最近 60 个数据点
      if (dataRef.current.length > maxDataPoints) {
        dataRef.current.shift();
      }

      updateChart();
    },
  }), [updateChart]);

  return <div ref={chartRef} className={`w-full ${className || 'h-[300px]'}`} />;
});

TrafficChart.displayName = 'TrafficChart';

function formatSpeed(bytesPerSecond: number): string {
  if (bytesPerSecond === 0) return '0 B/s';
  const k = 1024;
  const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
  const i = Math.floor(Math.log(bytesPerSecond) / Math.log(k));
  return Math.round((bytesPerSecond / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
}

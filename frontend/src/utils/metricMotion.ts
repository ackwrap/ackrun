export interface FrameScheduler {
  now(): number;
  request(callback: (time: number) => void): number;
  cancel(id: number): void;
}

/** Retarget from the currently displayed value; never replay from zero. */
export function createNumberTransition(
  render: (value: number | null) => void,
  scheduler: FrameScheduler,
  duration = 200,
) {
  let displayed: number | null = null;
  let frame: number | null = null;
  let disposed = false;
  function cancel() {
    if (frame !== null) scheduler.cancel(frame);
    frame = null;
  }
  return {
    set(value: number | null, animate = true) {
      if (disposed) return;
      cancel();
      const target =
        value !== null && Number.isFinite(value) ? Math.max(0, value) : null;
      if (
        !animate ||
        displayed === null ||
        target === null ||
        displayed === target
      ) {
        displayed = target;
        render(displayed);
        return;
      }
      const from = displayed;
      const start = scheduler.now();
      function tick(time: number) {
        const progress = Math.min(1, Math.max(0, (time - start) / duration));
        displayed = from + (target! - from) * (1 - (1 - progress) ** 3);
        render(displayed);
        frame = progress < 1 ? scheduler.request(tick) : null;
      }
      frame = scheduler.request(tick);
    },
    dispose() {
      cancel();
      disposed = true;
    },
  };
}

export function appendTrafficSample(
  samples: readonly number[],
  value: number,
  limit = 32,
): number[] {
  return [...samples, Number.isFinite(value) ? Math.max(0, value) : 0].slice(
    -Math.max(1, limit),
  );
}

/** A fixed baseline and bounded history keep the tiny curve stable between samples. */
export function trafficSparkline(samples: readonly number[]): string {
  if (samples.length < 2) return "";
  const peak = Math.max(1, ...samples);
  return samples
    .map((value, index) => {
      const x = (index / (samples.length - 1)) * 100;
      const y = 11 - Math.min(1, Math.max(0, value / peak)) * 9;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");
}

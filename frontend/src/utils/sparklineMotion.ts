import { createNumberTransition, type FrameScheduler } from "./metricMotion.ts";

/** Equal-sized profiles let new samples and scale changes morph without jumping. */
export function trafficProfile(samples: readonly number[]): number[] {
  if (samples.length < 2) return [];
  const values = samples
    .slice(-32)
    .map((value) => (Number.isFinite(value) ? Math.max(0, value) : 0));
  const peak = Math.max(1, ...values);
  return Array.from({ length: 32 }, (_, index) => {
    const position = (index / 31) * (values.length - 1);
    const left = Math.floor(position);
    const fraction = position - left;
    const value =
      values[left]! * (1 - fraction) +
      values[Math.min(left + 1, values.length - 1)]! * fraction;
    return 11 - (value / peak) * 9;
  });
}

/** Monotone tangents round corners without overshooting peaks or the zero line. */
export function smoothTrafficPath(levels: readonly number[]): string {
  if (levels.length < 2) return "";
  const step = 100 / (levels.length - 1);
  const slopes = levels.slice(1).map((value, index) => value - levels[index]!);
  const tangent = (index: number) => {
    if (index === 0) return slopes[0]!;
    if (index === levels.length - 1) return slopes[slopes.length - 1]!;
    const before = slopes[index - 1]!;
    const after = slopes[index]!;
    return before * after <= 0 ? 0 : (2 * before * after) / (before + after);
  };
  const number = (value: number) => value.toFixed(3);
  let path = `M 0 ${number(levels[0]!)}`;
  for (let index = 1; index < levels.length; index++) {
    const x = index * step;
    path += ` C ${number(x - (step * 2) / 3)} ${number(levels[index - 1]! + tangent(index - 1) / 3)} ${number(x - step / 3)} ${number(levels[index]! - tangent(index) / 3)} ${number(x)} ${number(levels[index]!)}`;
  }
  return path;
}

export function createSparklineTransition(
  render: (path: string) => void,
  scheduler: FrameScheduler,
) {
  let displayed: number[] = [];
  let from: number[] = [];
  let target: number[] = [];
  let disposed = false;
  const tween = createNumberTransition(
    (progress) => {
      if (progress === null) return;
      displayed = target.map(
        (value, index) => from[index]! + (value - from[index]!) * progress,
      );
      render(smoothTrafficPath(displayed));
    },
    scheduler,
    240,
  );
  return {
    set(samples: readonly number[], animate = true) {
      if (disposed) return;
      tween.set(null, false);
      const next = trafficProfile(samples);
      if (
        next.length === displayed.length &&
        next.every((value, index) => value === displayed[index])
      )
        return;
      if (!animate || displayed.length === 0 || next.length === 0) {
        displayed = next;
        render(smoothTrafficPath(displayed));
        return;
      }
      from = displayed.slice();
      target = next;
      tween.set(0, false);
      tween.set(1);
    },
    dispose() {
      disposed = true;
      tween.dispose();
    },
  };
}

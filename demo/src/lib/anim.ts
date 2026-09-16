import { Easing, interpolate } from "remotion";

const clampOpts = {
  extrapolateLeft: "clamp" as const,
  extrapolateRight: "clamp" as const,
};

const easeOut = Easing.bezier(0.22, 1, 0.36, 1);

/** frame が start を過ぎてから dur フレームかけて 0→1 になる不透明度。 */
export function revealOpacity(frame: number, start: number, dur = 14): number {
  return interpolate(frame, [start, start + dur], [0, 1], {
    ...clampOpts,
    easing: easeOut,
  });
}

/** フェードアップ(下から上へ)の Y オフセット(px)。 */
export function revealY(
  frame: number,
  start: number,
  dur = 14,
  distance = 18,
): number {
  return interpolate(frame, [start, start + dur], [distance, 0], {
    ...clampOpts,
    easing: easeOut,
  });
}

/** start より前は真、経過後は偽になる真偽値をなめらかな 0/1 として返す(表示切替のクロスフェード用)。 */
export function stepOpacity(frame: number, at: number, dur = 8): number {
  return interpolate(frame, [at, at + dur], [1, 0], {
    ...clampOpts,
    easing: Easing.bezier(0.4, 0, 1, 1),
  });
}

export function clamp01(t: number): number {
  return t < 0 ? 0 : t > 1 ? 1 : t;
}

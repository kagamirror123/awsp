import React from "react";
import { AbsoluteFill, interpolate, useCurrentFrame, useVideoConfig } from "remotion";
import { SCENE_FADE, theme } from "../theme";

/**
 * シーンの入り/抜けをフェードで包む共通ラッパー。
 * 背景色がシーン間で共通なので、フェードイン/アウトがクロスディゾルブのように見える。
 */
export const SceneFade: React.FC<{ children: React.ReactNode; noFadeOut?: boolean; noFadeIn?: boolean }> = ({
  children,
  noFadeOut,
  noFadeIn,
}) => {
  const frame = useCurrentFrame();
  const { durationInFrames } = useVideoConfig();

  const fadeIn = noFadeIn
    ? 1
    : interpolate(frame, [0, SCENE_FADE], [0, 1], {
        extrapolateLeft: "clamp",
        extrapolateRight: "clamp",
      });
  const fadeOut = noFadeOut
    ? 1
    : interpolate(
        frame,
        [durationInFrames - SCENE_FADE, durationInFrames],
        [1, 0],
        { extrapolateLeft: "clamp", extrapolateRight: "clamp" },
      );

  return (
    <AbsoluteFill style={{ background: theme.bg, opacity: fadeIn * fadeOut }}>
      {children}
    </AbsoluteFill>
  );
};

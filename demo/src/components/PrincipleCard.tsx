import React from "react";
import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { theme } from "../theme";

export interface PrincipleCardProps {
  start: number;
  icon: React.ReactNode;
  title: string;
  accent: string;
  width?: number;
  height?: number;
}

export const PrincipleCard: React.FC<PrincipleCardProps> = ({
  start,
  icon,
  title,
  accent,
  width = 500,
  height = 300,
}) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const local = frame - start;
  if (local < -5) return null;

  const pop = spring({
    frame: local,
    fps,
    config: { damping: 15, stiffness: 160, mass: 0.9 },
    durationInFrames: 22,
  });
  const opacity = interpolate(local, [0, 12], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const scale = interpolate(pop, [0, 1], [0.85, 1]);
  const y = interpolate(pop, [0, 1], [30, 0]);

  return (
    <div
      style={{
        width,
        height,
        opacity,
        transform: `translateY(${y}px) scale(${scale})`,
        borderRadius: 20,
        background: theme.panel,
        border: `1px solid ${theme.border}`,
        padding: "34px 34px",
        display: "flex",
        flexDirection: "column",
        gap: 22,
        boxShadow: "0 30px 70px rgba(0,0,0,0.4)",
      }}
    >
      <div
        style={{
          width: 64,
          height: 64,
          borderRadius: 16,
          background: `${accent}1f`,
          border: `1px solid ${accent}55`,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          color: accent,
        }}
      >
        {icon}
      </div>
      <div
        style={{
          fontFamily: theme.fontSans,
          fontSize: 27,
          fontWeight: 700,
          color: theme.text,
          lineHeight: 1.5,
          whiteSpace: "pre-line",
        }}
      >
        {title}
      </div>
      <div style={{ height: 4, width: 56, background: accent, borderRadius: 2 }} />
    </div>
  );
};

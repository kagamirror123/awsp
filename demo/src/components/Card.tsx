import React from "react";
import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { theme } from "../theme";
import { revealOpacity, revealY } from "../lib/anim";

export interface CardLine {
  label: string;
  value: string;
  valueColor?: string;
}

export interface CardProps {
  start: number;
  heading: string;
  headingColor?: string;
  lines: CardLine[];
  width?: number;
  fontSize?: number;
  labelWidth?: number;
}

/** ui.RenderCard(internal/ui/card.go) と同じ「見出し + 角丸ボックス」の体裁の汎用カード。 */
export const Card: React.FC<CardProps> = ({
  start,
  heading,
  headingColor = theme.green,
  lines,
  width = 900,
  fontSize = 26,
  labelWidth = 150,
}) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const local = frame - start;
  if (local < -5) return null;

  const pop = spring({
    frame: local,
    fps,
    config: { damping: 16, stiffness: 180, mass: 0.8 },
    durationInFrames: 20,
  });
  const opacity = interpolate(local, [0, 10], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const scale = interpolate(pop, [0, 1], [0.92, 1]);

  return (
    <div style={{ opacity, transform: `scale(${scale})`, transformOrigin: "left top" }}>
      <div
        style={{
          fontFamily: theme.fontMono,
          fontSize: fontSize * 0.95,
          fontWeight: 700,
          color: headingColor,
          marginBottom: 14,
        }}
      >
        {heading}
      </div>
      <div
        style={{
          width,
          border: `1px solid ${theme.blue}66`,
          borderRadius: 14,
          background: theme.panelAlt,
          padding: "24px 30px",
          display: "flex",
          flexDirection: "column",
          gap: 10,
        }}
      >
        {lines.map((line, i) => {
          const rowStart = 8 + i * 3;
          const rowOpacity = revealOpacity(local, rowStart, 10);
          const rowY = revealY(local, rowStart, 10, 10);
          return (
            <div
              key={line.label}
              style={{
                display: "flex",
                fontFamily: theme.fontMono,
                fontSize,
                whiteSpace: "pre",
                opacity: rowOpacity,
                transform: `translateY(${rowY}px)`,
              }}
            >
              <span style={{ color: theme.muted, width: labelWidth, flexShrink: 0 }}>
                {line.label} :
              </span>
              <span style={{ color: line.valueColor ?? theme.text }}>
                {line.value}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
};

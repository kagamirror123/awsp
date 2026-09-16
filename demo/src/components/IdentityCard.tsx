import React from "react";
import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { theme } from "../theme";
import { revealOpacity, revealY } from "../lib/anim";

export interface IdentityCardProps {
  start: number;
  profile: string;
  account: string;
  userId: string;
  arn: string;
  width?: number;
  fontSize?: number;
}

/**
 * `renderIdentityCard`(internal/awsp/runner.go)と同じ体裁の "🪪 AWS Caller Identity" カード。
 */
export const IdentityCard: React.FC<IdentityCardProps> = ({
  start,
  profile,
  account,
  userId,
  arn,
  width = 980,
  fontSize = 26,
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

  const rows: Array<[string, string]> = [
    ["🔐 Profile", profile],
    ["🧾 Account", account],
    ["👤 UserId ", userId],
    ["🌍 ARN    ", arn],
  ];

  return (
    <div style={{ opacity, transform: `scale(${scale})`, transformOrigin: "left top" }}>
      <div
        style={{
          fontFamily: theme.fontMono,
          fontSize: fontSize * 0.95,
          fontWeight: 700,
          color: theme.cyan,
          marginBottom: 14,
        }}
      >
        🪪 AWS Caller Identity
      </div>
      <div
        style={{
          width,
          border: `1px solid ${theme.blue}66`,
          borderRadius: 14,
          background: theme.panelAlt,
          padding: "26px 32px",
          display: "flex",
          flexDirection: "column",
          gap: 12,
        }}
      >
        {rows.map(([label, value], i) => {
          const rowStart = 8 + i * 3;
          const rowOpacity = revealOpacity(local, rowStart, 10);
          const rowY = revealY(local, rowStart, 10, 10);
          return (
            <div
              key={label}
              style={{
                display: "flex",
                fontFamily: theme.fontMono,
                fontSize,
                whiteSpace: "pre",
                opacity: rowOpacity,
                transform: `translateY(${rowY}px)`,
              }}
            >
              <span style={{ color: theme.muted, width: 230, flexShrink: 0 }}>
                {label} :
              </span>
              <span style={{ color: theme.text }}>{value}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
};

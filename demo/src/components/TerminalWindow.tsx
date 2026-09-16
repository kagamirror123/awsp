import React from "react";
import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { theme } from "../theme";

export interface TerminalWindowProps {
  title?: string;
  width?: number;
  height?: number;
  fontSize?: number;
  children?: React.ReactNode;
  /** 出現アニメーションを省略する(既に画面にある想定のとき)。 */
  static?: boolean;
}

/**
 * タイトルバー付きの角丸ターミナルウィンドウ。実画面のスクショは使わず React で描く。
 */
export const TerminalWindow: React.FC<TerminalWindowProps> = ({
  title = "~/repos/awsp",
  width = 1360,
  height = 760,
  fontSize = 28,
  children,
  static: isStatic = false,
}) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const intro = isStatic
    ? 1
    : spring({
        frame,
        fps,
        config: { damping: 20, stiffness: 170, mass: 0.8 },
        durationInFrames: 24,
      });

  const opacity = isStatic
    ? 1
    : interpolate(frame, [0, 14], [0, 1], {
        extrapolateLeft: "clamp",
        extrapolateRight: "clamp",
      });
  const scale = isStatic ? 1 : interpolate(intro, [0, 1], [0.96, 1]);
  const y = isStatic ? 0 : interpolate(intro, [0, 1], [18, 0]);

  return (
    <div
      style={{
        width,
        height,
        borderRadius: 16,
        overflow: "hidden",
        background: theme.panel,
        boxShadow:
          "0 40px 100px rgba(0,0,0,0.55), 0 0 0 1px rgba(255,255,255,0.05)",
        display: "flex",
        flexDirection: "column",
        fontFamily: theme.fontMono,
        opacity,
        transform: `translateY(${y}px) scale(${scale})`,
      }}
    >
      {/* Chrome */}
      <div
        style={{
          height: 52,
          flexShrink: 0,
          background: theme.chrome,
          display: "flex",
          alignItems: "center",
          padding: "0 22px",
          gap: 10,
          borderBottom: `1px solid ${theme.borderSoft}`,
        }}
      >
        <Light color="#ff5f57" />
        <Light color="#febc2e" />
        <Light color="#28c840" />
        <div
          style={{
            flex: 1,
            textAlign: "center",
            color: theme.muted,
            fontSize: 17,
            fontFamily: theme.fontMono,
          }}
        >
          {title}
        </div>
      </div>

      {/* Body */}
      <div
        style={{
          flex: 1,
          position: "relative",
          padding: "30px 40px",
          fontSize,
          color: theme.text,
          overflow: "hidden",
        }}
      >
        {children}
      </div>
    </div>
  );
};

const Light: React.FC<{ color: string }> = ({ color }) => (
  <div
    style={{
      width: 14,
      height: 14,
      borderRadius: "50%",
      background: color,
      opacity: 0.9,
    }}
  />
);

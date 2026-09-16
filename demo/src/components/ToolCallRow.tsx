import React from "react";
import { useCurrentFrame } from "remotion";
import { theme } from "../theme";
import { revealOpacity, revealY } from "../lib/anim";

export interface ToolCallRowProps {
  start: number;
  kind: "mcp" | "shell";
  label: string;
  resultText?: string;
  resultColor?: string;
  resultAt?: number;
  fontSize?: number;
  width?: number;
}

/** Claude Code 風のツール呼び出し行(折りたたみチップ)+ 結果ピル。 */
export const ToolCallRow: React.FC<ToolCallRowProps> = ({
  start,
  kind,
  label,
  resultText,
  resultColor = theme.green,
  resultAt,
  fontSize = 22,
  width = 640,
}) => {
  const frame = useCurrentFrame();
  const local = frame - start;
  if (local < -5) return null;
  const opacity = revealOpacity(local, 0, 10);
  const y = revealY(local, 0, 10, 12);

  const showResult = resultAt !== undefined && frame >= resultAt;
  const resultOpacity = showResult ? revealOpacity(frame, resultAt!, 10) : 0;
  const resultY = showResult ? revealY(frame, resultAt!, 10, 8) : 0;

  return (
    <div style={{ opacity, transform: `translateY(${y}px)`, width }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 12,
          fontFamily: theme.fontMono,
          fontSize,
          padding: "12px 18px",
          borderRadius: 10,
          background: theme.panelAlt,
          border: `1px solid ${theme.border}`,
          borderLeft: `3px solid ${kind === "mcp" ? theme.blue : theme.yellow}`,
        }}
      >
        <span>{kind === "mcp" ? "🧩" : "💻"}</span>
        <span style={{ color: theme.textDim }}>{label}</span>
      </div>
      {resultText && (
        <div
          style={{
            marginTop: 10,
            marginLeft: 18,
            display: "inline-block",
            opacity: resultOpacity,
            transform: `translateY(${resultY}px)`,
            fontFamily: theme.fontMono,
            fontSize: fontSize * 0.85,
            padding: "6px 14px",
            borderRadius: 999,
            background: `${resultColor}1f`,
            color: resultColor,
            border: `1px solid ${resultColor}66`,
          }}
        >
          {resultText}
        </div>
      )}
    </div>
  );
};

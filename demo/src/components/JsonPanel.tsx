import React from "react";
import { useCurrentFrame } from "remotion";
import { theme } from "../theme";
import { revealOpacity } from "../lib/anim";

export interface JsonLine {
  at: number;
  content: React.ReactNode;
  indent?: number;
}

export interface JsonPanelProps {
  title: string;
  lines: JsonLine[];
  width?: number;
  height?: number;
  fontSize?: number;
}

/** MCP のやり取りを模した右側の JSON ビュー。行ごとに `at` フレームでフェードインする。 */
export const JsonPanel: React.FC<JsonPanelProps> = ({
  title,
  lines,
  width = 620,
  height = 760,
  fontSize = 19,
}) => {
  const frame = useCurrentFrame();

  return (
    <div
      style={{
        width,
        height,
        borderRadius: 16,
        overflow: "hidden",
        background: theme.panel,
        border: `1px solid ${theme.border}`,
        display: "flex",
        flexDirection: "column",
        fontFamily: theme.fontMono,
      }}
    >
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
          color: theme.muted,
          fontSize: 16,
        }}
      >
        <span style={{ color: theme.yellow }}>▤</span>
        <span>{title}</span>
      </div>
      <div style={{ flex: 1, padding: "24px 26px", overflow: "hidden" }}>
        {lines.map((line, i) => {
          const opacity = revealOpacity(frame, line.at, 8);
          return (
            <div
              key={i}
              style={{
                opacity,
                fontSize,
                lineHeight: 1.7,
                whiteSpace: "pre",
                paddingLeft: (line.indent ?? 0) * 22,
              }}
            >
              {line.content}
            </div>
          );
        })}
      </div>
    </div>
  );
};

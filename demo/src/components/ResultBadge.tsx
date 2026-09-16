import React from "react";
import { useCurrentFrame } from "remotion";
import { theme } from "../theme";
import { revealOpacity, revealY } from "../lib/anim";

export interface ResultBadgeProps {
  start: number;
  text: string;
  color?: string;
  fontSize?: number;
  indent?: number;
}

/** ツール呼び出し結果の小さなピル(独立した flex 子要素として並び順を確定させる)。 */
export const ResultBadge: React.FC<ResultBadgeProps> = ({
  start,
  text,
  color = theme.green,
  fontSize = 20,
  indent = 18,
}) => {
  const frame = useCurrentFrame();
  const local = frame - start;
  if (local < -5) return null;
  const opacity = revealOpacity(local, 0, 10);
  const y = revealY(local, 0, 10, 8);

  return (
    <div
      style={{
        marginLeft: indent,
        opacity,
        transform: `translateY(${y}px)`,
        display: "inline-block",
        fontFamily: theme.fontMono,
        fontSize,
        padding: "6px 14px",
        borderRadius: 999,
        background: `${color}1f`,
        color,
        border: `1px solid ${color}66`,
      }}
    >
      {text}
    </div>
  );
};

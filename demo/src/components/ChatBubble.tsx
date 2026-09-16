import React from "react";
import { useCurrentFrame } from "remotion";
import { theme } from "../theme";
import { revealOpacity, revealY } from "../lib/anim";

export interface ChatBubbleProps {
  start: number;
  align?: "left" | "right";
  text: string;
  tone?: "user" | "assistant" | "thinking";
  fontSize?: number;
  maxWidth?: number;
}

export const ChatBubble: React.FC<ChatBubbleProps> = ({
  start,
  align = "left",
  text,
  tone = "assistant",
  fontSize = 24,
  maxWidth = 560,
}) => {
  const frame = useCurrentFrame();
  const local = frame - start;
  if (local < -5) return null;
  const opacity = revealOpacity(local, 0, 12);
  const y = revealY(local, 0, 12, 14);

  const isUser = tone === "user";
  const isThinking = tone === "thinking";

  return (
    <div
      style={{
        display: "flex",
        justifyContent: align === "right" ? "flex-end" : "flex-start",
        opacity,
        transform: `translateY(${y}px)`,
      }}
    >
      <div
        style={{
          maxWidth,
          fontFamily: theme.fontSans,
          fontSize,
          lineHeight: 1.55,
          padding: isUser ? "14px 20px" : "0",
          borderRadius: 14,
          background: isUser ? theme.panelAlt : "transparent",
          border: isUser ? `1px solid ${theme.border}` : "none",
          color: isThinking ? theme.muted : theme.text,
          fontStyle: isThinking ? "italic" : "normal",
        }}
      >
        {!isUser && !isThinking && (
          <span style={{ color: theme.cyan, fontWeight: 700, marginRight: 10 }}>
            ●
          </span>
        )}
        {text}
      </div>
    </div>
  );
};

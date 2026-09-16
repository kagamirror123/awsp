import React from "react";
import { useCurrentFrame } from "remotion";
import { theme } from "../theme";
import { Caret } from "./remocn/caret";

const DEFAULT_CHARS_PER_FRAME = 1.6;

/** テキストをタイプするのに必要なフレーム数(呼び出し側が後続要素の開始フレームを計算するために使う)。 */
export function typingFrames(
  text: string,
  charsPerFrame = DEFAULT_CHARS_PER_FRAME,
): number {
  return Math.ceil(text.length / charsPerFrame);
}

export interface TypedLineProps {
  text: string;
  start: number;
  prompt?: string;
  charsPerFrame?: number;
  color?: string;
  promptColor?: string;
  fontSize?: number;
  /** タイプ完了後もカーソルを点滅させ続ける。 */
  keepCaret?: boolean;
}

/**
 * `$ awsp status` のようなコマンド行を 1 文字ずつタイプさせる。
 * remocn の Caret をカーソルに使う。
 */
export const TypedLine: React.FC<TypedLineProps> = ({
  text,
  start,
  prompt = "$ ",
  charsPerFrame = DEFAULT_CHARS_PER_FRAME,
  color = theme.text,
  promptColor = theme.cyan,
  fontSize,
  keepCaret = false,
}) => {
  const frame = useCurrentFrame();
  if (frame < start) return null;
  const local = Math.max(0, frame - start);
  const visibleChars = Math.min(text.length, Math.floor(local * charsPerFrame));
  const shown = text.slice(0, visibleChars);
  const finishedTyping = visibleChars >= text.length;
  const startedTyping = frame >= start;

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        fontFamily: theme.fontMono,
        fontSize,
        whiteSpace: "pre",
      }}
    >
      <span style={{ color: promptColor, marginRight: 14 }}>{prompt}</span>
      <span style={{ color }}>{shown}</span>
      {startedTyping && (!finishedTyping || keepCaret) && (
        <Caret
          color={color}
          width={fontSize ? fontSize * 0.5 : 14}
          height={fontSize ? fontSize * 0.95 : 30}
          radius={2}
          marginLeft={4}
          blink={finishedTyping}
          blinkPerSecond={1.6}
        />
      )}
    </div>
  );
};

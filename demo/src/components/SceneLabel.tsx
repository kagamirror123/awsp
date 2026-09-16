import React from "react";
import { Sequence } from "remotion";
import { theme } from "../theme";
import { MaskRevealUp } from "./remocn/mask-reveal-up";

/**
 * 各シーン共通の左上ラベル(例: "02 / 認証")。remocn の MaskRevealUp で統一感を出す。
 */
export const SceneLabel: React.FC<{ index: string; title: string; from?: number }> = ({
  index,
  title,
  from = 6,
}) => {
  return (
    <div
      style={{
        position: "absolute",
        top: 56,
        left: 72,
        width: 700,
        height: 60,
      }}
    >
      <Sequence from={from} layout="none">
        <MaskRevealUp
          text={`${index}  ${title}`}
          fontSize={30}
          fontWeight={700}
          color={theme.muted}
          distance={16}
          align="left"
        />
      </Sequence>
    </div>
  );
};

import React from "react";
import { Sequence } from "remotion";
import { SceneFade } from "../components/SceneFade";
import { Wordmark } from "../components/Wordmark";
import { MarkerHighlight } from "../components/remocn/marker-highlight";
import { theme } from "../theme";

export const Scene1Title: React.FC = () => {
  return (
    <SceneFade noFadeIn>
      <div
        style={{
          position: "absolute",
          inset: 0,
          background:
            `radial-gradient(60% 55% at 50% 42%, ${theme.cyan}14 0%, transparent 70%), ` +
            `radial-gradient(120% 90% at 50% 100%, #0d1420 0%, ${theme.bg} 60%)`,
        }}
      />

      <div style={{ position: "absolute", top: 330, left: 0, right: 0, height: 300 }}>
        <Wordmark fontSize={210} />
      </div>

      <div style={{ position: "absolute", top: 660, left: 0, right: 0, height: 120 }}>
        <Sequence from={22} layout="none">
          <MarkerHighlight
            before="AWS プロファイル切り替え CLI が、"
            highlight="AI ネイティブ"
            after="に"
            fontSize={44}
            fontWeight={600}
            baseColor={theme.textDim}
            highlightedTextColor="#04241f"
            markerColor={theme.green}
          />
        </Sequence>
      </div>
    </SceneFade>
  );
};

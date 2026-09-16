import React from "react";
import { interpolate, Sequence, useCurrentFrame } from "remotion";
import { SceneFade } from "../components/SceneFade";
import { TypedLine } from "../components/TypedLine";
import { Wordmark } from "../components/Wordmark";
import { StaggeredFadeUp } from "../components/remocn/staggered-fade-up";
import { theme } from "../theme";

const INFO_FADE_START = 46;
const INFO_FADE_DUR = 10;
const WORDMARK_FROM = 48;

export const Scene6Outro: React.FC = () => {
  const frame = useCurrentFrame();
  const infoOpacity = interpolate(
    frame,
    [INFO_FADE_START, INFO_FADE_START + INFO_FADE_DUR],
    [1, 0],
    { extrapolateLeft: "clamp", extrapolateRight: "clamp" },
  );

  return (
    <SceneFade>
      <div
        style={{
          position: "absolute",
          inset: 0,
          background: `radial-gradient(60% 55% at 50% 55%, ${theme.cyan}12 0%, transparent 70%)`,
        }}
      />

      <div style={{ position: "absolute", top: 400, left: 0, right: 0, opacity: infoOpacity }}>
        <div style={{ display: "flex", justifyContent: "center" }}>
          <div
            style={{
              borderRadius: 12,
              border: `1px solid ${theme.border}`,
              background: theme.panelAlt,
              padding: "18px 32px",
            }}
          >
            <TypedLine
              text="claude mcp add awsp -- awsp mcp"
              start={6}
              charsPerFrame={3}
              fontSize={32}
              keepCaret
            />
          </div>
        </div>

        <div style={{ position: "absolute", top: 90, left: 0, right: 0, height: 80 }}>
          <Sequence from={26} layout="none">
            <StaggeredFadeUp
              text="github.com/kagamirror123/awsp"
              fontSize={30}
              fontWeight={500}
              color={theme.textDim}
              staggerDelay={0}
            />
          </Sequence>
        </div>
      </div>

      <div style={{ position: "absolute", top: 330, left: 0, right: 0, height: 300 }}>
        <Sequence from={WORDMARK_FROM} layout="none">
          <Wordmark fontSize={190} />
        </Sequence>
      </div>
    </SceneFade>
  );
};

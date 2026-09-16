import React from "react";
import { Sequence } from "remotion";
import { SceneFade } from "../components/SceneFade";
import { SceneLabel } from "../components/SceneLabel";
import { PrincipleCard } from "../components/PrincipleCard";
import { CodeIcon } from "../components/remocn/icon-code";
import { EyeOffIcon } from "../components/remocn/icon-eye-off";
import { ShieldIcon } from "../components/remocn/icon-shield";
import { theme } from "../theme";

const CARD_W = 500;
const CARD_H = 300;
const GAP = 60;
const START_X = (1920 - (CARD_W * 3 + GAP * 2)) / 2;
const TOP = 430;

export const Scene5Principles: React.FC = () => {
  return (
    <SceneFade>
      <SceneLabel index="04" title="設計の要点" from={4} />

      <div style={{ position: "absolute", left: START_X, top: TOP, display: "flex", gap: GAP }}>
        <div style={{ width: CARD_W, height: CARD_H }}>
          <PrincipleCard
            start={16}
            accent={theme.cyan}
            title={"aws CLI に依存しない\nPKCE を Go で内製"}
            icon={
              <Sequence from={10} layout="none">
                <CodeIcon size={34} color={theme.cyan} animation="both" />
              </Sequence>
            }
          />
        </div>
        <div style={{ width: CARD_W, height: CARD_H }}>
          <PrincipleCard
            start={40}
            accent={theme.yellow}
            title={"トークン値は\nどこにも出さない"}
            icon={
              <Sequence from={34} layout="none">
                <EyeOffIcon size={34} color={theme.yellow} animation="both" />
              </Sequence>
            }
          />
        </div>
        <div style={{ width: CARD_W, height: CARD_H }}>
          <PrincipleCard
            start={64}
            accent={theme.green}
            title={"AWS_CONFIG_FILE で\n人間と AI の権限を分ける"}
            icon={
              <Sequence from={58} layout="none">
                <ShieldIcon size={34} color={theme.green} animation="both" />
              </Sequence>
            }
          />
        </div>
      </div>
    </SceneFade>
  );
};

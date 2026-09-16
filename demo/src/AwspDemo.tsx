import React from "react";
import { AbsoluteFill, Sequence } from "remotion";
import { SCENES, theme } from "./theme";
import { Scene1Title } from "./scenes/Scene1Title";
import { Scene2Human } from "./scenes/Scene2Human";
import { Scene3Auth } from "./scenes/Scene3Auth";
import { Scene4Agent } from "./scenes/Scene4Agent";
import { Scene5Principles } from "./scenes/Scene5Principles";
import { Scene6Outro } from "./scenes/Scene6Outro";

export const AwspDemo: React.FC = () => {
  return (
    <AbsoluteFill style={{ background: theme.bg }}>
      <Sequence from={SCENES.title.start} durationInFrames={SCENES.title.duration} name="01 タイトル">
        <Scene1Title />
      </Sequence>
      <Sequence from={SCENES.human.start} durationInFrames={SCENES.human.duration} name="02 人間の流れ">
        <Scene2Human />
      </Sequence>
      <Sequence from={SCENES.auth.start} durationInFrames={SCENES.auth.duration} name="03 認証">
        <Scene3Auth />
      </Sequence>
      <Sequence from={SCENES.agent.start} durationInFrames={SCENES.agent.duration} name="04 エージェントの流れ">
        <Scene4Agent />
      </Sequence>
      <Sequence from={SCENES.principles.start} durationInFrames={SCENES.principles.duration} name="05 設計の要点">
        <Scene5Principles />
      </Sequence>
      <Sequence from={SCENES.outro.start} durationInFrames={SCENES.outro.duration} name="06 締め">
        <Scene6Outro />
      </Sequence>
    </AbsoluteFill>
  );
};

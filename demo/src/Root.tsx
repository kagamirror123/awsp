import React from "react";
import { Composition } from "remotion";
import "./style.css";
import { AwspDemo } from "./AwspDemo";
import { FPS, TOTAL_DURATION } from "./theme";

export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Composition
        id="AwspDemo"
        component={AwspDemo}
        durationInFrames={TOTAL_DURATION}
        fps={FPS}
        width={1920}
        height={1080}
      />
    </>
  );
};

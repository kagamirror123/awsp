import React from "react";
import { theme } from "../theme";
import { SoftBlurIn } from "./remocn/soft-blur-in";

export const Wordmark: React.FC<{ fontSize?: number }> = ({ fontSize = 210 }) => {
  return (
    <div
      style={{
        position: "absolute",
        inset: 0,
        filter: `drop-shadow(0 0 60px ${theme.cyan}55)`,
      }}
    >
      <SoftBlurIn
        text="awsp"
        fontSize={fontSize}
        fontWeight={800}
        color={theme.text}
        blur={16}
      />
    </div>
  );
};

import React from "react";
import { interpolate, useCurrentFrame } from "remotion";
import { SceneFade } from "../components/SceneFade";
import { SceneLabel } from "../components/SceneLabel";
import { TerminalWindow } from "../components/TerminalWindow";
import { TypedLine } from "../components/TypedLine";
import { StatusTable } from "../components/StatusTable";
import { Card } from "../components/Card";
import { BrowserApprovalCard } from "../components/BrowserApprovalCard";
import { theme } from "../theme";
import { revealOpacity, revealY } from "../lib/anim";

const STATUS_TABLE_START = 40;
const GUIDANCE_START = 58;
const LOGIN_CMD_START = 112;
const OUT1_START = 134;
const OUT2_START = 140;
const CARD_APPEAR = 146;
const CARD_CLICK = 172;
const CARD_EXIT = 192;
const SWAP_AT = 176;
const LOGIN_RESULT_START = 180;

export const Scene3Auth: React.FC = () => {
  const frame = useCurrentFrame();

  const guidanceOpacity = interpolate(frame, [0, 100, 112], [1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const guidanceIn = revealOpacity(frame, GUIDANCE_START, 10);
  const guidanceY = revealY(frame, GUIDANCE_START, 10, 8);

  const out1Opacity = revealOpacity(frame, OUT1_START, 10);
  const out2Opacity = revealOpacity(frame, OUT2_START, 10);

  return (
    <SceneFade>
      <SceneLabel index="02" title="認証 ── awsp status / login" />
      <div style={{ position: "absolute", left: 160, top: 170 }}>
        <TerminalWindow title="~/repos/awsp" width={1180} height={700} fontSize={28}>
          <div style={{ position: "absolute", top: 0, left: 0 }}>
            <TypedLine text="status" prompt="$ awsp " start={16} fontSize={28} />
          </div>

          <div style={{ position: "absolute", top: 54, left: 0 }}>
            <StatusTable
              start={STATUS_TABLE_START}
              session="corp"
              state={{ values: ["🔴 error", "🟢 ok"], at: SWAP_AT }}
              expires="09-16 04:30"
              remaining={{ values: ["-11h", "8h"], at: SWAP_AT }}
              profiles="16"
              width={1080}
              fontSize={24}
            />
          </div>

          <div
            style={{
              position: "absolute",
              top: 200,
              left: 0,
              opacity: guidanceOpacity * guidanceIn,
              transform: `translateY(${guidanceY}px)`,
              fontFamily: theme.fontMono,
              fontSize: 24,
              color: theme.yellow,
            }}
          >
            → awsp login --sso-session corp
          </div>

          <div style={{ position: "absolute", top: 200, left: 0 }}>
            <TypedLine
              text="login --sso-session corp"
              prompt="$ awsp "
              start={LOGIN_CMD_START}
              fontSize={28}
            />
          </div>

          <div
            style={{
              position: "absolute",
              top: 246,
              left: 0,
              opacity: out1Opacity,
              fontFamily: theme.fontMono,
              fontSize: 26,
              color: theme.cyan,
            }}
          >
            🌐 ブラウザで承認してください…
          </div>
          <div
            style={{
              position: "absolute",
              top: 284,
              left: 0,
              opacity: out2Opacity,
              fontFamily: theme.fontMono,
              fontSize: 22,
              color: theme.muted,
            }}
          >
            認可 URL をブラウザで開いています
          </div>

          <div style={{ position: "absolute", top: 332, left: 0 }}>
            <Card
              start={LOGIN_RESULT_START}
              heading="✅ AWS SSO Login"
              headingColor={theme.green}
              width={760}
              fontSize={24}
              labelWidth={210}
              lines={[
                { label: "🔐 Session", value: "corp" },
                { label: "📶 State  ", value: "ok", valueColor: theme.green },
                { label: "⏳ Expires", value: "2026-09-16T20:30:00+09:00" },
              ]}
            />
          </div>
        </TerminalWindow>
      </div>

      <BrowserApprovalCard
        appearAt={CARD_APPEAR}
        clickAt={CARD_CLICK}
        exitAt={CARD_EXIT}
        right={110}
        top={260}
      />
    </SceneFade>
  );
};

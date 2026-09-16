import React from "react";
import { interpolate, useCurrentFrame } from "remotion";
import { SceneFade } from "../components/SceneFade";
import { SceneLabel } from "../components/SceneLabel";
import { TerminalWindow } from "../components/TerminalWindow";
import { TypedLine } from "../components/TypedLine";
import { ProfileListPane, ProfileRow, SelectionStep } from "../components/ProfileListPane";
import { IdentityCard } from "../components/IdentityCard";
import { theme } from "../theme";

const ROWS: ProfileRow[] = [
  { name: "billing", region: "us-west-2", account: "123456789012", role: "AdministratorAccess", state: "ok", remaining: "52m" },
  { name: "security", region: "us-west-2", account: "123456789012", role: "AdministratorAccess", state: "ok", remaining: "23m" },
  { name: "dev", region: "us-west-2", account: "123456789012", role: "AdministratorAccess", state: "ok", remaining: "1h58m" },
  { name: "staging", region: "us-west-2", account: "123456789012", role: "AdministratorAccess", state: "warning", remaining: "3m" },
  { name: "prod", region: "us-west-2", account: "123456789012", role: "AIAgentReadOnlyAccess", state: "ok", remaining: "6m" },
  { name: "sandbox", region: "us-west-2", account: "123456789012", role: "AdministratorAccess", state: "error", remaining: "-2h" },
];

const STEPS: SelectionStep[] = [
  { index: 0, at: 54 },
  { index: 1, at: 92, key: "down" },
  { index: 2, at: 130, key: "down" },
  { index: 2, at: 156, key: "enter" },
];

const PANE_FADE_OUT_START = 176;
const IDENTITY_START = 194;
const SUCCESS_START = 230;

export const Scene2Human: React.FC = () => {
  const frame = useCurrentFrame();

  const commandOpacity = interpolate(frame, [0, 6, 44, 52], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  const paneOpacity = interpolate(
    frame,
    [0, PANE_FADE_OUT_START, PANE_FADE_OUT_START + 12],
    [1, 1, 0],
    { extrapolateLeft: "clamp", extrapolateRight: "clamp" },
  );

  const successOpacity = interpolate(frame, [SUCCESS_START, SUCCESS_START + 10], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const successY = interpolate(frame, [SUCCESS_START, SUCCESS_START + 10], [10, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <SceneFade>
      <SceneLabel index="01" title="人間の流れ ── awsp" />
      <div style={{ position: "absolute", left: 160, top: 210 }}>
        <TerminalWindow title="~/repos/awsp" width={1600} height={640} fontSize={30}>
          <div style={{ position: "absolute", inset: "0 40px auto 0", top: 0, opacity: commandOpacity }}>
            <TypedLine text="awsp" start={16} fontSize={30} />
          </div>

          <div style={{ position: "absolute", inset: 0, opacity: paneOpacity }}>
            <ProfileListPane start={54} rows={ROWS} steps={STEPS} width={1420} fontSize={24} />
          </div>

          <div style={{ position: "absolute", inset: 0 }}>
            <IdentityCard
              start={IDENTITY_START}
              profile="dev"
              account="123456789012"
              userId="AROADBQP57FAAEXAMPLE:you@example.com"
              arn="arn:aws:sts::123456789012:assumed-role/AdministratorAccess/you@example.com"
              width={1180}
              fontSize={26}
            />
          </div>

          <div
            style={{
              position: "absolute",
              left: 0,
              top: 296,
              opacity: successOpacity,
              transform: `translateY(${successY}px)`,
              fontFamily: theme.fontMono,
              fontSize: 28,
              color: theme.green,
              fontWeight: 700,
            }}
          >
            ✅ Set AWS_PROFILE=dev
          </div>
        </TerminalWindow>
      </div>
    </SceneFade>
  );
};

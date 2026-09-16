import React from "react";
import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { theme } from "../theme";
import { revealOpacity, revealY } from "../lib/anim";

export type RowState = "ok" | "warning" | "error";

export interface ProfileRow {
  name: string;
  region: string;
  account: string;
  role: string;
  state: RowState;
  remaining: string;
}

export interface SelectionStep {
  index: number;
  at: number;
  /** このステップで矢印キー表示を出す(既定 true)。最後の Enter ステップでは false + confirm を使う。 */
  key?: "down" | "enter";
}

export interface ProfileListPaneProps {
  start: number;
  rows: ProfileRow[];
  steps: SelectionStep[];
  width?: number;
  fontSize?: number;
}

const STATE_DOT: Record<RowState, string> = {
  ok: "🟢",
  warning: "🟡",
  error: "🔴",
};
const STATE_COLOR: Record<RowState, string> = {
  ok: theme.green,
  warning: theme.yellow,
  error: theme.red,
};

const ROW_H = 58;
const HEADER_H = 56;

function activeStepIndex(steps: SelectionStep[], frame: number): number {
  let idx = 0;
  for (let i = 0; i < steps.length; i++) {
    if (frame >= steps[i].at) idx = i;
  }
  return idx;
}

function dip(frame: number, at: number, dur: number): number {
  if (frame <= at) {
    return interpolate(frame, [at - 4, at], [1, 0.1], {
      extrapolateLeft: "clamp",
      extrapolateRight: "clamp",
    });
  }
  return interpolate(frame, [at, at + dur], [0.1, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
}

export const ProfileListPane: React.FC<ProfileListPaneProps> = ({
  start,
  rows,
  steps,
  width = 1280,
  fontSize = 24,
}) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const local = frame - start;
  if (local < -5) return null;

  const opacity = interpolate(local, [0, 16], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  const curStepIdx = activeStepIndex(steps, local);
  const curStep = steps[curStepIdx];
  const prevStep = steps[curStepIdx - 1] ?? steps[0];
  const stepLocal = local - curStep.at;
  const stepProgress =
    curStepIdx === 0
      ? 1
      : spring({
          frame: stepLocal,
          fps,
          config: { damping: 14, stiffness: 190, mass: 0.6 },
          durationInFrames: 14,
        });
  const fromIndex = curStepIdx === 0 ? curStep.index : prevStep.index;
  const barIndex = interpolate(stepProgress, [0, 1], [fromIndex, curStep.index]);
  const barY = HEADER_H + barIndex * ROW_H;

  const detailDip = dip(local, curStep.at, 14);
  const activeRow = rows[curStep.index];

  // 矢印/Enter キーキャップの点滅
  const keyFlash = interpolate(local, [curStep.at, curStep.at + 6, curStep.at + 16], [0, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const showKeyFlash = curStep.key && local >= curStep.at - 2;

  const leftW = width * 0.56;
  const rightW = width - leftW - 36;

  return (
    <div style={{ opacity, display: "flex", gap: 36, width }}>
      {/* Left: list */}
      <div style={{ width: leftW }}>
        <div
          style={{
            fontFamily: theme.fontMono,
            fontSize: fontSize * 0.95,
            color: theme.cyan,
            fontWeight: 700,
            height: HEADER_H,
            display: "flex",
            alignItems: "center",
          }}
        >
          📚 Available profiles
        </div>
        <div style={{ position: "relative" }}>
          {/* highlight bar */}
          <div
            style={{
              position: "absolute",
              left: -14,
              right: -14,
              top: barY - HEADER_H,
              height: ROW_H - 6,
              borderRadius: 10,
              background: `${theme.blue}26`,
              border: `1px solid ${theme.blue}80`,
            }}
          />
          {rows.map((row, i) => {
            const rowOpacity = revealOpacity(local, 4 + i * 3, 12);
            const rowY = revealY(local, 4 + i * 3, 12, 12);
            const isCurrent = i === curStep.index;
            return (
              <div
                key={row.name}
                style={{
                  position: "relative",
                  height: ROW_H,
                  display: "flex",
                  alignItems: "center",
                  gap: 14,
                  fontFamily: theme.fontMono,
                  fontSize,
                  opacity: rowOpacity,
                  transform: `translateY(${rowY}px)`,
                  color: isCurrent ? theme.text : theme.textDim,
                }}
              >
                <span style={{ fontSize: fontSize * 0.75 }}>{STATE_DOT[row.state]}</span>
                <span style={{ width: 300, fontWeight: isCurrent ? 700 : 400 }}>
                  {row.name}
                </span>
                <span style={{ color: theme.muted, fontSize: fontSize * 0.8 }}>
                  {row.remaining}
                </span>
              </div>
            );
          })}
        </div>
        {showKeyFlash && (
          <div
            style={{
              marginTop: 22,
              opacity: keyFlash,
              display: "inline-flex",
              alignItems: "center",
              gap: 10,
              fontFamily: theme.fontMono,
              fontSize: fontSize * 0.8,
              color: theme.blue,
            }}
          >
            <KeyCap label={curStep.key === "enter" ? "Enter" : "↓"} />
            <span style={{ color: theme.muted }}>
              {curStep.key === "enter" ? "選択を確定" : "次の profile へ"}
            </span>
          </div>
        )}
      </div>

      {/* Right: detail */}
      <div
        style={{
          width: rightW,
          opacity: detailDip,
          border: `1px solid ${theme.border}`,
          borderRadius: 14,
          background: theme.panelAlt,
          padding: "26px 30px",
          height: HEADER_H + rows.length * ROW_H - 8,
          fontFamily: theme.fontMono,
          fontSize: fontSize * 0.85,
        }}
      >
        <div style={{ color: theme.cyan, fontWeight: 700, marginBottom: 18 }}>
          {activeRow.name}
        </div>
        <DetailRow label="Region" value={activeRow.region} />
        <DetailRow label="Account" value={activeRow.account} />
        <DetailRow label="Role" value={activeRow.role} />
        <DetailRow
          label="State"
          value={`${STATE_DOT[activeRow.state]} ${activeRow.state}`}
          valueColor={STATE_COLOR[activeRow.state]}
        />
        <DetailRow label="Remaining" value={activeRow.remaining} />
      </div>
    </div>
  );
};

const DetailRow: React.FC<{ label: string; value: string; valueColor?: string }> = ({
  label,
  value,
  valueColor,
}) => (
  <div style={{ display: "flex", marginBottom: 12 }}>
    <span style={{ width: 150, color: theme.muted, flexShrink: 0 }}>{label}</span>
    <span style={{ color: valueColor ?? theme.text }}>{value}</span>
  </div>
);

const KeyCap: React.FC<{ label: string }> = ({ label }) => (
  <span
    style={{
      border: `1px solid ${theme.blue}`,
      borderRadius: 6,
      padding: "4px 10px",
      background: `${theme.blue}20`,
    }}
  >
    {label}
  </span>
);

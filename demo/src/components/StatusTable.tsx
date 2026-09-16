import React from "react";
import { interpolate, useCurrentFrame } from "remotion";
import { theme } from "../theme";
import { ValueSwap } from "./remocn/value-swap";

export type CellValue = string | { values: string[]; at: number };

const COLUMNS = ["Session", "State", "Expires", "Remaining", "Profiles"] as const;

export interface StatusTableProps {
  start: number;
  session: string;
  state: CellValue;
  expires: string;
  remaining: CellValue;
  profiles: string;
  width?: number;
  fontSize?: number;
}

const renderCell = (value: CellValue, fontSize: number, color: string) => {
  if (typeof value === "string") {
    return <span>{value}</span>;
  }
  return (
    <ValueSwap
      values={value.values}
      at={value.at}
      duration={14}
      style={{ fontSize, color, fontFamily: theme.fontMono }}
    />
  );
};

/**
 * `awsp status` の表(Session / State / Expires / Remaining / Profiles)を再現する。
 * State / Remaining は CellValue に {values,at} を渡すと ValueSwap でその場で切り替わる。
 */
export const StatusTable: React.FC<StatusTableProps> = ({
  start,
  session,
  state,
  expires,
  remaining,
  profiles,
  width = 1180,
  fontSize = 26,
}) => {
  const frame = useCurrentFrame();
  const local = frame - start;
  if (local < -5) return null;

  const opacity = interpolate(local, [0, 14], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const y = interpolate(local, [0, 14], [16, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  const widths = [1.6, 1.1, 1.5, 1.1, 1];
  const totalW = widths.reduce((a, b) => a + b, 0);

  return (
    <div
      style={{
        width,
        opacity,
        transform: `translateY(${y}px)`,
        fontFamily: theme.fontMono,
        fontSize,
        border: `1px solid ${theme.border}`,
        borderRadius: 14,
        overflow: "hidden",
        background: theme.panelAlt,
      }}
    >
      <div
        style={{
          display: "flex",
          background: theme.chrome,
          borderBottom: `1px solid ${theme.border}`,
          padding: "16px 28px",
          color: theme.cyan,
          fontWeight: 700,
        }}
      >
        {COLUMNS.map((c, i) => (
          <div key={c} style={{ flex: widths[i] / totalW }}>
            {c}
          </div>
        ))}
      </div>
      <div style={{ display: "flex", padding: "20px 28px", color: theme.text }}>
        <div style={{ flex: widths[0] / totalW }}>{session}</div>
        <div style={{ flex: widths[1] / totalW }}>
          {renderCell(state, fontSize, theme.text)}
        </div>
        <div style={{ flex: widths[2] / totalW, color: theme.textDim }}>
          {expires}
        </div>
        <div style={{ flex: widths[3] / totalW }}>
          {renderCell(remaining, fontSize, theme.text)}
        </div>
        <div style={{ flex: widths[4] / totalW, color: theme.textDim }}>
          {profiles}
        </div>
      </div>
    </div>
  );
};

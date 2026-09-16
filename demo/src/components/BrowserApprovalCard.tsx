import React from "react";
import { interpolate, Sequence, useCurrentFrame } from "remotion";
import { theme } from "../theme";
import { SimulatedCursor } from "./remocn/simulated-cursor";

export interface BrowserApprovalCardProps {
  appearAt: number;
  clickAt: number;
  exitAt: number;
  width?: number;
  height?: number;
  right?: number;
  top?: number;
}

const CURSOR_TRAVEL = 24;
const BUTTON_X = 392;
const BUTTON_Y = 306;

/**
 * 「ブラウザで承認してください」のブラウザ風カード。remocn の SimulatedCursor で
 * 「Allow access」をクリックする様子を描く。
 */
export const BrowserApprovalCard: React.FC<BrowserApprovalCardProps> = ({
  appearAt,
  clickAt,
  exitAt,
  width = 520,
  height = 360,
  right = 90,
  top = 300,
}) => {
  const frame = useCurrentFrame();
  if (frame < appearAt - 2) return null;

  const inX = interpolate(frame, [appearAt, appearAt + 16], [70, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const inOpacity = interpolate(frame, [appearAt, appearAt + 14], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const outX = interpolate(frame, [exitAt, exitAt + 14], [0, 70], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const outOpacity = interpolate(frame, [exitAt, exitAt + 14], [1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  const approved = interpolate(frame, [clickAt, clickAt + 4], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  const cursorStart = clickAt - CURSOR_TRAVEL;

  return (
    <div
      style={{
        position: "absolute",
        right,
        top,
        width,
        height,
        opacity: inOpacity * outOpacity,
        transform: `translateX(${inX + outX}px)`,
      }}
    >
      <div
        style={{
          position: "relative",
          width,
          height,
          borderRadius: 16,
          overflow: "hidden",
          background: "#f3f4f6",
          boxShadow: "0 30px 90px rgba(0,0,0,0.6), 0 0 0 1px rgba(255,255,255,0.08)",
          fontFamily: theme.fontSans,
        }}
      >
        {/* browser chrome */}
        <div
          style={{
            height: 44,
            background: "#e5e7eb",
            display: "flex",
            alignItems: "center",
            padding: "0 14px",
            gap: 8,
          }}
        >
          <Dot color="#ff5f57" />
          <Dot color="#febc2e" />
          <Dot color="#28c840" />
          <div
            style={{
              marginLeft: 10,
              flex: 1,
              background: "#ffffff",
              borderRadius: 8,
              padding: "6px 12px",
              fontSize: 14,
              color: "#374151",
              display: "flex",
              alignItems: "center",
              gap: 6,
            }}
          >
            <span>🔒</span>
            <span>oidc.us-west-2.amazonaws.com</span>
          </div>
        </div>

        {/* body */}
        <div style={{ padding: "26px 28px", color: "#111827" }}>
          <div style={{ fontSize: 20, fontWeight: 700, marginBottom: 8 }}>
            AWS IAM Identity Center
          </div>
          <div style={{ fontSize: 15, color: "#4b5563", lineHeight: 1.6 }}>
            <b>awsp</b> があなたの AWS アカウントへの
            <br />
            アクセスを要求しています
          </div>

          <div
            style={{
              marginTop: 30,
              display: "flex",
              justifyContent: "flex-end",
              gap: 12,
            }}
          >
            <button
              style={{
                border: "1px solid #d1d5db",
                background: "#ffffff",
                color: "#374151",
                borderRadius: 8,
                padding: "12px 18px",
                fontSize: 15,
              }}
            >
              Cancel
            </button>
            <div
              style={{
                position: "relative",
                width: 200,
                height: 48,
              }}
            >
              <button
                style={{
                  position: "absolute",
                  inset: 0,
                  border: "none",
                  borderRadius: 8,
                  padding: "12px 18px",
                  fontSize: 15,
                  fontWeight: 700,
                  color: "#04211c",
                  background: `linear-gradient(180deg, ${theme.greenBright}, ${theme.green})`,
                  opacity: 1 - approved,
                }}
              >
                Allow access
              </button>
              <div
                style={{
                  position: "absolute",
                  inset: 0,
                  borderRadius: 8,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  gap: 8,
                  fontSize: 15,
                  fontWeight: 700,
                  color: "#04211c",
                  background: `linear-gradient(180deg, ${theme.greenBright}, ${theme.green})`,
                  opacity: approved,
                }}
              >
                ✅ Allowed
              </div>
            </div>
          </div>
        </div>

        <Sequence from={cursorStart} layout="none">
          <SimulatedCursor
            color="#111827"
            size={30}
            points={[
              { x: width - 40, y: height + 40, hold: 1 },
              { x: BUTTON_X, y: BUTTON_Y, hold: 26, click: true },
            ]}
          />
        </Sequence>
      </div>
    </div>
  );
};

const Dot: React.FC<{ color: string }> = ({ color }) => (
  <div style={{ width: 11, height: 11, borderRadius: "50%", background: color }} />
);

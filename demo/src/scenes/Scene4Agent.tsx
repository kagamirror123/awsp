import React from "react";
import { interpolate, useCurrentFrame } from "remotion";
import { SceneFade } from "../components/SceneFade";
import { SceneLabel } from "../components/SceneLabel";
import { ChatBubble } from "../components/ChatBubble";
import { ToolCallRow } from "../components/ToolCallRow";
import { ResultBadge } from "../components/ResultBadge";
import { JsonPanel, JsonLine } from "../components/JsonPanel";
import { BrowserApprovalCard } from "../components/BrowserApprovalCard";
import { theme } from "../theme";
import { revealOpacity, revealY } from "../lib/anim";

const PANEL_TOP = 156;
const PANEL_HEIGHT = 840;
const LEFT_X = 115;
const LEFT_W = 1010;
const RIGHT_X = 1175;
const RIGHT_W = 630;

const CARD_APPEAR = 150;
const CARD_CLICK = 178;
const CARD_EXIT = 202;

const K = (s: string) => <span style={{ color: theme.cyan }}>{s}</span>;
const Sv = (s: string) => <span style={{ color: theme.green }}>{s}</span>;
const N = (s: string) => <span style={{ color: theme.yellow }}>{s}</span>;
const P = (s: string) => <span style={{ color: theme.textDim }}>{s}</span>;
const C = (s: string) => <span style={{ color: theme.muted }}>{s}</span>;

const JSON_LINES: JsonLine[] = [
  { at: 46, content: C("// auth_status") },
  { at: 66, content: P("{") },
  { at: 66, indent: 1, content: <>{K('"schemaVersion"')}{P(": ")}{N("1")}{P(",")}</> },
  { at: 66, indent: 1, content: <>{K('"overall"')}{P(": ")}{Sv('"error"')}</> },
  { at: 66, content: P("}") },
  { at: 118, content: C("// login") },
  { at: 118, indent: 1, content: <>{K('"profile"')}{P(": ")}{Sv('"prod"')}</> },
  { at: 184, content: P("{") },
  { at: 184, indent: 1, content: <>{K('"schemaVersion"')}{P(": ")}{N("1")}{P(",")}</> },
  { at: 184, indent: 1, content: <>{K('"status"')}{P(": ")}{Sv('"ok"')}{P(",")}</> },
  { at: 184, indent: 1, content: <>{K('"session"')}{P(": ")}{Sv('"corp"')}</> },
  { at: 184, content: P("}") },
];

const BUCKETS = ["prod-datalake", "prod-app-logs", "prod-backups"];

export const Scene4Agent: React.FC = () => {
  const frame = useCurrentFrame();

  const introOpacity = interpolate(frame, [0, 16], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const leftX = interpolate(frame, [0, 18], [-40, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const rightX = interpolate(frame, [0, 18], [40, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <SceneFade>
      <SceneLabel index="03" title="エージェントの流れ ── awsp mcp" />

      {/* Left: chat */}
      <div
        style={{
          position: "absolute",
          left: LEFT_X + leftX,
          top: PANEL_TOP,
          width: LEFT_W,
          height: PANEL_HEIGHT,
          opacity: introOpacity,
          borderRadius: 16,
          background: theme.panel,
          border: `1px solid ${theme.border}`,
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
        }}
      >
        <div
          style={{
            height: 56,
            flexShrink: 0,
            display: "flex",
            alignItems: "center",
            gap: 12,
            padding: "0 24px",
            background: theme.chrome,
            borderBottom: `1px solid ${theme.borderSoft}`,
            fontFamily: theme.fontSans,
            fontSize: 20,
            color: theme.textDim,
            fontWeight: 700,
          }}
        >
          <span style={{ color: theme.cyan }}>✳</span> Claude Code
        </div>
        <div
          style={{
            flex: 1,
            padding: "26px 30px",
            display: "flex",
            flexDirection: "column",
            gap: 18,
          }}
        >
          <ChatBubble start={20} align="right" tone="user" fontSize={23} text="本番の S3 バケット一覧を見せて" />
          <ToolCallRow start={46} kind="mcp" label='awsp › auth_status' fontSize={21} width={520} />
          <ResultBadge start={66} text='overall: "error"' color={theme.red} />
          <ChatBubble start={92} tone="thinking" fontSize={21} text="SSO セッションが失効しています。ログインします" />
          <ToolCallRow start={118} kind="mcp" label='awsp › login { profile: "prod" }' fontSize={21} width={620} />
          <ChatBubble start={144} tone="thinking" fontSize={21} text="🌐 ブラウザで承認待ち…" />
          <ResultBadge start={CARD_CLICK + 6} text='status: "ok"' color={theme.green} />
          <ToolCallRow start={206} kind="shell" label="bash › aws s3 ls --profile prod" fontSize={21} width={620} />
          <BucketList start={232} />
          <ChatBubble
            start={272}
            tone="assistant"
            fontSize={22}
            text="本番の S3 バケットは 3 件でした。"
          />
        </div>
      </div>

      {/* Right: MCP JSON */}
      <div
        style={{
          position: "absolute",
          left: RIGHT_X + rightX,
          top: PANEL_TOP,
          opacity: introOpacity,
        }}
      >
        <JsonPanel title="awsp mcp ・ stdio" lines={JSON_LINES} width={RIGHT_W} height={PANEL_HEIGHT} fontSize={19} />
      </div>

      <BrowserApprovalCard
        appearAt={CARD_APPEAR}
        clickAt={CARD_CLICK}
        exitAt={CARD_EXIT}
        right={140}
        top={420}
      />
    </SceneFade>
  );
};

const BucketList: React.FC<{ start: number }> = ({ start }) => {
  const frame = useCurrentFrame();
  const local = frame - start;
  if (local < -5) return null;
  return (
    <div
      style={{
        marginLeft: 18,
        fontFamily: theme.fontMono,
        fontSize: 20,
        color: theme.textDim,
        display: "flex",
        flexDirection: "column",
        gap: 4,
      }}
    >
      {BUCKETS.map((b, i) => {
        const o = revealOpacity(local, i * 6, 10);
        const y = revealY(local, i * 6, 10, 6);
        return (
          <div key={b} style={{ opacity: o, transform: `translateY(${y}px)` }}>
            {b}
          </div>
        );
      })}
    </div>
  );
};

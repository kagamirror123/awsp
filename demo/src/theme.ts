// 配色は internal/ui/palette.go (Lip Gloss v2 の ANSI パレット) に合わせた近似値。
// success=10(green) info=12(blue) warning=11(yellow) error=9(red) heading=14(cyan) border=12 muted=8(gray)
export const theme = {
  bg: "#0a0c10",
  bgSoft: "#0d1016",
  panel: "#13161d",
  panelAlt: "#191d26",
  chrome: "#1b1f28",
  border: "#2a3040",
  borderSoft: "#232838",
  text: "#e7eaf0",
  textDim: "#c2c8d4",
  muted: "#7c8494",
  cyan: "#3fd7e0",
  cyanBright: "#7bf0f5",
  green: "#3ddc84",
  greenBright: "#7dfab0",
  yellow: "#f5c451",
  red: "#f76a6a",
  blue: "#5da8f7",
  fontMono:
    '"SF Mono", "JetBrains Mono", Menlo, Consolas, "Liberation Mono", monospace',
  fontSans:
    '"Hiragino Sans", "Hiragino Kaku Gothic ProN", "Noto Sans JP", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
} as const;

export const FPS = 30;

// シーンの秒数(目安)とフレーム範囲。合計 38s = 1140f。
export const SCENES = {
  title: { start: 0, duration: 90 }, // 0-3s
  human: { start: 90, duration: 270 }, // 3-12s
  auth: { start: 360, duration: 210 }, // 12-19s
  agent: { start: 570, duration: 330 }, // 19-30s
  principles: { start: 900, duration: 150 }, // 30-35s
  outro: { start: 1050, duration: 90 }, // 35-38s
} as const;

export const TOTAL_DURATION =
  SCENES.outro.start + SCENES.outro.duration; // 1140

// シーン全体の入り/抜けフェード(フレーム数)。背景色が共通なのでディゾルブに見える。
export const SCENE_FADE = 10;

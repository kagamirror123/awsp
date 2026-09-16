# awsp デモ動画

`awsp` の紹介デモ動画(1920x1080 / 30fps / 38秒 / 音声なし)を作る Remotion プロジェクト。
端末・対話 UI・ブラウザ承認カード・チャット画面はすべて React (Remotion) で描いており、実画面のスクショは使っていない。

## レンダー手順

```bash
npm install
npm run render   # out/awsp-demo.mp4 を書き出す
npm run stills    # out/still-<秒>s.png (2/8/15/24/32/37s) を書き出す
```

`npm start` で Remotion Studio が起動し、タイムラインを見ながら確認できる。

- Node v24 / npm 前提。pnpm は使っていない。
- ffmpeg は同梱の `@remotion/compositor-*` を使うので別途インストール不要。
- 出力(`out/`)と `node_modules/` は `.gitignore` 済み。MP4・静止画はリポジトリに含めない。

## シーン構成と秒数

| # | 秒 | シーン | 内容 |
|---|---|---|---|
| 1 | 0–3s | タイトル | `awsp` ワードマーク(remocn `SoftBlurIn`)+ サブコピーを `MarkerHighlight` でハイライト |
| 2 | 3–12s | 人間の流れ | `awsp` 型入力 → 対話 UI モック(左: profile 一覧 + 認証状態・残り時間、右: 詳細)→ ↓↓ で `dev` 選択 → Enter → identity カード → `✅ Set AWS_PROFILE=dev` |
| 3 | 12–19s | 認証 | `awsp status`(State 🔴 error / Remaining `-11h`)→ `awsp login --sso-session corp` → ブラウザ承認カード(remocn `SimulatedCursor` でクリック)→ `✅ AWS SSO Login` カード → 表が 🟢 ok に切り替わる(remocn `ValueSwap`) |
| 4 | 19–30s | エージェントの流れ(山場) | 左に Claude Code 風チャット、右に MCP の JSON。`auth_status`(error)→ `login`(ブラウザ承認待ち → 承認)→ `aws s3 ls --profile prod` → バケット一覧 |
| 5 | 30–35s | 設計の要点 | 3 枚のカードが順に登場(remocn アイコン `CodeIcon` / `EyeOffIcon` / `ShieldIcon`) |
| 6 | 35–38s | 締め | `claude mcp add awsp -- awsp mcp` と `github.com/kagamirror123/awsp` → ワードマークで終了 |

フレーム範囲は `src/theme.ts` の `SCENES` に定義している(30fps 換算)。

## 使用した remocn コンポーネント

`https://remocn.dev/r/<name>.json` の registry API から取得し、`src/components/remocn/` にそのまま配置(一部は背景色やインポートパスのみ調整、ファイル冒頭にコメントで明記)。

- `soft-blur-in` — ワードマークのブラーリビール(タイトル/締め)
- `marker-highlight` — サブコピーのハイライト(タイトル)
- `mask-reveal-up` — 各シーン左上のラベル(`SceneLabel`)。`align` prop を追加(既定は upstream と同じ `center`)
- `staggered-fade-up` — 締めの URL 表示
- `simulated-cursor` — ブラウザ承認カードのクリック演出
- `value-swap` — `awsp status` の State / Remaining の切り替え
- `caret` — 端末のタイプ入力カーソル点滅
- `icon-code` / `icon-eye-off` / `icon-shield`(+ 共有ライブラリ `icons-core`)— 設計の要点カードのアイコン

`npx shadcn@latest add @remocn/<component>` は `components.json` 作成の確認プロンプトが対話専用で、この環境(非 TTY)では応答できず自動化できなかった。
そのため registry の JSON をそのまま取得してファイルを配置している(内容は CLI が生成するものと同一)。

remocn の「シーン全体を差し替えるトランジション」系コンポーネントはテキスト差し替え用(1つのフレーズを別のフレーズに置き換える用途)で、
シーン間の切り替えには使えなかったため、シーン間はすべて `useCurrentFrame()` / `interpolate()` による自前のフェード(`SceneFade`)にしている。

## その他の構成

- `src/components/` — 端末ウィンドウ・identity カード・status 表・対話 UI モック・ブラウザ承認カード・チャット部品など自作コンポーネント
- `src/scenes/` — 6 シーンの実装
- `src/theme.ts` — 配色(`internal/ui/palette.go` の ANSI パレットに準拠)とフレーム範囲
- 表示している profile 名・アカウント ID・ARN 等はすべてダミー(`123456789012` / `you@example.com`)。実際の値は使っていない

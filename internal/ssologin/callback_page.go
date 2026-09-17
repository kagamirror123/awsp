package ssologin

import (
	"html"
	"strings"
)

// コールバック画面の見た目の定義
// 外部リソース(CDN のフォント・CSS・画像)は一切読み込まない
// 認可直後のブラウザはネットワークが不安定なこともあり 取得失敗で崩れるのを避けるため
// 配色は端末側の awsp(internal/ui)と揃え ライト/ダークは prefers-color-scheme で切り替える
const (
	callbackAccentSuccess = "#3fb950"
	callbackAccentError   = "#f85149"
	callbackAccentNeutral = "#58a6ff"

	// callbackIconCheck は成功時のチェックマーク(線を描くアニメーションを掛ける)
	callbackIconCheck = `<path class="stroke" d="M19 33.5 28 42.5 45 22.5"/>`
	// callbackIconCross は失敗時のバツ印
	callbackIconCross = `<path class="stroke" d="M22 22 42 42M42 22 22 42"/>`
	// callbackIconAlert は想定外リクエスト時の感嘆符(縦線と点)
	// 点は長さ 0 の線分に丸いキャップを付けて描く
	callbackIconAlert = `<path class="stroke" d="M32 19.5v17"/><path class="stroke" d="M32 44.5v.01"/>`
)

// callbackPage はブラウザへ返す 1 枚完結の HTML を組み立てる
// accent はアイコンの色 icon は SVG のパス heading と detail は本文 note は補足(空なら出さない)
// 引数はすべて awsp 自身が持つ固定文字列で 外部入力をそのまま渡さない
func callbackPage(accent string, icon string, heading string, detail string, note string) string {
	noteBlock := ""
	if note != "" {
		noteBlock = `<div class="note">` + html.EscapeString(note) + `</div>`
	}

	return strings.NewReplacer(
		"{{ACCENT}}", accent,
		"{{ICON}}", icon,
		"{{HEADING}}", html.EscapeString(heading),
		"{{DETAIL}}", html.EscapeString(detail),
		"{{NOTE}}", noteBlock,
	).Replace(callbackPageTemplate)
}

const callbackPageTemplate = `<!doctype html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex">
<title>awsp</title>
<style>
:root{
  color-scheme: dark light;
  --accent:{{ACCENT}};
  --bg:#0b0f14; --card:#131a23; --border:rgba(255,255,255,.07);
  --text:#e6edf3; --muted:#8b949e; --glow:rgba(94,234,212,.10);
  --shadow:0 24px 60px rgba(0,0,0,.45);
}
@media (prefers-color-scheme: light){
  :root{
    --bg:#f4f6f8; --card:#fff; --border:rgba(16,22,26,.10);
    --text:#1f2328; --muted:#5b6570; --glow:rgba(45,212,191,.16);
    --shadow:0 20px 48px rgba(16,22,26,.10);
  }
}
*{box-sizing:border-box}
html,body{height:100%}
body{
  margin:0; padding:24px; display:grid; place-items:center;
  background:var(--bg);
  background-image:radial-gradient(58% 55% at 50% 36%, var(--glow), transparent 72%);
  color:var(--text);
  font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","Hiragino Sans","Noto Sans JP",sans-serif;
  -webkit-font-smoothing:antialiased;
}
.card{
  width:100%; max-width:440px; padding:44px 36px 30px; text-align:center;
  background:var(--card); border:1px solid var(--border); border-radius:20px;
  box-shadow:var(--shadow);
  animation:rise .5s cubic-bezier(.2,.7,.3,1) both;
}
@keyframes rise{from{opacity:0;transform:translateY(14px) scale(.985)}}
.mark{
  font-size:32px; font-weight:800; letter-spacing:-.025em; line-height:1;
  margin-bottom:30px; color:var(--text);
  text-shadow:0 0 28px var(--glow);
}
.icon{width:62px;height:62px;display:block;margin:0 auto 22px}
.icon .ring{fill:var(--accent);opacity:.14}
.icon .stroke{
  fill:none; stroke:var(--accent); stroke-width:5.5;
  stroke-linecap:round; stroke-linejoin:round;
  stroke-dasharray:72;
  animation:draw .5s .2s cubic-bezier(.2,.7,.3,1) both;
}
h1{margin:0 0 12px; font-size:20px; font-weight:650; letter-spacing:.01em}
p{margin:0; font-size:14.5px; line-height:1.75; color:var(--muted); text-wrap:pretty}
.note{
  margin-top:24px; padding:12px 14px; border-radius:10px;
  background:rgba(127,127,127,.09);
  font-family:ui-monospace,SFMono-Regular,Menlo,monospace;
  font-size:12.5px; line-height:1.6; color:var(--muted); word-break:break-all;
}
.hint{
  margin-top:26px; padding-top:18px; border-top:1px solid var(--border);
  font-size:12.5px; color:var(--muted);
}
/* 既定を描画済み(dashoffset 0)にして アニメーションが走らない環境でも印が消えないようにする */
@keyframes draw{from{stroke-dashoffset:72}to{stroke-dashoffset:0}}
@media (prefers-reduced-motion: reduce){
  *{animation:none !important}
}
</style>
</head>
<body>
<main class="card">
  <div class="mark">awsp</div>
  <svg class="icon" viewBox="0 0 64 64" role="img" aria-label="{{HEADING}}">
    <circle class="ring" cx="32" cy="32" r="30"/>
    {{ICON}}
  </svg>
  <h1>{{HEADING}}</h1>
  <p>{{DETAIL}}</p>
  {{NOTE}}
  <div class="hint">このタブは閉じて構いません</div>
</main>
</body>
</html>`

// successCallbackPage は認可応答を受け付けた画面。トークン交換・保存の成功はまだ確定していない。
func successCallbackPage() string {
	return callbackPage(
		callbackAccentSuccess,
		callbackIconCheck,
		"承認を受け付けました",
		"ログイン処理を続けています。ターミナルに戻り、完了したことを確認してください。",
		"",
	)
}

// failureCallbackPage は認可が拒否されたときの画面
// 失敗の理由は認可サーバーからの入力なのでブラウザには出さず ターミナル側のエラーで伝える
func failureCallbackPage() string {
	return callbackPage(
		callbackAccentError,
		callbackIconCross,
		"認証に失敗しました",
		"ターミナルの表示を確認してください。",
		"",
	)
}

// unexpectedCallbackPage は code も error も無いリクエストへの画面
func unexpectedCallbackPage() string {
	return callbackPage(
		callbackAccentNeutral,
		callbackIconAlert,
		"想定外のリクエストです",
		"認可コードの無いリクエストが届きました。",
		"",
	)
}

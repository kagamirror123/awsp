# AGENTS

このリポジトリで作業するエージェント向けのガイド

## 目的

- `awsp` は AWS プロファイル切り替え CLI
- `~/.aws/config`(`AWS_CONFIG_FILE` があればそちら)の profile を選択して利用する
- caller identity は AWS SDK v2 で確認する
- SSO セッション開始は aws CLI を使わず `internal/ssologin`(SDK 内製、既定は Authorization Code + PKCE)で行う

## リポジトリ前提

- 公開リポジトリ: `kagamirror123/awsp`
- ドキュメントや設定で旧オーナーや組織名を残さない

## ブランチ運用

- デフォルトブランチ: `main`
- 作業ブランチ: `feature/*`
- 例: `feature/readme-toc` `feature/list-ui-polish`

## コミット規約

- Semantic Commit Messages に準拠
- 形式: `type(scope)?: メッセージ`
- メッセージ本文は日本語で書く
- 代表的な type: `feat` `fix` `docs` `refactor` `test` `chore` `ci`
- 例: `feat(list): テーブルヘッダの視認性を改善`

## 技術スタック

- Go 1.26+
- Cobra + Fang(ヘルプ・エラー表示・version)
- slog
- AWS SDK for Go v2
- Bubble Tea v2 / Bubbles v2 / Lip Gloss v2(D8)
- Taskfile
- mise

## 実装ルール

- 型は厳密に保つ
- 可読性優先で責務を小さく分割する
- docstring とコメントは日本語で書く
- 不要な抽象化は避ける
- エラーメッセージは利用者が次の行動を判断できる内容にする
- Cobra の `Short` / `Long` は日本語で書き始める(Fang が先頭の単語を Title Case にするため「AWS」が「Aws」になる)。設計番号(D1 など)は利用者向け文言に出さない

## 主要ディレクトリ

- `cmd`: CLI エントリとサブコマンド
- `internal/awsp`: ユースケースと JSON 出力の型
- `internal/awscli`: SDK 呼び出し(caller identity 確認)
- `internal/awsconfig`: `~/.aws/config` の読み取り(profile と sso-session)
- `internal/ssocache`: `~/.aws/sso/cache` `~/.aws/cli/cache` の状態モデル(読み取り専用 ネットワーク禁止)
- `internal/ssologin`: SSO ログインフローの実装。既定は Authorization Code + PKCE、`--use-device-code` で device authorization flow に切り替え
- `internal/prompt`: TUI
- `internal/ui`: CLI 出力(カード・表・行・バッジ)の描画を Lip Gloss v2 に一本化する共通部品(D8)
- `internal/mcp`: MCP サーバー(stdio)。auth_status / list_profiles / whoami / login の 4 ツール

## 主要コマンド

- `awsp [profile]`
- `awsp current`
- `awsp list --json`
- `awsp status [--json]`
- `awsp preflight`
- `awsp login [profile]`
- `awsp whoami [profile]`(省略時は AWS_PROFILE)
- `awsp mcp`
- `awsp init zsh|bash|fish`
- `awsp completion zsh`

## 開発コマンド

- `mise install`
- `task tools`
- `task fmt`
- `task lint`
- `task test`
- `task check`

## 変更時のチェック

1. `task fmt`
2. `task lint`
3. `task test`
4. `task build`

## 備考

- `Taskfile` は `mise` がある場合に自動で `mise exec` を使う
- `awsp init zsh|bash|fish` で関数を導入すると `awsp <profile>` 実行時に親シェルへ反映される
- `--shell` はその関数が内部で使う出力モード。値なしは posix(export / unset)、`--shell=fish` は set -gx / set -e
- 配布は darwin / linux のみ(Windows は D25 で対象外)

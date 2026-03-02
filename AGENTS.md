# AGENTS

このリポジトリで作業するエージェント向けのガイド

## 目的

- `awsp` は AWS プロファイル切り替え CLI
- `~/.aws/config` の profile を選択して利用する
- caller identity は AWS SDK v2 で確認する
- SSO セッション開始は `aws sso login` を使う

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
- Cobra
- slog
- AWS SDK for Go v2
- Bubble Tea + Bubbles + Lip Gloss
- Taskfile
- mise

## 実装ルール

- 型は厳密に保つ
- 可読性優先で責務を小さく分割する
- docstring とコメントは日本語で書く
- 不要な抽象化は避ける
- エラーメッセージは利用者が次の行動を判断できる内容にする

## 主要ディレクトリ

- `cmd`: CLI エントリとサブコマンド
- `internal/awsp`: ユースケース
- `internal/awscli`: SDK 呼び出しと SSO ログイン
- `internal/awsconfig`: `~/.aws/config` の読み取り
- `internal/prompt`: TUI

## 主要コマンド

- `awsp [profile]`
- `awsp current`
- `awsp list --json`
- `awsp init zsh`
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
- `awsp init zsh` で関数を導入すると `awsp <profile>` 実行時に親シェルへ反映される
- `--shell` はその関数が内部で使う export / unset 出力モード

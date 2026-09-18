# Development

## Prerequisites

- [Task](https://taskfile.dev/)(`task` コマンド)
- `mise` 推奨、または Go 1.26.x

## セットアップと日常コマンド

```bash
mise install
task tools      # golangci-lint
task fmt
task lint
task test
task build      # ./bin/awsp
task check      # fmt + lint + test
```

## コードの置き場所

| ディレクトリ | 役割 |
|---|---|
| `cmd` | CLI エントリとサブコマンド |
| `internal/awsp` | ユースケースと JSON 出力の型 |
| `internal/awsconfig` | config の読み取り(profile と sso-session) |
| `internal/ssocache` | `~/.aws/sso/cache` と `~/.aws/cli/cache` の状態モデル(読み取り専用) |
| `internal/ssologin` | Authorization Code + PKCE と device code のログイン |
| `internal/awscli` | STS の呼び出し |
| `internal/mcp` | MCP サーバー(4 ツール) |
| `internal/ui` | Lip Gloss v2 の描画部品 |
| `internal/prompt` | Bubble Tea v2 の対話 UI |
| `demo` | 紹介動画の Remotion プロジェクト |

コーディング規約は [AGENTS.md](../AGENTS.md)、設計の決定と却下した案は [design.md](./design.md) にあります。

## デモ動画

```bash
cd demo
npm install
npm run render   # out/awsp-demo.mp4
npm run stills   # out/still-*.png
```

MP4 と静止画は git に入れません。

## リリース

```bash
task release-check
task release-snapshot
```

- CI: `main` push / Pull Request で format lint test
- CD: `v*` タグ push で GoReleaser が GitHub Release を作成し、Homebrew tap(`kagamirror123/homebrew-tap`)の cask を更新
- Dependabot の PR は CI 通過で自動マージされる。マージ後は定期実行の `auto-release.yml`(平日 10〜18 時 JST に毎時)が main の HEAD を見て、未タグかつ author が dependabot なら patch を自動タグしてリリースする。手動で起動するなら Actions の workflow_dispatch
- tap の更新には Secrets の `HOMEBREW_TAP_GITHUB_TOKEN`(homebrew-tap への contents: write を持つ fine-grained PAT)が必要

## Contributing

Issue / Pull Request を歓迎します。大きめの変更は先に Issue で方針共有してもらえると助かります。

1. `feature/*` ブランチを作る
2. 実装する
3. `task check` を通す
4. 変更内容を説明する Pull Request を作る

コミットメッセージは `type(scope): メッセージ` の形で、本文は日本語です。

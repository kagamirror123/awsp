# awsp

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev/)
[![Cobra](https://img.shields.io/badge/Cobra-1.10.2-00A3E0)](https://github.com/spf13/cobra)
[![AWS SDK for Go v2](https://img.shields.io/badge/AWS_SDK_v2-config%20%2F%20sts-FF9900?logo=amazon-aws)](https://github.com/aws/aws-sdk-go-v2)
[![Bubble Tea v2](https://img.shields.io/badge/Bubble_Tea-v2-FF75B7)](https://github.com/charmbracelet/bubbletea)
[![Fang](https://img.shields.io/badge/Fang-v1-874BFD)](https://github.com/charmbracelet/fang)
[![Lint](https://img.shields.io/badge/lint-golangci--lint-blue)](https://golangci-lint.run/)
[![CI](https://github.com/kagamirror123/awsp/actions/workflows/ci.yml/badge.svg)](https://github.com/kagamirror123/awsp/actions/workflows/ci.yml)
[![Release](https://github.com/kagamirror123/awsp/actions/workflows/release.yml/badge.svg)](https://github.com/kagamirror123/awsp/actions/workflows/release.yml)

ターミナルで AWS プロファイルを安全に切り替える CLI  
`~/.aws/config` を読み取り、対話 UI か `awsp <profile>` で選択し、その場で caller identity まで確認できます  
複数アカウント運用時の誤操作防止と切替速度の両立を狙ったツールです

> [!TIP]
> 最短導線は `Quick Start` の 1-3 を実行して `awsp` を叩くだけです

## 目次

- [Quick Start](#quick-start)
- [Usage](#usage)
- [AWS Config Example](#aws-config-example)
- [MCP サーバー](#mcp-サーバー)
- [Design Notes](#design-notes)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

---

## Quick Start

エンドユーザー向けの最短手順  
GitHub Releases のバイナリだけで使い始められます

1. バイナリをダウンロード
    - リリースページ: [Releases](https://github.com/kagamirror123/awsp/releases)
    - `raw` 生バイナリ と `tar.gz / zip` を配布
    - 例: macOS arm64 の生バイナリ

    ```bash
    AWSP_VERSION=0.6.0
    curl -fL -o /tmp/awsp "https://github.com/kagamirror123/awsp/releases/download/v${AWSP_VERSION}/awsp_${AWSP_VERSION}_darwin_arm64"
    ```

2. PATH に配置

    ```bash
    install -m 0755 /tmp/awsp /usr/local/bin/awsp
    ```

3. シェル連携を有効化

    ```bash
    echo 'eval "$(awsp init zsh)"' >> ~/.zshrc
    exec zsh
    ```

4. 動作確認

    ```bash
    awsp
    awsp current
    ```

> [!IMPORTANT]
> `awsp <profile>` の結果を親シェルに反映するには `awsp init zsh` の読み込みが必要です

---

## Usage

### Demo

`awsp list`(D9: 認証状態 / 残り時間 / 最終使用も一覧に表示)

```text
$ awsp list

📚 Available profiles
total=2  current=-

╭─────────┬───┬───────────────┬────────────────┬────────┬──────────────┬─────────────────────┬────────┬─────────┬─────────┬───────────╮
│ Current │ # │ Profile       │ Region         │ Auth   │ Account      │ Role                │ Source │ State   │ Expires │ Last used │
├─────────┼───┼───────────────┼────────────────┼────────┼──────────────┼─────────────────────┼────────┼─────────┼─────────┼───────────┤
│ .       │ 1 │ dev           │ us-west-2      │ sso    │ 123456789012 │ AdministratorAccess │ -      │ unknown │ -       │ -         │
│ .       │ 2 │ prod-readonly │ ap-northeast-1 │ static │ -            │ -                   │ dev    │ -       │ -       │ -         │
╰─────────┴───┴───────────────┴────────────────┴────────┴──────────────┴─────────────────────┴────────┴─────────┴─────────┴───────────╯
```

`awsp preflight`(SessionStart フック向けの 1 行確認)

```text
$ awsp preflight; echo exit=$?
awsp preflight: AWS SSO 未ログイン(corp)。AWS を使う前に 'awsp login --sso-session corp' を実行してください
exit=1
```

### 主要コマンド

```bash
# 対話選択
awsp

# 直接指定
awsp <profile>

# 現在の identity 確認
awsp current
awsp current --json

# プロファイル一覧
awsp list
awsp list --json

# AWS SSO セッションの認証状態
awsp status
awsp status --json
awsp status --grace 8h

# SessionStart フック向けの 1 行確認(exit code で成否を返す)
awsp preflight

# AWS SSO へログイン(既定: Authorization Code + PKCE、承認完了までブロック)
# ブラウザで承認すると自動で戻る --use-device-code で device code に切り替え可(組織側で無効な場合あり)
awsp login
awsp login dev
awsp login --sso-session corp
awsp login --no-browser
awsp login --use-device-code

# 指定 profile の caller identity を確認(自動ログインしない)
awsp whoami        # AWS_PROFILE の identity
awsp whoami dev    # profile を指定

# MCP サーバー(stdio)を起動 詳細は MCP サーバー の節を参照
awsp mcp


# 補完とシェル連携
awsp completion zsh
awsp init zsh

# nwrelay 経由でシェル連携する場合
eval "$(awsp init zsh --command 'NWRELAY_TARGET_BIN=awsp command nwrelay')"
```

<details>
<summary>オプション詳細</summary>

`--login-only`: profile は変更せずログイン状態だけ確認  
`--no-login`: caller identity / sso login を省略して反映処理のみ実施

`--shell`: `awsp init zsh` が内部利用する export / unset 出力モード

`init zsh --command`: 連携関数内で実行する awsp コマンドを差し替える

</details>

---

## AWS Config Example

`awsp` は `~/.aws/config` の `[profile ...]` を読み取ります

<details>
<summary>SSO の最小構成例</summary>

SSO の最小構成例:

```ini
[profile dev]
region = us-west-2
sso_session = corp
sso_account_id = 123456789012
sso_role_name = AdministratorAccess

[sso-session corp]
sso_start_url = https://example.awsapps.com/start
sso_region = us-west-2
sso_registration_scopes = sso:account:access
```

</details>

<details>
<summary>role + source_profile の例</summary>

role + source_profile の例:

```ini
[profile base]
region = ap-northeast-1
output = json

[profile prod-readonly]
region = ap-northeast-1
role_arn = arn:aws:iam::123456789012:role/ProdReadOnly
source_profile = base
```

</details>

---

## MCP サーバー

`awsp mcp` で stdio の MCP サーバーとして起動します  
コーディングエージェントが AWS の状態確認・ログインを行うための道具です

### 登録

Claude Code:

```bash
claude mcp add awsp -- awsp mcp
```

Codex(`~/.codex/config.toml` に追記):

```toml
[mcp_servers.awsp]
command = "awsp"
args = ["mcp"]
```

### ツール

| ツール | 用途 | ネットワーク |
|---|---|---|
| `auth_status` | AWS SSO セッションの有効性を確認(`awsp status` 相当) | 使わない(ローカルキャッシュのみ) |
| `list_profiles` | profile 一覧と認証状態・最終使用情報を取得(`awsp list --json` 相当) | 使わない(ローカルファイルのみ) |
| `whoami` | 指定 profile の caller identity を確認(自動ログインしない) | 使う(STS) |
| `login` | AWS SSO へログイン(既定: Authorization Code + PKCE)。承認完了まで待ち、タイムアウト時は認可 URL を返す(`use_device_code` 指定時は device code に切り替え、コードも返す) | 使う(SSO OIDC) |

> [!NOTE]
> profile の切り替えは人間のシェル(`awsp init zsh` の関数)が行います。MCP サーバーからは
> エージェント側の環境変数を変更できないため、エージェントは `list_profiles` で得た名前を
> `aws` コマンドの `--profile <name>` に渡して使います。トークン値・credentials はどのツールの
> 結果にも含まれません。

## Design Notes

- caller identity は AWS SDK for Go v2 で型安全に取得
- SSO セッション確立は aws CLI を使わず AWS SDK for Go v2(ssooidc)で Authorization Code + PKCE(既定)を自前実装。`--use-device-code` で device authorization flow に切り替え可(組織側で無効な場合あり)(D12)
- 書き出すトークンキャッシュ(`~/.aws/sso/cache`)は aws CLI / SDK と完全互換
- 状態確認(`status` / `preflight`)はローカルファイルのみを読み、ネットワークを使わない。ネットワークを使うのは `whoami` と `login` だけ
- トークン値・credentials はディスク・出力・ログに一切載せない
- `AWS_CONFIG_FILE` を尊重し、複数の config を切り替えて使えるようにする
- 描画は Lip Gloss v2 に一本化(`internal/ui`)。TUI は Bubble Tea v2 + Bubbles v2、CLI 外装(ヘルプ・エラー・version)は Fang(D8)
- 非 TTY 出力や `NO_COLOR` では ANSI 装飾を自動的に落とす(`colorprofile`)
- 詳細な設計判断は [`docs/design.md`](./docs/design.md) を参照

---

## Development

開発者向け情報はここだけ見れば進められるように整理しています

### Prerequisites

- [Task](https://taskfile.dev/) (`task` コマンド)
- `mise` 推奨 または Go 1.26.x

### セットアップ

```bash
mise install
task tools
```

### 日常コマンド

```bash
task fmt
task test
task check
task build
```

### リリース確認

```bash
task release-check
task release-snapshot
```

### CI/CD

- CI: `main` push / Pull Request で format lint test
- CD: `v*` タグ push で GoReleaser が GitHub Release を作成
- Release note: GitHub 自動生成をベースに必要なら手動編集

---

## Contributing

Issue / Pull Request を歓迎します  
大きめの変更は先に Issue で方針共有してもらえると助かります

1. ブランチ作成
2. 実装
3. 検証

    ```bash
    task check
    ```

4. 変更内容を説明する Pull Request を作成

---

## License

MIT License  
詳細は [LICENSE](./LICENSE) を参照

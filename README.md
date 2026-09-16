# awsp

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev/)
[![AWS SDK for Go v2](https://img.shields.io/badge/AWS_SDK_v2-sts%20%2F%20ssooidc-FF9900?logo=amazon-aws)](https://github.com/aws/aws-sdk-go-v2)
[![Bubble Tea v2](https://img.shields.io/badge/Bubble_Tea-v2-FF75B7)](https://github.com/charmbracelet/bubbletea)
[![Fang](https://img.shields.io/badge/Fang-v1-874BFD)](https://github.com/charmbracelet/fang)
[![MCP](https://img.shields.io/badge/MCP-server-000000)](https://modelcontextprotocol.io/)
[![CI](https://github.com/kagamirror123/awsp/actions/workflows/ci.yml/badge.svg)](https://github.com/kagamirror123/awsp/actions/workflows/ci.yml)
[![Release](https://github.com/kagamirror123/awsp/actions/workflows/release.yml/badge.svg)](https://github.com/kagamirror123/awsp/actions/workflows/release.yml)

ターミナルで AWS プロファイルを安全に切り替える CLI  
人間はシェルから、AI エージェントは MCP から、同じバイナリを使います

- **切り替え**: 一覧から選ぶか `awsp <profile>`。切り替えた瞬間に caller identity を確認して表示
- **認証**: SSO セッションの有効・失効をローカルで判定。失効していればブラウザで 1 クリック承認するだけのログイン
- **エージェント**: `awsp mcp` で `auth_status` / `list_profiles` / `whoami` / `login` の 4 ツール。`login` は人の承認を待ってから返る
- **土台**: aws CLI 不要。トークン値はどこにも出さない。`AWS_CONFIG_FILE` を尊重するので人間と AI で config を分けられる

<!--
デモ動画: demo/ の Remotion プロジェクトからレンダーした MP4 を GitHub の Issue か PR のコメント欄に
ドラッグ&ドロップし、得られた user-attachments の URL をここに貼る(README 上でインライン再生される)。
動画ファイル自体は git に入れない。
-->

## 目次

- [Quick Start](#quick-start)
- [使い方](#使い方)
  - [切り替える](#切り替える)
  - [認証状態とログイン](#認証状態とログイン)
  - [エージェントから使う](#エージェントから使う)
- [AWS Config Example](#aws-config-example)
- [Design Notes](#design-notes)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

---

## Quick Start

GitHub Releases のバイナリだけで使い始められます。aws CLI は不要です

1. バイナリをダウンロードして PATH に置く(例: macOS arm64)

    ```bash
    AWSP_VERSION=$(curl -fsSL https://api.github.com/repos/kagamirror123/awsp/releases/latest | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p')
    curl -fL -o /tmp/awsp "https://github.com/kagamirror123/awsp/releases/download/v${AWSP_VERSION}/awsp_${AWSP_VERSION}_darwin_arm64"
    install -m 0755 /tmp/awsp /usr/local/bin/awsp
    ```

    他の OS / アーキテクチャは [Releases](https://github.com/kagamirror123/awsp/releases) から

2. シェル連携を有効化(`awsp <profile>` の結果を親シェルに反映するために必要)

    ```bash
    echo 'eval "$(awsp init zsh)"' >> ~/.zshrc
    exec zsh
    ```

3. 使う

    ```bash
    awsp            # 一覧から選ぶ
    awsp dev        # 直接切り替え
    awsp status     # SSO セッションの状態
    ```

4. エージェントからも使うなら MCP サーバーとして登録

    ```bash
    claude mcp add awsp -- awsp mcp
    ```

---

## 使い方

### 切り替える

```bash
awsp                 # 対話 UI。文字を打つと絞り込み、↑↓ で移動、Enter で決定
awsp <profile>       # 直接切り替え。失効していれば自動でログインしてから反映
awsp "(unset)"       # AWS_PROFILE と静的認証情報を解除
awsp current         # いまの AWS_PROFILE の caller identity(失効していれば自動ログイン)
awsp whoami [profile]  # caller identity を確認するだけ。自動ログインしない。省略時は AWS_PROFILE
awsp list            # 一覧。認証状態と残り時間も見える
```

対話 UI は左に一覧、右に詳細。各行に認証状態と残り時間が付きます

```text
$ awsp list

📚 Available profiles
total=3  current=dev

╭──────────┬────────────────┬──────────────┬─────────────────────┬────────┬───────┬─────────╮
│ Profile  │ Region         │ Account      │ Role                │ Source │ State │ Expires │
├──────────┼────────────────┼──────────────┼─────────────────────┼────────┼───────┼─────────┤
│ ▶ dev    │ us-west-2      │ 123456789012 │ AdministratorAccess │ -      │ ok    │ 51m     │
│   legacy │ ap-northeast-1 │ -            │ -                   │ dev    │ 🪪    │ -       │
│   prod   │ ap-northeast-1 │ 210987654321 │ ReadOnlyAccess      │ -      │ ok    │ 51m     │
╰──────────┴────────────────┴──────────────┴─────────────────────┴────────┴───────┴─────────╯
```

切り替えると caller identity を確認して表示します

```text
$ awsp dev
╭─────────── 🪪 AWS Caller Identity ───────────╮
│ 🔐 Profile : dev                              │
│ 🧾 Account : 123456789012                     │
│ 👤 UserId  : AROA…:you@example.com            │
│ 🌍 ARN     : arn:aws:sts::123456789012:…      │
╰───────────────────────────────────────────────╯
✅ Set AWS_PROFILE=dev
```

表は端末幅に収まります。狭ければ優先度の低い列から落とし、パイプ先では装飾を付けません

### 認証状態とログイン

```bash
awsp status                  # sso-session ごとの有効・失効・残り時間。ネットワークは使わない
awsp login [profile]         # ブラウザで承認するだけ。有効なら何もしない
awsp login --sso-session corp
awsp login --no-browser      # URL を表示するだけ(自分で開く)
awsp login --use-device-code # device code 方式(組織側で無効な場合あり)
awsp preflight               # 1 行 + exit code。フックやスクリプト向け
```

```text
$ awsp status

🔐 AWS SSO Status (error)

╭─────────┬───────┬─────────────┬───────────┬──────────╮
│ Session │ State │ Expires     │ Remaining │ Profiles │
├─────────┼───────┼─────────────┼───────────┼──────────┤
│ corp    │ error │ 09-15 09:00 │ -1d       │ 2        │
╰─────────┴───────┴─────────────┴───────────┴──────────╯
❌ corp: 失効(1d 前)。'awsp login --sso-session corp' を実行してください

$ awsp preflight; echo exit=$?
awsp preflight: AWS SSO 失効(corp 1d 前)。AWS を使う前に 'awsp login --sso-session corp' を実行してください
exit=1
```

ログインは Authorization Code + PKCE を Go で内製しています。ブラウザで「Allow access」を押すと localhost に戻って自動で完了し、
書き出したトークンキャッシュは aws CLI や各言語の SDK がそのまま使います

状態は 4 つです

| 状態 | 意味 |
|---|---|
| `ok` | 有効 |
| `warning` | 期限切れだが refresh token があり、次に使うとき自動更新される見込み |
| `error` | 失効。`awsp login` が必要 |
| `unknown` | 未ログイン |

### エージェントから使う

`awsp mcp` は stdio の MCP サーバーです。Claude Code なら 1 行で登録できます

```bash
claude mcp add awsp -- awsp mcp
```

Codex は `~/.codex/config.toml` に追記します

```toml
[mcp_servers.awsp]
command = "awsp"
args = ["mcp"]
```

| ツール | 何をするか | ネットワーク |
|---|---|---|
| `auth_status` | SSO セッションの有効・失効。作業の最初に呼ぶ | 使わない |
| `list_profiles` | profile 一覧と account / role / 認証状態 | 使わない |
| `whoami` | 指定 profile の caller identity。自動ログインしない | STS |
| `login` | ログインを起こし、人がブラウザで承認するまで待ってから返る | SSO OIDC |

`login` の挙動

- 既に有効なら何もせず即 `status: "ok"`
- ブラウザを開けたら承認完了まで待つ(既定 5 分、`timeout_seconds` で変更)
- ブラウザを開けなかったか時間切れなら `status: "pending"` と認可 URL を返す。人が URL を開いて承認し、エージェントがもう一度 `login` を呼ぶと同じフローに合流して完了する
- 同じ sso-session への同時呼び出しは 1 つのフローに合流する。二重にブラウザは開かない

> [!NOTE]
> profile の切り替えは人間のシェル関数が行います。MCP からエージェント側の環境変数は変えられないので、
> エージェントは `list_profiles` で得た名前を `aws ... --profile <name>` に渡して使います。
> どのツールの結果にもトークン値や credentials は含まれません。

全コマンドは `--json` で機械可読になります(`schemaVersion` 付き)

```text
$ awsp status --json
{
  "schemaVersion": 1,
  "generatedAt": "2026-09-16T03:00:00Z",
  "configFile": "/Users/you/.aws/config",
  "overall": "error",
  "sessions": [
    {
      "name": "corp",
      "state": "error",
      "expiresAt": "2026-09-15T00:00:00Z",
      "remainingSeconds": -102532,
      "summary": "失効(1d 前)。'awsp login --sso-session corp' を実行してください",
      "profiles": ["dev", "prod"]
    }
  ]
}
```

#### 人間と AI で config を分ける

awsp は `AWS_CONFIG_FILE` を尊重します。エージェントの環境変数に読み取り専用ロールだけを書いた config を指しておけば、
人間は通常の config、エージェントはその config、を同じバイナリと同じトークンキャッシュで使い分けられます

| 起動元 | `AWS_CONFIG_FILE` | 読む config |
|---|---|---|
| 自分のシェル | 未設定 | `~/.aws/config` |
| Claude Code のセッションと `awsp mcp` | `~/.claude/settings.json` の `env` で指定 | 例: `~/.aws/config-agent` |

読み取り専用の config は、`sso_role_name` だけを置き換えたコピーで作れます

```bash
sed 's/^sso_role_name *=.*/sso_role_name = ReadOnlyAccess/' ~/.aws/config > ~/.aws/config-agent
```

```json
{ "env": { "AWS_CONFIG_FILE": "/Users/you/.aws/config-agent" } }
```

`awsp preflight` はセッション開始時のフックにも使えます。1 行を返し、失効していれば exit 1 です

```json
{ "hooks": { "SessionStart": [ { "hooks": [ { "type": "command", "command": "awsp preflight 2>/dev/null || true" } ] } ] } }
```

<details>
<summary>オプション詳細</summary>

`--login-only`: profile は変更せずログイン状態だけ確認  
`--no-login`: caller identity の確認とログインを省略して反映処理のみ実施  
`--shell`: `awsp init zsh` が内部利用する export / unset 出力モード  
`init zsh --command`: 連携関数内で実行する awsp コマンドを差し替える(例: `eval "$(awsp init zsh --command 'NWRELAY_TARGET_BIN=awsp command nwrelay')"`)  
`status --grace`, `preflight --grace`: 期限切れ後に自動更新を見込む猶予(既定 8h)  
非 TTY で `awsp` を引数なしで呼ぶと対話 UI を起こさず exit 2 で終わります

</details>

---

## AWS Config Example

`awsp` は `~/.aws/config`(または `AWS_CONFIG_FILE`)の `[profile ...]` と `[sso-session ...]` を読みます

<details>
<summary>SSO の最小構成例</summary>

```ini
[sso-session corp]
sso_start_url = https://example.awsapps.com/start
sso_region = us-west-2
sso_registration_scopes = sso:account:access

[profile dev]
region = us-west-2
sso_session = corp
sso_account_id = 123456789012
sso_role_name = AdministratorAccess
```

</details>

<details>
<summary>role + source_profile の例</summary>

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

## Design Notes

- CLI と MCP は同じ Go 関数の出口が 2 つあるだけ。JSON の型も共有
- SSO ログインは aws CLI を exec せず、AWS SDK for Go v2(ssooidc)で Authorization Code + PKCE を内製。トークンキャッシュは CLI と完全互換
- 状態確認はローカルファイルだけを読み、ネットワークを使わない。使うのは `whoami` と `login` だけ
- トークン値・credentials はディスク・出力・ログ・MCP の結果に一切載せない
- 描画は Lip Gloss v2 に一本化。TUI は Bubble Tea v2、ヘルプとエラーは Fang。非 TTY と `NO_COLOR` では装飾を落とす
- 決定と却下した案の記録は [`docs/design.md`](./docs/design.md)

---

## Development

### Prerequisites

- [Task](https://taskfile.dev/)(`task` コマンド)
- `mise` 推奨、または Go 1.26.x

### セットアップと日常コマンド

```bash
mise install
task tools      # golangci-lint
task fmt
task lint
task test
task build      # ./bin/awsp
task check      # fmt + lint + test
```

### デモ動画

`demo/` は紹介動画の Remotion プロジェクトです。`cd demo && npm install && npm run render` で MP4 をレンダーします。動画ファイルは git に入れません

### リリース

```bash
task release-check
task release-snapshot
```

- CI: `main` push / Pull Request で format lint test
- CD: `v*` タグ push で GoReleaser が GitHub Release を作成

---

## Contributing

Issue / Pull Request を歓迎します。大きめの変更は先に Issue で方針共有してもらえると助かります

1. ブランチ作成
2. 実装
3. `task check`
4. 変更内容を説明する Pull Request を作成

---

## License

MIT License  
詳細は [LICENSE](./LICENSE) を参照

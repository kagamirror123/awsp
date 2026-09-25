# AI エージェントから使う

`awsp mcp` は stdio の MCP サーバーです。エージェントは AWS を触る前に認証状態を確かめ、失効していればログインを起こして人の承認を待てます。

## 登録

Claude Code:

```bash
claude mcp add awsp -- awsp mcp
```

Codex(`~/.codex/config.toml`):

```toml
[mcp_servers.awsp]
command = "awsp"
args = ["mcp"]
```

## ツール

| ツール | 何をするか | ネットワーク |
|---|---|---|
| `auth_status` | SSO セッションの有効・失効。作業の最初に呼ぶ。`autoRefresh: true` なら `remainingSeconds` はアクセストークンの残りで、自動で延びる | 使わない |
| `list_profiles` | profile 一覧と account / role / 認証状態。`currentProfile` に人間がシェルで選んでいる profile(サーバーが引き継いだ `AWS_PROFILE`)を返す | 使わない |
| `whoami` | 指定 profile の caller identity。自動ログインしない | STS |
| `login` | ログインを起こし、人がブラウザで承認するまで待ってから返る | SSO OIDC、profile 指定時は STS |

出力は CLI の `--json` と同じ型で、outputSchema 付きです。トークン値や credentials はどのツールの結果にも含まれません。

### `login` の挙動

- 期限内なら profile 指定時に STS で確認して `status: "ok"`。認証エラーなら再ログインし、通信障害や権限不足では元のエラーを返す
- 開始処理を含めて承認完了まで待つ(既定 5 分、`timeout_seconds` で最大 15 分)。開始 API 自体は 30 秒で打ち切る
- ブラウザを開けなかったか時間切れなら `status: "pending"` と認可 URL を返す。人が URL を開いて承認し、エージェントがもう一度 `login` を呼ぶと同じフローに合流して完了する
- 開始待ちのまま期限を迎えた場合は `status: "pending", phase: "starting"` を返す。この時点では URL はない。再呼び出しで進捗を確認する。URL 確定後は `phase: "authorizing"`
- 共有フローは呼び出し側の期限では停止せず、サーバー終了、認可期限、または内部上限 15 分で終了する
- 同じ sso-session への同時呼び出しは 1 つのフローに合流する。二重にブラウザは開かない
- `use_device_code: true` で device code 方式に切り替え可(組織側で無効な場合あり)

### エージェントは何をどう使うか

MCP を登録すると、エージェントはツールの説明を読んで次のように振る舞います。

- 今と別のアカウントが要るとき、`list_profiles` で profile を探して `--profile` で読む。サーバーの instructions で伝えているので、ツールが遅延読み込みで説明がまだ読まれていなくても効く。instructions は 3 文で、毎セッションのコンテキストに載る
- `aws` コマンドが認証エラーで失敗したとき、`login` を呼んで復帰する。ここは説明文で誘導しているのでほぼ自動で起きる
- 作業前に `auth_status` で先回りするかは、エージェントの判断に任される。確実にしたいならプロジェクトの `CLAUDE.md` や `AGENTS.md` に 1 行書く

```markdown
- AWS を触る前に awsp の auth_status を呼び、失効していれば login を呼んでから続ける
```

profile の切り替えは人間のシェル関数が行います。MCP からエージェント側の環境変数は変えられないので、
エージェントは `list_profiles` で得た名前を `aws ... --profile <name>` に渡して使います。
どの profile を使うか指示が無いときは `currentProfile`(人間がシェルで選んでいるもの)を使うようツールの説明で誘導しています。
人間用と AI 用で config を分けている場合、`currentProfile` がその config に無い名前のこともあります。
`currentProfile` が出るのは、エージェントを `awsp <profile>` したシェルから起動したときだけです。デスクトップアプリなどシェル以外から起動すると `AWS_PROFILE` を引き継がないので出ません。

## 人間と AI で config を分ける

awsp は `AWS_CONFIG_FILE` を尊重します。エージェントの環境変数に読み取り専用ロールだけを書いた config を指しておけば、
人間は通常の config、エージェントはその config、を同じバイナリと同じトークンキャッシュで使い分けられます。

| 起動元 | `AWS_CONFIG_FILE` | 読む config |
|---|---|---|
| 自分のシェル | 未設定 | `~/.aws/config` |
| Claude Code のセッションと `awsp mcp` | `~/.claude/settings.json` の `env` | 例: `~/.aws/config-agent` |

読み取り専用の config は `sso_role_name` だけを置き換えたコピーで作れます。

```bash
sed 's/^sso_role_name *=.*/sso_role_name = ReadOnlyAccess/' ~/.aws/config > ~/.aws/config-agent
```

```json
{ "env": { "AWS_CONFIG_FILE": "/Users/you/.aws/config-agent" } }
```

いま読んでいる config は `awsp status` の 2 行目と `--json` の `configFile` に出ます。
SSO のトークンキャッシュは sso-session 名で決まるので、どちらの config でも同じログインが使えます。

## セッション開始時のフック

`awsp preflight` は 1 行を返し、失効していれば exit 1 です。SessionStart フックに入れると、失効を作業前にエージェントへ伝えられます。
MCP の `login` で途中復帰が安くなったので必須ではありません。フックの出力はセッション毎にコンテキストを消費します。

```json
{ "hooks": { "SessionStart": [ { "hooks": [ { "type": "command", "command": "awsp preflight 2>/dev/null || true" } ] } ] } }
```

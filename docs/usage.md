# Usage

`awsp --help` で一覧が見えます。ここでは各コマンドの挙動と出力例をまとめます。

## 切り替える

| コマンド | 何をするか |
|---|---|
| `awsp` | 対話 UI。文字を打つと絞り込み、↑↓ で移動、Enter で決定、q で中止 |
| `awsp <profile>` | 直接切り替え。失効していれば自動でログインしてから反映 |
| `awsp "(unset)"` | `AWS_PROFILE` と静的認証情報(`AWS_ACCESS_KEY_ID` など)を解除 |
| `awsp current` | いまの `AWS_PROFILE` の caller identity。失効していれば自動ログイン |
| `awsp whoami [profile]` | caller identity を確認するだけ。自動ログインしない。省略時は `AWS_PROFILE` |
| `awsp list` | 一覧。認証状態と残り時間、認証情報取得時刻も見える |

対話 UI は広い端末では左に一覧、右に詳細を表示し、狭い端末では縦に並べます。各行に状態と残り時間が付き、現在の profile を ● で示して初期選択します。詳細が収まらない場合は PgUp/PgDn でスクロールできます。

文字入力で検索を開始できます。q は検索中には文字として入力でき、検索中以外は中止に使います。q で始まる検索は / を押してから入力してください。Ctrl+C はどの状態でも中止します。

| 記号 | 意味 |
|---|---|
| 🟢 | SSO セッションが有効 |
| 🟡 | 期限切れだが refresh token があり、名前付き sso-session では使用時に自動更新を試みる(成功は未確認) |
| 🔴 | 失効。ログインが必要 |
| ⚪ | 未ログイン |
| 🪪 | SSO を使わない profile(静的認証情報や `source_profile`) |

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

切り替えると caller identity を確認して表示します。

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

`awsp <profile>` の結果を親シェルに反映するには `eval "$(awsp init zsh)"` が必要です。関数が `awsp ... --shell` の出力を `eval` します。

## 認証状態とログイン

| コマンド | 何をするか |
|---|---|
| `awsp status [--json] [--grace 8h]` | sso-session ごとの有効・失効・残り時間。ネットワークは使わない |
| `awsp login [profile]` | ブラウザで承認するだけのログイン。有効なら何もしない |
| `awsp login --sso-session <name>` | profile ではなく sso-session を指定 |
| `awsp login --no-browser` | ブラウザを開かず URL を表示するだけ |
| `awsp login --force` | 有効なセッションが残っていてもログインし直す |
| `awsp login --use-device-code` | device code 方式(組織側で無効な場合あり) |
| `awsp login --timeout 5m` | 承認待ちの上限 |
| `awsp preflight [--grace 8h]` | 1 行と exit code。ok / warning は 0、error / unknown は 1 |

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

### 状態の意味

| 状態 | 条件 |
|---|---|
| `ok` | トークンの期限内 |
| `warning` | 期限切れだが refresh token があり、猶予(既定 8 時間)以内。名前付き sso-session に限り、SDK が使用時に自動更新を試みる。認証エラーなら `awsp login <profile>` を実行 |
| `error` | legacy 形式の期限切れ、refresh token が無い、猶予超過、またはキャッシュを読めない。`awsp login` が必要 |
| `unknown` | トークンキャッシュが無い(未ログイン) |

判定はローカルキャッシュの期限と refresh token の有無を使います。値をデコードすることはありますが、トークン値を出力には含めません。実際の失効や更新可否はローカルだけでは確定できません。

壊れたキャッシュは該当セッションの error として表示し、他の profile の表示・切り替えは続けられます。通常の `awsp login` でもトークンキャッシュを修復できます。

一覧の Fetched と詳細の fetched は CLI がロール認証情報のキャッシュを取得・更新した時刻です。API の最終使用時刻ではなく、Go SDK の利用でも更新されません。JSON の `lastUsedAt` は既存の名前を維持しています。

期限内のトークンでも、profile 指定時の STS 確認で認証エラーになれば再ログインします。通信障害や権限不足ではブラウザを開かず、元のエラーを返します。開始 API の待機上限は 30 秒です。

### ログインの仕組み

Authorization Code + PKCE を Go で実装しています。`awsp login` は 127.0.0.1 の空きポートで待ち受けを始めてブラウザを開き、
「Allow access」を押すと localhost に戻って自動で完了します。書き出したトークンキャッシュは aws CLI や各言語の SDK がそのまま使います。

## JSON 出力

`current` / `list` / `status` / `login` / `whoami` は `--json` で機械可読になります。すべて `schemaVersion` を持ち、トークン値は含みません。成功時の stdout 全体が 1 つの JSON になり、承認 URL や案内は stderr に出ます。whoami はフラットな profile / account / userId / arn、login は CLI と MCP 共通の型です。

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

## その他のオプション

| オプション | 説明 |
|---|---|
| `--login-only` | profile は変更せずログイン状態だけ確認 |
| `--no-login` | caller identity の確認とログインを省略して反映処理のみ |
| `--shell` | `awsp init zsh` が内部で使う export / unset 出力モード |
| `-v, --verbose` | 詳細ログ |

## 端末と出力

- 表は端末幅に収まります。狭ければ優先度の低い列から落とし、それでも超えれば折り返します
- パイプ先や `NO_COLOR` では装飾を付けません。非 TTY では `COLUMNS` を幅として使います(無ければ 120)
- 引数なしの `awsp` を非 TTY で呼ぶと対話 UI を起こさず exit 2 で終わります

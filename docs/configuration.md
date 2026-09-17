# Configuration

awsp は `~/.aws/config`(または `AWS_CONFIG_FILE`)の `[profile ...]` と `[sso-session ...]` を読みます。書き込みはしません。

## SSO の最小構成

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

`sso_session` を使わず profile に `sso_start_url` を直書きする旧形式も読めます。

## role + source_profile

```ini
[profile base]
region = ap-northeast-1
output = json

[profile prod-readonly]
region = ap-northeast-1
role_arn = arn:aws:iam::123456789012:role/ProdReadOnly
source_profile = base
```

このような profile は SSO の状態を持たないので、一覧では 🪪 で表示します。

## 環境変数

| 変数 | 意味 |
|---|---|
| `AWS_CONFIG_FILE` | 読む config。未設定なら `~/.aws/config`。人間と AI で使い分ける方法は [mcp.md](./mcp.md) |
| `AWS_PROFILE` | `awsp current` と `awsp whoami` の既定の profile。`awsp <profile>` が設定する |
| `NO_COLOR` | 装飾を付けない |
| `COLUMNS` | 非 TTY のときの表の幅(無ければ 120) |

## 読むファイルと書くファイル

| パス | 読み書き | 何を見るか |
|---|---|---|
| `~/.aws/config` | 読む | profile と sso-session |
| `~/.aws/sso/cache/<sha1(sso-session 名)>.json` | 読む・書く | 期限と refresh token の有無を読む。ログイン時に aws CLI 互換の形式で書く |
| `~/.aws/cli/cache/*.json` | 読む | 一覧の認証情報取得時刻とロール認証情報の期限 |

トークン値・credentials は読み取っても出力せず、ログにも残しません。

## zsh 連携

```bash
echo 'eval "$(awsp init zsh)"' >> ~/.zshrc
```

生成される関数は、`awsp <profile>` のときだけ `awsp <profile> --shell` を呼んでその出力(`export` / `unset`)を `eval` します。
サブコマンド(`current` `list` `status` `login` など)はそのままバイナリに渡します。

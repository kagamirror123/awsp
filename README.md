# awsp

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev/)
[![Cobra](https://img.shields.io/badge/Cobra-1.10.2-00A3E0)](https://github.com/spf13/cobra)
[![AWS SDK for Go v2](https://img.shields.io/badge/AWS_SDK_v2-config%20%2F%20sts-FF9900?logo=amazon-aws)](https://github.com/aws/aws-sdk-go-v2)
[![pterm](https://img.shields.io/badge/pterm-0.12.x-00C2FF)](https://github.com/pterm/pterm)
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

```bash
$ awsp dev
┌─ 🪪 AWS Caller Identity ──────┐
| 🔐 Profile : dev              |
| 🧾 Account : 123456789012     |
| 👤 UserId  : AIDA...          |
| 🌍 ARN     : arn:aws:...      |
└───────────────────────────────┘
✅ Profile validated: dev
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

# 補完とシェル連携
awsp completion zsh
awsp init zsh
```

<details>
<summary>オプション詳細</summary>

`--login-only`: profile は変更せずログイン状態だけ確認  
`--no-login`: caller identity / sso login を省略して反映処理のみ実施

`--shell`: `awsp init zsh` が内部利用する export / unset 出力モード

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

## Design Notes

- caller identity は AWS SDK for Go v2 で型安全に取得
- SSO セッション確立は `aws sso login` を利用
- OIDC デバイス認可とトークンキャッシュの実運用を CLI に委譲し、安定性を優先
- 静的出力は `go-pretty` `pterm` `lipgloss` を組み合わせて視認性を最適化

---

## Development

開発者向け情報はここだけ見れば進められるように整理しています

### Prerequisites

- AWS CLI v2
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

# awsp 設計ドキュメント: AI ネイティブ化

状態: 2026-09-16 の設計に基づく実装済み。2026-09-18 のレビュー修正と D24〜D34 を反映。設計の正典はこのファイル。
確定した項目は「決定事項」へ、退けた案は「却下した案」へ移し、理由を必ず残す。

## 1. 目的

awsp を「人間がシェルで切り替える手」に加えて、
「コーディングエージェントが AWS を触る前に呼ぶ道具」にする。

エージェントが必要とするのは次の 4 つ。

1. いま SSO セッションは有効か(作業開始時に知りたい)
2. どんな profile があり、どの account / role を指すか
3. ある profile で叩いたとき自分は誰か
4. 失効していたらログインを起こし、人の承認を待って完了を確認する

## 2. 背景

- authdoc の 60 日実績: preflight 1230 回 / 失効検知 157 回 / fix 12 回。
  価値は「作業開始時の失効検知」に集中し、常駐 GUI 固有の価値は薄かった
- 2026-09-16 にメニューバーアプリ awsee を中止し、awsp の AI ネイティブ化に舵を切った
- 出典: 作者の作業記録(前身ツール authdoc の運用実績と、メニューバーアプリ案の中止判断)

## 3. 原則

- 公開の汎用ツール。特定個人の config ファイル名や role 名を埋め込まない
- トークン値・credentials を出力・ログ・MCP の結果に載せない。ログイン時のみ AWS CLI 互換の sso/cache に保存する。状態確認は expiresAt と refreshToken の有無を使い、読み取ったトークン値は公開しない
- 状態確認はローカルファイルのみでネットワークを使わない。ネットワークは whoami と login だけ
- エラー文には次に打つコマンドを含める

## 4. 決定事項

| # | 項目 | 決定 | 理由 |
|---|---|---|---|
| D1 | 提供形態 | CLI と MCP サーバー(stdio)の 2 出口。同じ Go 関数を共有 | SessionStart フックは CLI しか呼べない。エージェントの通常利用は型付きツールのほうが発見性と構造化で勝る |
| D2 | MCP ツール | auth_status / list_profiles / whoami / login の 4 つ | エージェントの要求(§1)と 1 対 1 |
| D3 | login の方式 | ブロック方式。人の承認完了まで待ち、STS で再確認して返す | 「login を 1 回呼ぶ → 続き」で流れが閉じる。authdoc の fix も同方式で 12 回成立 |
| D4 | login の作法 | ローカルで期限内なら profile 指定時は STS で確認し、認証エラーなら再ログインする / 待ち上限を awsp 自身が持つ / 上限超過時はフローを止めず pending として URL とコードを返す / 進行中の二重起動は合流 | クライアント側タイムアウトとの二重管理を避け、再試行を可能にする。D12 でフローが in-process になったので「子プロセスを殺す」は不要になり、認可フローの期限までは再呼び出しが同じフローに合流できる(2026-09-16 実装時に改訂)。CLI ではプロセス終了と共にフローも終わる |
| D5 | AWS_CONFIG_FILE | 尊重する。現状の `~/.aws/config` 固定を廃止 | エージェントセッションは別 config を向いている。今は `awsp list` が人間用 config を返す |
| D6 | 出力契約 | JSON に schemaVersion。MCP ツールは outputSchema 付き | モデルが読む出力なので契約を明示し、変更を検知可能にする |
| D7 | preflight | CLI のみ。1 行・装飾なし・exit code。ローカル判定 | SessionStart フック用。authdoc の状態モデルを移植 |
| D8 | 描画 | Lip Gloss v2 に一本化。ヘルプとエラーは Fang。TUI は Bubble Tea v2 | pterm / go-pretty / lipgloss の 3 本立てを解消 |
| D9 | TUI | 一覧行に認証状態と現在値の印を載せる。詳細には認証情報取得時刻を表示する。**2026-09-25 改訂**: 一覧行の残り時間は外した(D34) | 選ぶ時点で「使えるか」が見える。残り時間はアクセストークンの残りで、同じ sso-session の profile には同じ値が並ぶだけだった |
| D10 | 非 TTY | stdin が端末でなければ TUI を出さず、次のコマンドを含むエラーで終わる | エージェントが誤って対話 UI を起こしても固まらない |
| D11 | 切り替え | AWS_PROFILE の切り替えは人間用のシェル関数(D24)に残す。エージェントは list_profiles で名前を知り `--profile` を付けて叩く | MCP からエージェントの Bash の環境変数は変えられない |
| D12 | SSO ログインの実装 | aws CLI の exec をやめ、SDK(ssooidc)で **Authorization Code + PKCE**(localhost へのリダイレクト)を Go で持つ。CLI 互換の `~/.aws/sso/cache` を書く。人間の `awsp login` も同じ実装。device code は `--use-device-code` の opt-in として残す | URL とコードが構造化で取れ、D4 が素直に実装できる。更新前は保守負荷を理由に CLI へ委譲していた(`internal/awscli/client.go`)が、当時は自前実装の便益がゼロだった。要件が変わったので覆す。条件: 「awsp が書いたトークンで `aws sts get-caller-identity` が通る」検証を手順に残す。**2026-09-16 改訂**: 当初は device code を採用したが、作者の Identity Center では公式 CLI の `--use-device-code` でも承認画面で「サインイン認証情報を確認できませんでした」と弾かれ、device code の grant 自体が通らないと判明。CLI 既定と同じ PKCE に切り替えた。OIDC クライアント登録は追加のキャッシュ管理を避けるため毎回新規に行う。登録 URI はポートなしであり、再利用は技術的には可能 |
| D13 | login の待ちと通知 | 待ち上限は開始処理を含めて既定 5 分、フラグで変更可。登録・開始 API には別途 30 秒の上限を設ける。MCP では開始待ちのまま呼び出し期限を迎えた場合も pending (phase=starting、URL なし) を返す。認可 URL が確定した後は phase=authorizing。共有処理はサーバーの寿命と認可期限、最大 15 分で終了する。MCP では進行通知で認可 URL を流す(PKCE ではコードは無い。device code 時だけ userCode が付く)。ブラウザ起動に失敗したら認可 URL を結果に入れて即返し(pending)、フローはサーバー内で継続、次の login 呼び出しが合流してブロックする | 進行通知を表示しないクライアントでも人に URL が届く |
| D14 | 判定単位 | sso-session 単位。config に複数あれば全部判定し、1 つでも error なら overall は error | profile は同じ session を共有するので profile 単位は冗長 |
| D15 | 読み取り専用 config の派生生成 | **2026-09-16 に撤回し却下へ**(§5 参照) | — |
| D16 | TTY 別の状態ファイル | やらない | ps 方式は不可と実測済み。需要が出てから |
| D17 | TUI の骨格 | 幅 96 文字以上では左一覧・右詳細、狭い端末では縦配置。詳細は PgUp/PgDn でスクロールし、画面内に収める | 使い慣れた形を壊す理由がない |
| D18 | 失効後の猶予 | 既定 8 時間、`--grace` で変更可 | IAM Identity Center のセッション期間の既定が 8 時間で、refreshToken はその期間内だけ使える。キャッシュにはセッション終端が無いので expiresAt からの推定で代用する |
| D19 | コマンド名 | `status` / `preflight` / `whoami` / `login` / `mcp`。`current` と `list` は残す | 既存利用者の手癖を壊さない |
| D20 | 配布 | goreleaser 継続。Homebrew tap は D28 で追加。MCP 登録は README に `claude mcp add awsp -- awsp mcp` を書き、生成コマンドは作らない | 登録は 1 行で済む |
| D21 | SessionStart フック | 作者環境では使わない。`awsp preflight` は使いたい人向けの CLI として残す | セッション毎にフック出力がコンテキストを消費するのが無駄。MCP の `login` で途中失効からの復帰が 1 呼び出し + ブラウザ 1 クリックになったので、開始時に先回りする価値が下がった(2026-09-16)。フックは authdoc のものを削除済み |
| D22 | 表の幅 | 人間向けの表は端末幅(非 TTY なら COLUMNS、無ければ 120)に収める。優先度の低い列から落とし、それでも超えれば折り返す。ID 系の列は「…」で切る。一覧の中身(profile 名の列挙など)は件数にして表に入れない | 内容が長いと表が崩れ、読めなくなる(2026-09-16 に status の Profiles 列で実害)。表は概観、詳細は別コマンドか JSON |
| D23 | ログインのやり直し | CLI に `awsp login --force` を足す(有効でもログインし直す)。MCP の `login` には出さない | 画面や挙動を確認したいときに手段が無かった。エージェント側に出さないのは、有効なのにブラウザを開かせる操作を自律的に選ばせたくないため(D4 の「有効なら即返す」を MCP では守る) |
| D24 | シェル連携 | `awsp init` は zsh / bash / fish の 3 つ。`--shell` は値なしで posix(export / unset)、`--shell=fish` で fish 構文(`set -gx` / `set -q ...; and set -e -g`)。fish 向けの出力は各行を `;` で終える | `completion` が 4 シェル対応なのに切り替え本体が zsh 専用なのは公開ツールとして目立つ穴。bash は zsh と同じ関数で動く。fish は `export` / `unset` を持たないので構文を分ける。行末の `;` は fish が command substitution を行のリストにし eval が空白で連結するため。PowerShell は D25 で Windows を外すので作らない(2026-09-18) |
| D25 | Windows | ビルド対象から外す(darwin / linux の amd64 / arm64 のみ) | ブラウザ自動起動が未実装、シェル連携が無く、作者に検証環境も無い。動かないバイナリを配るより正直に外す。需要が出たら `BROWSER` 環境変数の尊重と PowerShell 連携を揃えて戻す(2026-09-18) |
| D26 | 依存更新の自動化 | Dependabot(gomod / github-actions、週次、エコシステムごとに 1 PR にグループ化)→ CI 通過で auto-merge(squash)→ 定期実行(平日 10〜18 時 JST に毎時)が main の HEAD を見て、未タグかつ author が dependabot なら patch を自動タグ → goreleaser。次の版は HEAD から辿れるタグではなく全タグの最大から求める(v0.11.0 / v0.11.1 が作業ブランチのコミットに付いていて main から辿れないため)。`task release-tag` は main で origin/main と一致しているときだけ動く。Go モジュールの semver-major だけは auto-merge から除外して人が見る。人間の PR は従来どおり `task release-tag` | 依存更新の PR を人が眺める価値は無く、CI が門番になっている。**当初は main への push で起動する設計だったが、auto-merge は GITHUB_TOKEN で予約するためマージ後の push イベントがワークフローを起動しない**(v0.11.0 直後の #15 で実測。CI も auto-release も動かなかった)。PAT を足せば push 起動にできるが、Dependabot 起点のワークフローは Dependabot secrets しか読めず設定が 2 箇所に増えるので、鍵を増やさない定期実行にした。GITHUB_TOKEN で push したタグも `release.yml` の tag トリガーを起動しないので、タグ作成と同じ実行内で `release.yml` を reusable workflow として呼ぶ。リポジトリ側の前提: "Allow auto-merge" 有効、main のルールセットが `CI / Lint and Test` を必須(既存)(2026-09-18) |
| D27 | SBOM | goreleaser の `sboms` で syft を使い、アーカイブごとに SPDX JSON を Release へ添付 | 公開バイナリの依存の証跡。設定 2 行と syft のインストール 1 ステップで済む。成果物の署名(cosign keyless)は未着手。Rekor の公開ログに記録が残る運用なので需要が見えてから(2026-09-18) |
| D28 | Homebrew tap | goreleaser の `homebrew_casks` で `kagamirror123/homebrew-tap` に cask を自動生成・push。`brew install --cask kagamirror123/tap/awsp`。未署名バイナリなので post-install で quarantine 属性を外す。Linux は Releases の curl のまま | goreleaser は v2.10 以降 `brews`(formula)を非推奨にし、バイナリ配布は cask を推奨している。cask は macOS 専用だが awsp の利用者は macOS が主で、Linux に Homebrew を入れている人はまれ。tap への push は GITHUB_TOKEN では届かないので fine-grained PAT を Secrets に置く(2026-09-18) |
| D29 | profile 名の補完 | `awsp` / `login` / `whoami` の位置引数に Cobra の ValidArgsFunction で config の profile 名を返す。補完スクリプトはリリース時に生成して tar.gz と cask に同梱する(Homebrew が zsh の `awsp.zsh` を `_awsp` に改名する)。シェル関数は `__complete` / `__completeNoDesc` を素通しする | 十数個の profile 名を正確に打つのは負担で、TUI を開くより `awsp de<Tab>` のほうが速い。読むのは config だけでネットワークは使わない。`(unset)` は括弧の補完エスケープが読みにくいので候補に入れない。素通しを忘れると関数が補完呼び出しを切り替えと誤解して `--shell` を付ける(2026-09-19) |
| D30 | currentProfile | `list --json` と MCP の `list_profiles` に `currentProfile`(プロセスの `AWS_PROFILE`)を載せる。MCP サーバーは起動元シェルの環境を引き継ぐので、それが人間の選択になる。ツール説明で「指示が無ければこれを使う」と誘導する。schemaVersion は 1 のまま(任意フィールドの追加) | エージェントは D11 で切り替えを持たず、どの profile で叩くべきかを推測か質問で決めていた。人間の選択を 1 フィールドで伝えれば往復が減る。人間用と AI 用で config を分けている場合はこの config に無い名前になり得るので、その旨を説明に書く(2026-09-19)。**制約(2026-09-25)**: 効くのはエージェントをそのシェルから起動したときだけ。デスクトップアプリなどシェル以外から起動したエージェントは `AWS_PROFILE` を持たないので出ない。作者の 9/16 以降のセッションは全部これで一度も出ていないが、エージェントは依頼内容や貼られたプロンプトから profile を選べていたので足さない |
| D31 | コンソールを開く | `awsp console [profile] [url]`。IAM Identity Center のアクセスポータルの deep link(`<start_url>/#/console?account_id=…&role_name=…[&destination=…]`)を組んでブラウザで開く。profile 省略時は AWS_PROFILE。destination は https の AWS コンソール(`*.aws.amazon.com` / `*.amazonaws.com` 系)に限る。`--no-browser` と `--json` あり。MCP には出さない | awsee を中止したとき「GUI に残る価値はコンソール deep link だけ」と判断した、その 1 点を CLI に持つ。ターミナルで使っている profile のままコンソールへ移れる。Slack やチケットのコンソール URL はサインイン中のアカウントに飛ぶので、destination で正しいアカウントに向ける。URL を組んで開くだけでトークンに触れず、ネットワークも使わない。エージェントにブラウザを開かせる用途は無いので MCP ツールにしない。SSO を使わない profile は federation(GetSigninToken に認証情報を送る)が要るので対象外(2026-09-19) |
| D32 | MCP サーバーの instructions | initialize で短い利用方針を返す。内容は「profile ごとに別の account / role に届く。今と別のアカウントが要るときは list_profiles で profile を探し、`--profile` を付けて aws CLI で読む。届かないと結論したり、人に読み取りを頼んだりしない」と「失効していれば login」の 2 点だけ | 2026-09-18 に、1 アカウント固定の別 MCP サーバー(指示文で aws CLI を使わないよう求める)を先に使ったエージェントが「このアカウントにしか繋がっていない」と人に読み取りを頼み、指摘されるまで list_profiles を呼ばなかった。Claude Code はツールを遅延読み込みにするので、ツールの説明は検索するまで読まれない。instructions はツールが遅延でも毎セッション読まれる。代償は毎セッション約 70 トークンで、D21 と同じ種類のコストになる。固定の短文で、人の手を止めた失敗を防げるので受け入れる(2026-09-25) |
| D33 | カードの幅 | 切り替え後・`current`・`whoami`・`console`・`login` のカードは、端末幅に収まるときだけ枠を付け、はみ出すときは枠を外して字下げだけで出す。切り替え後と `login` のカードは ARN を載せず、ARN から取り出したロール名(IAM Identity Center のロールは許可セット名)と UserId を載せる。ARN は `current` / `whoami` / `--json` で見る | 枠の幅は最長行で決まり、ARN やコンソール URL で 133〜137 桁になっていた。それより狭い端末では全行が折り返して枠が割れる。枠の中で折り返すと ARN や URL に罫線が混ざってコピーもクリックもできなくなるので、はみ出すときは枠を外して端末の折り返しに任せる。一番よく見る切り替え後のカードは ARN を外すと 60 桁ほどになり、たいていの端末で枠が残る(2026-09-25) |
| D34 | 残り時間の出し方 | 自動更新が効く(名前付き sso-session で refreshToken がある)とき、ok の表示に残り時間を出さない。`status` / `auth_status` / `preflight` の要約は「有効(自動更新あり)」、`status` 表の Remaining は「自動更新」。TUI の一覧行と `list` 表の残り時間は外す。自動更新が効かない legacy 形式などは従来どおり残り時間を出す。JSON は `expiresAt` / `remainingSeconds` を残し、任意フィールド `autoRefresh` を足す(schemaVersion は 1 のまま) | 残り時間はアクセストークン(1 時間)の残りで、SDK / CLI が refreshToken で取り直すので、ok ならいつ見ても 1 時間弱になる。「あと何分でログインし直し」と読めてしまう。2026-09-18 に「残り 35m」と出たあと 8 時間使えた。本当の期限(セッション終端)はキャッシュに無い(Q12)。TUI では全 profile の右に同じ値が並んで邪魔だった(2026-09-25) |

## 5. 却下した案

| 案 | 理由 | 出典 |
|---|---|---|
| メニューバー GUI / awsp を GUI のバックエンドにする / Go + systray 統合 | 他シェルの AWS_PROFILE を読めず切り替えもできない。2 言語の JSON 境界の維持コスト | `awsee-cancelled-awsp-ai-native`, `awsee-rebuild-from-authdoc` |
| LLM を組み込んで自然言語で profile を選ぶ | 十数 profile なら fuzzy filter で足りる。ネットワーク依存と鍵管理が増えるだけ | 本セッション |
| awsp に書き込み禁止ロジックを入れる | 境界は認証情報の分離(読み取り専用 config)であり、ツール側の deny は境界ではない | `ai-aws-readonly-guardrail` |
| login が URL とコードを返して即終了し、auth_status で確認させる | 往復が増える。ブロック方式(D3)を採用 | 本セッション |
| `awsp derive`(sso_role_name だけ置換した読み取り専用 config を生成) | 一度実装したが撤回。中身は sed 1 行と同じで、使う頻度は初回と profile 追加時だけ。用途が「config を分けて AI に読み取り専用を見せる」1 方式に固定され、公開ツールの芯(ローカル状態を読む・ログインする・エージェントに見せる)から外れる唯一の書き込み系コマンドだった。config-agent のズレは「作る」より「知る」問題で、生成コマンドは解になっていない。作り方は README に sed の 1 行として残す | 本セッション(2026-09-16) |
| バイナリと関数の名前を分ける(`awsp` はバイナリのまま、切り替えだけ `ap` などの関数にする。zoxide の `z`、Granted の `assume` と同じ形) | `which awsp` がパスではなく関数定義を返す違和感は消えるが、`awsp list` と `ap dev` の 2 つの名前を覚えることになる。子プロセスは親シェルの環境変数を変えられないので、切り替えを関数にすること自体は避けられず、名前を 1 つにまとめる今の形の代償は `which` の見た目だけ。動作上の差は無い | 本セッション(2026-09-18) |
| 打ち間違いに候補を出す(`awsp lit` に「もしかして: awsp list」) | 作者のシェル履歴に `lit` / `sattus` / `consple` / `agoop-edv` / `agoop0dev` があり、どれも「指定プロファイルが見つかりません」だけで終わっていたが、作者判断で見送り | 利用履歴の振り返り(2026-09-25) |

## 6. 状態モデル

### 6.1 SSO セッション(auth_status / preflight)

入力は `~/.aws/sso/cache/<sha1(sso_session 名)>.json` の `expiresAt` と `refreshToken` の有無のみ。
legacy 形式(`sso_start_url` 直書き)は `sha1(start URL)` がキー。

| 状態 | 条件 | 表示 |
|---|---|---|
| ok | expiresAt が未来 | 有効(自動更新あり)。自動更新が効かない legacy などは有効(残り時間)(D34) |
| warning | 名前付き sso-session で期限切れだが refreshToken あり、猶予内 | 使用時に自動更新を試行。認証エラーなら awsp login を案内 |
| error | legacy の期限切れ、refreshToken なし、猶予超過、またはキャッシュ読み取りエラー | 失効。`awsp login` を案内 |
| unknown | キャッシュファイルなし | 未ログイン |

猶予は既定 8 時間(D18)。refresh token の実際の有効期限はキャッシュから分からないため、自動更新の成功を保証しない。legacy 形式は SDK/CLI の自動更新対象ではない。キャッシュが壊れていても他のセッションの判定は続け、該当セッションに diagnostic を付けて error とする。

### 6.2 profile 別の認証情報取得時刻(TUI / list_profiles)

`~/.aws/cli/cache/<sha1(json)>.json` の mtime と `Credentials.Expiration` を使う。
mtime は CLI が認証情報を取得・更新した時刻であり、API の最終使用時刻ではない。Go SDK の利用では更新されない。
表示名は Fetched とし、既存の JSON フィールド名 `lastUsedAt` は互換性のため維持する。

キーは `{"accountId","roleName","sessionName"}` をキー順、空白なし、ASCII エスケープ付きの JSON にした SHA-1。
legacy 形式では `sessionName` の代わりに `startUrl` を使う。`Credentials` から読むのは Expiration だけ。
読み取りエラーは profile の diagnostics に載せ、一覧全体や他の profile の切り替えは止めない。

### 6.3 JSON の契約

`current` / `list` / `status` / `login` / `whoami` の CLI JSON、および MCP の各出力に `schemaVersion: 1` を付ける。
whoami は profile / account / userId / arn をフラットに保つ。login は CLI/MCP 共通の型を使い、status は ok または pending。
CLI は成功時だけ JSON を stdout に出し、URL・承認案内は stderr に出す。MCP には継続フローの管理と通知を残す。
新規の任意フィールド追加はバージョン 1 で行い、既存フィールドの意味・型を壊す変更時にバージョンを上げる。

## 7. コマンドとツールの対応

| 機能 | CLI | MCP |
|---|---|---|
| 認証状態 | `awsp status [--json]` / `awsp preflight` | auth_status |
| profile 一覧 | `awsp list --json` | list_profiles |
| caller identity | `awsp current --json` / `awsp whoami <profile>` | whoami |
| コンソールを開く | `awsp console [profile] [url]` | なし(人間向け、D31) |
| ログイン | `awsp login [profile]` | login |
| 切り替え | `awsp <profile>`(zsh / bash / fish の関数、D24) | なし(D11) |
| MCP 配信 | `awsp mcp` | - |

## 8. 未決事項

実装中に出た論点はここへ追加し、決めたら §4 へ移す。

| # | 問い | 現時点の推奨 |
|---|---|---|
| Q10 | `warning`(期限切れだが refreshToken あり)の状態で `login` を呼んだとき、ブラウザを開かず refresh_token grant で静かに更新する経路を持つか | 持つ価値はある。現在は再認証が必要なら PKCE(指定時のみ device code)を使う。名前付き sso-session は SDK が使用時に更新を試みる。静かな refresh の追加は需要が見えてから |
| Q11 | Fang のヘルプ・エラー表示が説明文の先頭語を Title Case にする(「AWS」→「Aws」)問題への恒久対応 | 説明文を日本語で書き始める運用で回避(AGENTS.md に規則化)。エラー表示は自前ハンドラで原文のまま出す。Fang 側に無効化オプションが入れば置き換える |

| Q12 | 期限切れから 8 時間以内でも refresh token が無効なことがある | 初回認証からのセッション終端はキャッシュにない。独自の履歴は作らず warning を推定として表示し、認証エラーなら login を案内する。legacy は error とする |

## 9. スタック

| 役割 | 採用 |
|---|---|
| MCP | modelcontextprotocol/go-sdk v1.8.0、stdio |
| TUI | Bubble Tea v2.0.9 / Bubbles v2.2.1 / Lip Gloss v2.0.6 |
| CLI 外装 | Cobra + Fang v1.0.0 |
| AWS | SDK v2 + ssooidc v1.35.15 |
| テスト | TUI は Update とコマンドを同期実行するゴールデン。点滅しないカーソルと固定時刻・タイムゾーンを使う。MCP は in-memory transport |

## 10. 実装と検証

AWS_CONFIG_FILE、JSON、非 TTY、status / preflight、PKCE login、MCP、描画の統一は実装済み。
派生 config 生成は撤回済み(D15)。2026-09-18 のレビューで認証復旧、破損キャッシュ、MCP の並行処理と期限、JSON 出力、TUI の端末幅への対応を修正した。
同日に D24〜D28 を実装。2026-09-19 に D29〜D31、2026-09-25 に D32〜D34 を実装。bash / zsh の関数は実バイナリで `awsp <profile> --no-login` の反映まで確認した。fish は作者の環境に無く、構文の確認は未実施。

D12 の実 AWS での確認は 2026-09-16 に実施済み: `awsp login <profile>` が書いたキャッシュで `aws sts get-caller-identity --profile <profile>` が成功し、トークンファイルのキーは CLI と同じ 8 個、権限 0600、refreshToken あり。レビュー修正の自動テストでは実 AWS を使わず、OIDC/STS のフェイク、隔離したキャッシュ、localhost のコールバックを使う。

リリース前に必要な実環境の互換性確認は、利用者が `awsp login <profile> --force`、続いて `aws sts get-caller-identity --profile <profile>` を実行して行う。

フックは 2026-09-16 に作者環境から削除済み(D21)。前身ツール authdoc に残る役割は無い。

## 11. 参照

- authdoc の `status --json` 形: schemaVersion / generatedAt / overall / targets[]{id, state, summary, expiresAt, remainingSeconds, detail}
- 前身ツール authdoc の設計と実測(sso/cache・cli/cache のキー導出、トークン値を公開しない原則)は本文 §6 に取り込んだ

# todo

- [x] lessons を確認する（`tasks/lessons.md` は未作成）
- [x] `awsp init zsh` と既存テストの構造を確認する
- [x] `init zsh` に実行コマンド差し替えオプションを追加する
- [x] README に最小限の利用例を追加する
- [x] fmt / test / build で検証する

## review

- `awsp init zsh --command` を追加し、生成される連携関数内の実行コマンドだけ差し替え可能にした。
- `nwrelay` 利用時は `eval "$(awsp init zsh --command 'NWRELAY_TARGET_BIN=awsp command nwrelay')"` を `.zshrc` に置けば、`--shell` / `eval` の親シェル反映処理を維持できる。
- `task fmt`、`task lint`、`task test`、`task build`、生成 zsh スクリプトの `zsh -n` を確認済み。

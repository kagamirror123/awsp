package prompt

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

// updateGolden は -update でゴールデンを書き直すためのフラグ
var updateGolden = flag.Bool("update", false, "ゴールデンファイルを現在の出力で更新する")

// goldenNow はゴールデンテストの基準時刻(残り時間表示を決定的にするため固定する)
var goldenNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// ゴールデンの描画に使う端末サイズ
const (
	goldenWidth  = 120
	goldenHeight = 30
)

func goldenProfiles() []awsp.Profile {
	okExpires := goldenNow.Add(52 * time.Minute)
	errExpires := goldenNow.Add(-11 * time.Hour)

	return []awsp.Profile{
		{
			Name:             "dev",
			Region:           "us-west-2",
			SSOSession:       "corp",
			SSOAccountID:     "111122223333",
			SSORoleName:      "AdministratorAccess",
			SessionState:     ssocache.StateOK,
			SessionExpiresAt: &okExpires,
		},
		{
			Name:             "prod",
			Region:           "ap-northeast-1",
			SSOSession:       "corp",
			SSOAccountID:     "444455556666",
			SSORoleName:      "ReadOnlyAccess",
			SessionState:     ssocache.StateError,
			SessionExpiresAt: &errExpires,
		},
		{
			Name:          "legacy",
			Region:        "ap-northeast-1",
			RoleARN:       "arn:aws:iam::999999999999:role/ReadOnly",
			SourceProfile: "base",
		},
	}
}

// newGoldenModel は端末サイズを与えた初期モデルを返す
func newGoldenModel(t *testing.T) tea.Model {
	t.Helper()

	model := newSelectModel(buildItems(goldenProfiles(), goldenNow))
	model.location = time.UTC
	return apply(t, model, tea.WindowSizeMsg{Width: goldenWidth, Height: goldenHeight})
}

// apply はモデルにメッセージを順に適用し 返ってきたコマンドも同期的に実行して結果を反映する
//
// tea.Program のループを回さないのが肝。プログラムを動かすとフレーム数と描画の刻みが
// スケジューリング次第で変わり、出力の比較が環境ごとに揺れる(実際に CI で散発的に落ちた)。
// ここでは Update を直接呼び 非同期コマンド(list の絞り込みが返す FilterMatchesMsg など)も
// その場で実行して畳むので、同じ入力なら必ず同じ結果になる。
func apply(t *testing.T, model tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()

	for _, msg := range msgs {
		var cmd tea.Cmd
		model, cmd = model.Update(msg)
		model = drain(t, model, cmd, 0)
	}
	return model
}

// drain はコマンドを実行して得たメッセージをモデルへ戻す
// depth は相互に発火し続けるコマンドで止まらなくなるのを防ぐための上限
func drain(t *testing.T, model tea.Model, cmd tea.Cmd, depth int) tea.Model {
	t.Helper()

	const maxDepth = 16
	if cmd == nil {
		return model
	}
	if depth >= maxDepth {
		t.Fatalf("コマンドの連鎖が %d 段を超えた", maxDepth)
	}

	// 点滅しないカーソルを使うため、タイマーを壁時計で選別せず全て実行できる。
	msg := cmd()

	switch typed := msg.(type) {
	case nil:
		return model
	case tea.BatchMsg:
		for _, batched := range typed {
			model = drain(t, model, batched, depth+1)
		}
		return model
	case tea.QuitMsg:
		// 終了要求は描画に影響しないので無視する
		return model
	default:
		var next tea.Cmd
		model, next = model.Update(msg)
		return drain(t, model, next, depth+1)
	}
}

// render はモデルの描画結果から装飾を取り除いた文字列を返す
// ANSI を剥がすのは 端末の色能力の判定でゴールデンが揺れないようにするため
func render(t *testing.T, model tea.Model) string {
	t.Helper()
	return ansi.Strip(model.View().Content)
}

func TestGolden_InitialView(t *testing.T) {
	requireGolden(t, render(t, newGoldenModel(t)))
}

func TestGolden_FilterByTyping(t *testing.T) {
	model := apply(t, newGoldenModel(t),
		tea.KeyPressMsg{Text: "d", Code: 'd'},
		tea.KeyPressMsg{Text: "e", Code: 'e'},
		tea.KeyPressMsg{Text: "v", Code: 'v'},
	)

	requireGolden(t, render(t, model))
}

func TestGolden_MoveDown(t *testing.T) {
	model := apply(t, newGoldenModel(t), tea.KeyPressMsg{Code: tea.KeyDown})

	requireGolden(t, render(t, model))
}

func TestGolden_EnterSelects(t *testing.T) {
	// カーソルは (unset) から始まるため 2 回下移動して "prod" を選ぶ
	model := apply(t, newGoldenModel(t),
		tea.KeyPressMsg{Code: tea.KeyDown},
		tea.KeyPressMsg{Code: tea.KeyDown},
		tea.KeyPressMsg{Code: tea.KeyEnter},
	)

	final, ok := model.(selectModel)
	if !ok {
		t.Fatal("最終モデルの型変換に失敗")
	}
	if final.selected != "prod" {
		t.Fatalf("Enter で確定した選択が想定外: %s", final.selected)
	}
}

func TestGolden_QuitAborts(t *testing.T) {
	model := apply(t, newGoldenModel(t), tea.KeyPressMsg{Text: "q", Code: 'q'})

	final, ok := model.(selectModel)
	if !ok {
		t.Fatal("最終モデルの型変換に失敗")
	}
	if !final.aborted {
		t.Fatal("q で中止扱いになっていない")
	}
}

// requireGolden は testdata のゴールデンと比較する -update で書き直す
func requireGolden(t *testing.T, got string) {
	t.Helper()

	// パスはテスト名から組み立てるだけで外部入力を含まない
	path := filepath.Join("testdata", t.Name()+".golden") //nolint:gosec // テスト名由来の固定パス
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatalf("testdata を作成できません: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("ゴールデンを更新できません: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path) //nolint:gosec // 上と同じくテスト名由来の固定パス
	if err != nil {
		t.Fatalf("ゴールデンを読めません(-update で作成できます): %v", err)
	}
	if got != string(want) {
		t.Fatalf("描画がゴールデンと一致しません\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}
}

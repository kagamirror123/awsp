package prompt

import (
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

// goldenNow はゴールデンテストの基準時刻(残り時間表示を決定的にするため固定する)
var goldenNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

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

// newGoldenTestModel は色プロファイルを固定した teatest.TestModel を作る
// 色プロファイル依存でゴールデンが不安定にならないようにする
func newGoldenTestModel(t *testing.T) *teatest.TestModel {
	t.Helper()

	model := newSelectModel(buildItems(goldenProfiles(), goldenNow))
	return teatest.NewTestModel(
		t,
		model,
		teatest.WithInitialTermSize(120, 30),
		teatest.WithProgramOptions(tea.WithColorProfile(colorprofile.NoTTY)),
	)
}

func TestGolden_InitialView(t *testing.T) {
	tm := newGoldenTestModel(t)

	tm.Send(tea.KeyPressMsg{Text: "q", Code: 'q'})

	out := readFinalOutput(t, tm)
	teatest.RequireEqualOutput(t, out)
}

func TestGolden_FilterByTyping(t *testing.T) {
	tm := newGoldenTestModel(t)

	// フィルタの絞り込みは list 内部で非同期コマンド(FilterMatchesMsg)として実行される
	// tm.Output() は FinalOutput() と同じストリームを消費してしまうため WaitFor では待てず
	// 完了を待つ短い猶予を置いてから quit する(bubbletea 本家のテストと同じ作法)
	tm.Type("dev")
	time.Sleep(100 * time.Millisecond)
	tm.Send(tea.KeyPressMsg{Text: "q", Code: 'q'})

	out := readFinalOutput(t, tm)
	teatest.RequireEqualOutput(t, out)
}

func TestGolden_MoveDown(t *testing.T) {
	tm := newGoldenTestModel(t)

	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Text: "q", Code: 'q'})

	out := readFinalOutput(t, tm)
	teatest.RequireEqualOutput(t, out)
}

func TestGolden_EnterSelects(t *testing.T) {
	tm := newGoldenTestModel(t)

	// カーソルは (unset) から始まるため 2 回下移動して "prod" を選ぶ
	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	out := readFinalOutput(t, tm)
	teatest.RequireEqualOutput(t, out)

	final, ok := tm.FinalModel(t).(selectModel)
	if !ok {
		t.Fatal("最終モデルの型変換に失敗")
	}
	if final.selected != "prod" {
		t.Fatalf("Enter で確定した選択が想定外: %s", final.selected)
	}
}

func readFinalOutput(t *testing.T, tm *teatest.TestModel) []byte {
	t.Helper()

	data, err := io.ReadAll(tm.FinalOutput(t))
	if err != nil {
		t.Fatalf("最終出力の取得に失敗: %v", err)
	}
	return data
}

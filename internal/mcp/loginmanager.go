package mcp

import (
	"sync"

	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
	"github.com/kagamirror123/awsp/internal/ssologin"
)

// loginFlowEntry は 1 つの sso-session に対する進行中(または直前に完了した)ログインフローの状態
//
// ready は認可 URL(flow と state)、または処理全体の結果が確定した時点で close する
// done は Login の結果(result と err)が確定した時点で close する
// close するまでは受信側をブロックさせる合図として使うだけで
// フィールドの読み書きそのものは「書き込み → close」「close の受信 → 読み込み」の順序を
// 必ず守ることで排他無しに安全にする(Go のメモリモデル上 close は receive に対して happens-before になる)
type loginFlowEntry struct {
	ready chan struct{}
	done  chan struct{}

	flow  *ssologin.Flow
	state ssocache.EvaluationState

	profile string
	result  awsp.LoginResult
	err     error
}

// loginManager は sso-session の CacheKey ごとに進行中のログインフローを高々 1 つに保つ
// 同じ session への同時 login 呼び出しは新しくログインフローを起こさず
// 進行中のものに合流させる(D4 D13)
type loginManager struct {
	mu    sync.Mutex
	flows map[string]*loginFlowEntry
}

func newLoginManager() *loginManager {
	return &loginManager{flows: make(map[string]*loginFlowEntry)}
}

// acquire は key に対応するエントリを返す
// 既存の進行中エントリがあればそれを返し starter=false
// 無ければ新規作成して登録し starter=true を返す(ログインフローを起こす責務を持つ側)
func (m *loginManager) acquire(key string) (entry *loginFlowEntry, starter bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.flows[key]; ok {
		return existing, false
	}

	entry = &loginFlowEntry{
		ready: make(chan struct{}),
		done:  make(chan struct{}),
	}
	m.flows[key] = entry
	return entry, true
}

// release は完了したエントリをマップから外す
// その間に新しいエントリへ置き換わっていれば何もしない(取り違え防止)
func (m *loginManager) release(key string, entry *loginFlowEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.flows[key] == entry {
		delete(m.flows, key)
	}
}

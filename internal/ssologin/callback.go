package ssologin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// callbackResult は PKCE のローカルコールバックサーバーが受け取った 1 回分の結果
type callbackResult struct {
	code  string
	state string
	err   error
}

// callbackServer は PKCE のリダイレクトを受けるローカル HTTP サーバー
// ブラウザを開く前に listen を始め 1 回リクエストを受けたら閉じる
type callbackServer struct {
	expectedState string
	listener      net.Listener
	server        *http.Server
	resultCh      chan callbackResult
	closeOnce     sync.Once
}

// startCallbackServer は addr で listen し PKCE のコールバックを待つサーバーを起動する
func startCallbackServer(addr, state string) (*callbackServer, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ローカルコールバックサーバーの起動に失敗: %w", err)
	}

	cs := &callbackServer{
		expectedState: state,
		listener:      listener,
		resultCh:      make(chan callbackResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(redirectPath, cs.handle)
	cs.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		_ = cs.server.Serve(listener)
	}()

	return cs, nil
}

// port はローカルコールバックサーバーが実際に listen しているポート番号を返す
func (c *callbackServer) port() int {
	if tcpAddr, ok := c.listener.Addr().(*net.TCPAddr); ok {
		return tcpAddr.Port
	}
	return 0
}

// redirectURIWithPort はポート付きの redirect URI を返す 認可 URL と CreateToken に使う
func (c *callbackServer) redirectURIWithPort() string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", c.port(), redirectPath)
}

// handle は OAuth コールバックを処理する
// error クエリがあれば失敗 code と state があれば取り込む それ以外(favicon 等)は無視する
func (c *callbackServer) handle(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	switch {
	case (query.Get("code") != "" || query.Get("error") != "") && query.Get("state") != c.expectedState:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, failureCallbackPage())
		c.send(callbackResult{err: errors.New("認可応答の state が一致しません。再度ログインしてください")})
	case query.Get("error") != "":
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, failureCallbackPage())
		c.send(callbackResult{err: fmt.Errorf("認可が拒否されました: %s", query.Get("error"))})

	case query.Get("code") != "":
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, successCallbackPage())
		c.send(callbackResult{code: query.Get("code"), state: query.Get("state")})

	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, unexpectedCallbackPage())
	}
}

// send は 1 回だけ resultCh へ結果を送る(バッファ 1) 2 回目以降のリクエストは捨てる
func (c *callbackServer) send(result callbackResult) {
	select {
	case c.resultCh <- result:
	default:
	}
}

func (c *callbackServer) close() {
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = c.server.Shutdown(ctx)
	})
}

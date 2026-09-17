package cmd

import (
	"context"

	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

type profileStoreAdapter struct {
	store *awsconfig.ProfileStore
}

// Profiles は config の profile 一覧に D9 の認証状態(sso-session の状態 / 残り時間 / 認証情報取得)を付与して返す
// awsp.BuildProfileList と同じくローカルファイル(ssocache)だけを読み ネットワークは使わない
func (a profileStoreAdapter) Profiles(ctx context.Context) ([]awsp.Profile, error) {
	items, err := a.store.ProfileDetails(ctx)
	if err != nil {
		return nil, err
	}
	sessions, err := a.store.SSOSessions(ctx)
	if err != nil {
		return nil, err
	}

	ssoCacheDir, err := ssocache.DefaultCacheDir()
	if err != nil {
		return nil, err
	}
	cliCacheDir, err := cliCacheDirOrEmpty()
	if err != nil {
		return nil, err
	}

	list, err := awsp.BuildProfileList(items, sessions, awsp.ProfileListOptions{
		ConfigFile:  a.store.ConfigPath(),
		SSOCacheDir: ssoCacheDir,
		CLICacheDir: cliCacheDir,
		Grace:       defaultGrace,
	})
	if err != nil {
		return nil, err
	}

	return list.Profiles, nil
}

func (a profileStoreAdapter) Sessions(ctx context.Context) ([]awsconfig.SSOSession, error) {
	return a.store.SSOSessions(ctx)
}

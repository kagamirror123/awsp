package ssologin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/kagamirror123/awsp/internal/awsconfig"
)

// startDeviceFlow は StartDeviceAuthorization を呼び device code 用の Flow を組み立てる
func startDeviceFlow(
	ctx context.Context,
	opts Options,
	session awsconfig.SSOSession,
	tokenPath, clientID, clientSecret string,
	registrationExpiresAt time.Time,
) (*Flow, error) {
	authOutput, err := opts.Client.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     &clientID,
		ClientSecret: &clientSecret,
		StartUrl:     &session.StartURL,
	})
	if err != nil {
		return nil, fmt.Errorf("デバイス認可の開始に失敗: %w", err)
	}

	interval := time.Duration(authOutput.Interval) * time.Second
	if interval <= 0 {
		interval = defaultPollInterval
	}

	authURL := strVal(authOutput.VerificationUriComplete)
	if authURL == "" {
		authURL = strVal(authOutput.VerificationUri)
	}

	flow := &Flow{
		Method:                MethodDeviceCode,
		AuthorizationURL:      authURL,
		UserCode:              strVal(authOutput.UserCode),
		ExpiresAt:             time.Now().Add(time.Duration(authOutput.ExpiresIn) * time.Second),
		Interval:              interval,
		client:                opts.Client,
		session:               session,
		tokenPath:             tokenPath,
		clientID:              clientID,
		clientSecret:          clientSecret,
		deviceCode:            strVal(authOutput.DeviceCode),
		registrationExpiresAt: registrationExpiresAt,
	}

	openBrowserAt(opts, flow, flow.AuthorizationURL)
	return flow, nil
}

// waitDeviceCode は device code の CreateToken をポーリングする
// ctx の期限切れ AuthorizationPendingException 以外の失敗系はすべてエラーを返す
func (f *Flow) waitDeviceCode(ctx context.Context) (Result, error) {
	sleep := f.sleepFunc
	if sleep == nil {
		sleep = sleepContext
	}

	interval := f.Interval
	if interval < 0 {
		interval = 0
	}

	for {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("ログイン待ちがタイムアウトしました: %w", err)
		}
		if time.Now().After(f.ExpiresAt) {
			return Result{}, errors.New("認可コードの有効期限が切れました: 'awsp login' をやり直してください")
		}

		output, err := f.client.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     &f.clientID,
			ClientSecret: &f.clientSecret,
			GrantType:    strPtr(deviceGrantType),
			DeviceCode:   &f.deviceCode,
		})
		if err == nil {
			return f.persistToken(output)
		}

		var pending *ssooidctypes.AuthorizationPendingException
		if errors.As(err, &pending) {
			if sleepErr := sleep(ctx, interval); sleepErr != nil {
				return Result{}, sleepErr
			}
			continue
		}

		var slowDown *ssooidctypes.SlowDownException
		if errors.As(err, &slowDown) {
			interval += 5 * time.Second
			if sleepErr := sleep(ctx, interval); sleepErr != nil {
				return Result{}, sleepErr
			}
			continue
		}

		if msg, ok := describeTokenError(err); ok {
			return Result{}, errors.New(msg)
		}
		return Result{}, fmt.Errorf("トークン取得に失敗: %w", err)
	}
}

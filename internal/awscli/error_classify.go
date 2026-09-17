package awscli

import (
	"errors"
	"strings"

	"github.com/aws/smithy-go"
)

// IsAuthRelatedError は認証起因の失敗かを判定する
// ネットワーク障害などは false にしてログイン再試行を抑止する
func IsAuthRelatedError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		// 認証と認可を区別する。AccessDenied や通信障害では再ログインしない。
		switch strings.ToLower(apiErr.ErrorCode()) {
		case "unauthorizedexception", "expiredtoken", "expiredtokenexception", "invalidtoken", "invalidtokenexception", "invalidclienttokenid", "invalid_grant", "invalidgrantexception":
			return true
		default:
			return false
		}
	}

	return hasAuthTokenHint(err.Error())
}

func hasAuthTokenHint(value string) bool {
	needle := strings.ToLower(value)
	for _, hint := range authErrorHints {
		if strings.Contains(needle, hint) {
			return true
		}
	}
	return false
}

var authErrorHints = []string{
	"unauthorizedexception",
	"sso session has expired or is invalid",
	"token has expired",
	"expired token",
	"invalid token",
	"invalidtoken",
	"ssoproviderinvalidtoken",
}

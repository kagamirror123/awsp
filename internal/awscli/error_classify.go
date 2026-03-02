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
		if hasAuthTokenHint(apiErr.ErrorCode()) {
			return true
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
	"sso session has expired or is invalid",
	"failed to refresh cached credentials",
	"token has expired",
	"expired token",
	"invalid token",
	"invalidtoken",
	"unauthorized",
	"not authorized",
	"access denied",
	"accessdenied",
	"ssoproviderinvalidtoken",
}

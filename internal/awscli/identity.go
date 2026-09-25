package awscli

import (
	"regexp"
	"strings"
)

// ssoRolePattern は IAM Identity Center が作るロール名(AWSReservedSSO_<許可セット名>_<16 桁の 16 進>)
var ssoRolePattern = regexp.MustCompile(`^AWSReservedSSO_(.+)_[0-9a-f]{16}$`)

// RoleName は caller identity の ARN から人が読むロール名を取り出す(D33)
// assumed-role ならロール名を返し IAM Identity Center のロールは許可セット名まで縮める
// IAM ユーザーなど assumed-role でない ARN や解釈できない ARN は空文字を返す
func RoleName(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) != 6 {
		return ""
	}
	resource, ok := strings.CutPrefix(parts[5], "assumed-role/")
	if !ok {
		return ""
	}
	role, _, _ := strings.Cut(resource, "/")
	if m := ssoRolePattern.FindStringSubmatch(role); m != nil {
		return m[1]
	}
	return role
}

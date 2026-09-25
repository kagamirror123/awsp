package awsp

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kagamirror123/awsp/internal/awscli"
)

// 切り替え後のカードは ARN を載せず ロール名と UserId を載せる(D33)
func TestRenderIdentityCard_ShowsRoleInsteadOfARN(t *testing.T) {
	t.Parallel()

	identity := awscli.Identity{
		Account: "123456789012",
		UserID:  "AROAEXAMPLEEXAMPLE123:you@example.com",
		ARN:     "arn:aws:sts::123456789012:assumed-role/AWSReservedSSO_AdministratorAccess_1dba405ab2e3f20c/you@example.com",
	}

	got := ansi.Strip(renderIdentityCard("dev", identity, 80))
	for _, want := range []string{"🔐 Profile : dev", "🧾 Account : 123456789012", "🎭 Role    : AdministratorAccess", "👤 UserId  : AROAEXAMPLEEXAMPLE123:you@example.com"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q が無い:\n%s", want, got)
		}
	}
	if strings.Contains(got, "arn:aws") {
		t.Errorf("ARN が載っている:\n%s", got)
	}
	// ARN を外した幅なら 80 桁の端末で枠に収まる
	if !strings.Contains(got, "╭") {
		t.Errorf("80 桁で枠が外れた:\n%s", got)
	}
}

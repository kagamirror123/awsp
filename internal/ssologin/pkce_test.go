package ssologin

import "testing"

func TestDefaultAuthorizeBaseURL(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		region    string
		dualStack bool
		want      string
	}{
		{region: "us-west-2", want: "https://oidc.us-west-2.amazonaws.com"},
		{region: "us-west-2", dualStack: true, want: "https://oidc.us-west-2.api.aws"},
		{region: "cn-north-1", want: "https://oidc.cn-north-1.amazonaws.com.cn"},
	} {
		if got := defaultAuthorizeBaseURL(tc.region, tc.dualStack); got != tc.want {
			t.Errorf("defaultAuthorizeBaseURL(%q, %v)=%s want %s", tc.region, tc.dualStack, got, tc.want)
		}
	}
}

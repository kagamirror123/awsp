package ssocache

import (
	"path/filepath"
	"testing"
)

func TestRoleCredentialPathMatchesPythonASCIIJSON(t *testing.T) {
	t.Parallel()
	// Python json.dumps(sort_keys=True, separators=(",", ":")) で独立に算出した値。
	cases := map[string]string{
		"corp":    "81003555f39f0dc0604abef2b030f38251123446",
		"社内":      "e548621fd54685c5835249d927b01d049a4fd04f",
		"team😀":   "8d3c8669dc56b131550b10c7928f8908811f6348",
		"<&>":     "8213809bcde89737e679df7464fa2a7bb3f8de44",
		"x\x7f":   "ec45d1699d9ac3cfac8738f328dabb63c706db90",
		"x\n":     "b35cae78f1779a709a79e28b1f8d692e16b22e73",
		"x\u2028": "18787f278148ea81f712c910e24bebd3c6927a32",
	}
	for name, want := range cases {
		got := RoleCredentialPath("cache", RoleCredentialKey{AccountID: "123456789012", RoleName: "ReadOnlyAccess", SessionName: name})
		if filepath.Base(got) != want+".json" {
			t.Errorf("%q: %s want %s", name, got, want)
		}
	}
}

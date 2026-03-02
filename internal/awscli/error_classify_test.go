package awscli

import "testing"

func TestIsAuthRelatedError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "auth token expired", err: &commandError{Err: errString("token has expired")}, want: true},
		{name: "auth invalid token", err: &commandError{Err: errString("SSOProviderInvalidToken")}, want: true},
		{name: "network timeout", err: &commandError{Err: errString("dial tcp: i/o timeout")}, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsAuthRelatedError(tc.err); got != tc.want {
				t.Fatalf("IsAuthRelatedError が想定外: want=%v got=%v", tc.want, got)
			}
		})
	}
}

type errString string

func (e errString) Error() string {
	return string(e)
}

package awsp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/smithy-go"
	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
	"github.com/kagamirror123/awsp/internal/ssologin"
)

type identityFunc func(context.Context, string) (awscli.Identity, error)

func (f identityFunc) CallerIdentity(ctx context.Context, profile string) (awscli.Identity, error) {
	return f(ctx, profile)
}

type recoveryOIDC struct{ registers int }

func (c *recoveryOIDC) RegisterClient(context.Context, *ssooidc.RegisterClientInput, ...func(*ssooidc.Options)) (*ssooidc.RegisterClientOutput, error) {
	c.registers++
	id, secret := "test-client", "test-secret"
	return &ssooidc.RegisterClientOutput{ClientId: &id, ClientSecret: &secret, ClientSecretExpiresAt: time.Now().Add(time.Hour).Unix()}, nil
}

func (c *recoveryOIDC) StartDeviceAuthorization(context.Context, *ssooidc.StartDeviceAuthorizationInput, ...func(*ssooidc.Options)) (*ssooidc.StartDeviceAuthorizationOutput, error) {
	code, uri := "test-code", "https://example.invalid/device"
	return &ssooidc.StartDeviceAuthorizationOutput{DeviceCode: &code, UserCode: &code, VerificationUriComplete: &uri, ExpiresIn: 600}, nil
}

func (c *recoveryOIDC) CreateToken(context.Context, *ssooidc.CreateTokenInput, ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error) {
	token := "test-token"
	return &ssooidc.CreateTokenOutput{AccessToken: &token, ExpiresIn: 3600}, nil
}

func TestLogin_RecoversOnlyAuthenticationFailures(t *testing.T) {
	for _, tc := range []struct {
		name           string
		firstError     error
		corrupt, force bool
		wantFlows      int
		wantError      bool
	}{
		{name: "valid cache"},
		{name: "revoked session", firstError: &smithy.GenericAPIError{Code: "UnauthorizedException", Message: "Session token not found or invalid"}, wantFlows: 1},
		{name: "network", firstError: errors.New("failed to refresh cached credentials: dial tcp: i/o timeout"), wantError: true},
		{name: "permissions", firstError: &smithy.GenericAPIError{Code: "AccessDenied", Message: "not authorized"}, wantError: true},
		{name: "corrupt cache", corrupt: true, wantFlows: 1},
		{name: "force corrupt cache", corrupt: true, force: true, wantFlows: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cache := t.TempDir()
			session := awsconfig.SSOSession{Name: "corp", StartURL: "https://example.invalid/start", Region: "us-west-2"}
			writeValidToken(t, cache, session.CacheKey())
			if tc.corrupt {
				if err := os.WriteFile(ssocache.TokenPath(cache, session.CacheKey()), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			client := &recoveryOIDC{}
			aws := identityFunc(func(context.Context, string) (awscli.Identity, error) {
				calls++
				if calls == 1 && tc.firstError != nil {
					return awscli.Identity{}, tc.firstError
				}
				return awscli.Identity{Account: "123456789012"}, nil
			})
			result, err := Login(context.Background(), session, "dev", LoginDeps{AWS: aws}, LoginOptions{CacheDir: cache, Force: tc.force, UseDeviceCode: true, OpenBrowser: func(string) error { return nil }, NewOIDCClient: func(string) ssologin.OIDCClient { return client }})
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v", err)
			}
			if client.registers != tc.wantFlows {
				t.Fatalf("flows=%d want=%d", client.registers, tc.wantFlows)
			}
			if !tc.wantError && (result.Identity == nil || result.Identity.Account != "123456789012" || result.Status != "ok") {
				t.Fatalf("result=%+v", result)
			}
			if tc.corrupt {
				meta, err := ssocache.ReadTokenMeta(ssocache.TokenPath(cache, session.CacheKey()))
				if err != nil || !meta.Exists {
					t.Fatalf("キャッシュが修復されていません: %v", err)
				}
			}
		})
	}
}

func TestIdentityReport_FlatVersionedJSON(t *testing.T) {
	data, err := json.Marshal(IdentityReport{SchemaVersion: 1, Identity: Identity{Profile: "dev", Account: "123", UserID: "user", ARN: "arn"}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got["schemaVersion"] != float64(1) || got["profile"] != "dev" || got["identity"] != nil {
		t.Fatalf("JSON=%s", data)
	}
}

package awscli

import "testing"

func TestRoleName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		arn  string
		want string
	}{
		{
			name: "IAM Identity Center のロールは許可セット名にする",
			arn:  "arn:aws:sts::123456789012:assumed-role/AWSReservedSSO_AdministratorAccess_1dba405ab2e3f20c/you@example.com",
			want: "AdministratorAccess",
		},
		{
			name: "許可セット名に _ を含んでも末尾の識別子だけを落とす",
			arn:  "arn:aws:sts::123456789012:assumed-role/AWSReservedSSO_Data_Reader_2878f9adf2d3f0ad/you@example.com",
			want: "Data_Reader",
		},
		{
			name: "SSO 以外の assumed-role はロール名のまま",
			arn:  "arn:aws:sts::123456789012:assumed-role/deploy-role/session",
			want: "deploy-role",
		},
		{
			name: "別パーティションでも読める",
			arn:  "arn:aws-cn:sts::123456789012:assumed-role/deploy-role/session",
			want: "deploy-role",
		},
		{
			name: "IAM ユーザーはロールが無いので空",
			arn:  "arn:aws:iam::123456789012:user/alice",
			want: "",
		},
		{
			name: "解釈できない値は空",
			arn:  "not-an-arn",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := RoleName(tc.arn); got != tc.want {
				t.Fatalf("RoleName(%q) = %q, want %q", tc.arn, got, tc.want)
			}
		})
	}
}

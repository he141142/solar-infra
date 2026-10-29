// Package githubci: GitHub Actions assume role vào AWS bằng OIDC (không dùng access key).
package githubci

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
)

const issuer = "token.actions.githubusercontent.com"

// NewProvider tạo OIDC provider của GitHub. Mỗi AWS account chỉ có MỘT provider cho issuer này.
func NewProvider(scope constructs.Construct, id string) {
	awsiam.NewOidcProviderNative(scope, jsii.String(id), &awsiam.OidcProviderNativeProps{
		Url:       jsii.String("https://" + issuer),
		ClientIds: jsii.Strings("sts.amazonaws.com"),
	})
}

// RoleName: "github-<repo>-<env>", vd. arn:aws:iam::<account>:role/github-solar-order-executor-dev.
func RoleName(repo, environment string) string { return "github-" + repo + "-" + environment }

// NewDeployRole: role chỉ assume được từ workflow của repo <owner>/<repo> khi job chạy trong
// GitHub environment = environment (dev/stg/prd). Environment trên GitHub giữ approval cho prd.
func NewDeployRole(scope constructs.Construct, id, repo, environment string) awsiam.Role {
	providerArn := awscdk.Fn_Join(jsii.String(""), &[]*string{
		jsii.String("arn:aws:iam::"), awscdk.Stack_Of(scope).Account(), jsii.String(":oidc-provider/" + issuer),
	})
	principal := awsiam.NewWebIdentityPrincipal(providerArn, &map[string]any{
		"StringEquals": map[string]any{
			issuer + ":aud": "sts.amazonaws.com",
			issuer + ":sub": "repo:" + config.GitHubOwner + "/" + repo + ":environment:" + environment,
		},
	})
	return awsiam.NewRole(scope, jsii.String(id), &awsiam.RoleProps{
		// Tên cố định = hợp đồng với GitHub: ARN biết trước nên cấu hình variable của repo trước khi deploy được.
		RoleName:           jsii.String(RoleName(repo, environment)),
		Description:        jsii.String("GitHub Actions deploy: " + config.GitHubOwner + "/" + repo + " (" + environment + ")"),
		AssumedBy:          principal,
		MaxSessionDuration: awscdk.Duration_Hours(jsii.Number(1)),
	})
}

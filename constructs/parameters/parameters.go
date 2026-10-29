// Package parameters tạo một cây parameter trong Parameter Store và cấp quyền đọc theo path.
package parameters

import (
	"maps"
	"slices"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// New tạo <path>/<key> = value cho mỗi phần tử của values.
func New(scope constructs.Construct, id string, path string, values map[string]string) constructs.Construct {
	c := constructs.NewConstruct(scope, &id)
	for _, k := range slices.Sorted(maps.Keys(values)) {
		awsssm.NewStringParameter(c, jsii.String(k), &awsssm.StringParameterProps{
			ParameterName: jsii.String(path + "/" + k),
			StringValue:   jsii.String(values[k]),
		})
	}
	return c
}

// GrantReadPath cho phép đọc mọi parameter dưới path (GetParametersByPath, kể cả path con).
func GrantReadPath(scope constructs.Construct, grantee awsiam.IGrantable, path string) awsiam.Grant {
	stack := awscdk.Stack_Of(scope)
	arn := func(name string) *string {
		return stack.FormatArn(&awscdk.ArnComponents{
			Service:      jsii.String("ssm"),
			Resource:     jsii.String("parameter"),
			ResourceName: jsii.String(strings.TrimPrefix(name, "/")),
		})
	}
	return awsiam.Grant_AddToPrincipal(&awsiam.GrantOnPrincipalOptions{
		Grantee:      grantee,
		Actions:      jsii.Strings("ssm:GetParametersByPath", "ssm:GetParameter", "ssm:GetParameters"),
		ResourceArns: &[]*string{arn(path), arn(path + "/*")},
	})
}

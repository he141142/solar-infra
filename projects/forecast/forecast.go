// Package forecast: project AI forecast.
//
//	infrastructure  VPC
//	foundation      role EC2: SSM Session Manager + publish lên central bus (account eventbus)
//	app             EC2 forecast engine
package forecast

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
	"github.com/he141142/solar-infra/constructs/forecastengine"
	"github.com/he141142/solar-infra/constructs/network"
	"github.com/he141142/solar-infra/projects/layer"
)

type Forecast struct {
	Infrastructure awscdk.Stack
	Foundation     awscdk.Stack
	App            awscdk.Stack
}

func Build(stage constructs.Construct, env config.Env) *Forecast {
	infra := layer.NewStack(stage, layer.Infrastructure, env, "Forecast - network")
	vpc := network.NewVpc(infra, "Vpc", env)

	found := layer.NewStack(stage, layer.Foundation, env, "Forecast - IAM")
	role := awsiam.NewRole(found, jsii.String("ForecastEngineRole"), &awsiam.RoleProps{
		Description: jsii.String("Forecast engine EC2"),
		AssumedBy:   awsiam.NewServicePrincipal(jsii.String("ec2.amazonaws.com"), nil),
		ManagedPolicies: &[]awsiam.IManagedPolicy{
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("AmazonSSMManagedInstanceCore")),
		},
	})
	// Cross-account: quyền phía caller; phía central bus cũng phải cho phép account này (project eventbus).
	role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions:   jsii.Strings("events:PutEvents"),
		Resources: &[]*string{env.CentralBusArn()},
	}))

	app := layer.NewStack(stage, layer.App, env, "Forecast - engine")
	engine := forecastengine.New(app, "ForecastEngine", &forecastengine.Props{
		Vpc:           vpc,
		Role:          layer.ImportRole(app, "ForecastEngineRole", role),
		CentralBusArn: env.CentralBusArn(),
	})
	layer.Output(app, "ForecastEngineInstanceId", engine.Instance.InstanceId())

	return &Forecast{Infrastructure: infra, Foundation: found, App: app}
}

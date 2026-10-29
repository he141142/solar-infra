package trading

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
	"github.com/he141142/solar-infra/constructs/githubci"
	"github.com/he141142/solar-infra/constructs/parameters"
	"github.com/he141142/solar-infra/projects/layer"
	"github.com/he141142/solar-shared/domain"
)

// Foundation: IAM.
//   - Role của workload + quyền trên tài nguyên của layer infrastructure.
//     Quyền trên tài nguyên do app tự tạo (queue, X-Ray) được cấp ở layer app.
//   - Role deploy cho GitHub Actions của từng repo app: push image + cập nhật đúng workload của nó.
type Foundation struct {
	Stack                  awscdk.Stack
	PositionCalculatorRole awsiam.IRole
	OrderExecutorTaskRole  awsiam.IRole
}

func newFoundation(stage constructs.Construct, env config.Env, infra *Infrastructure) *Foundation {
	stack := layer.NewStack(stage, layer.Foundation, env, "Trading - IAM")

	position := awsiam.NewRole(stack, jsii.String("PositionCalculatorRole"), &awsiam.RoleProps{
		Description: jsii.String("position-calculator Lambda"),
		AssumedBy:   awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
		ManagedPolicies: &[]awsiam.IManagedPolicy{
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/AWSLambdaBasicExecutionRole")),
		},
	})
	infra.Table.GrantReadWriteData(position)
	infra.Bus.GrantPutEventsTo(position, nil)
	parameters.GrantReadPath(stack, position, domain.ParamRoot+"/zones")

	executor := awsiam.NewRole(stack, jsii.String("OrderExecutorTaskRole"), &awsiam.RoleProps{
		Description: jsii.String("order-executor ECS task"),
		AssumedBy:   awsiam.NewServicePrincipal(jsii.String("ecs-tasks.amazonaws.com"), nil),
	})
	infra.Table.GrantReadWriteData(executor)
	infra.Bus.GrantPutEventsTo(executor, nil)
	parameters.GrantReadPath(stack, executor, domain.ExchangeParamPath)

	// ---------- CI deploy roles (app tự release image, không đụng CDK) ----------
	// Workload được tham chiếu bằng tên cố định (config/contracts.go) để foundation không phụ thuộc layer app.
	positionDeploy := githubci.NewDeployRole(stack, "PositionCalculatorDeployRole", config.PositionCalculatorRepo, env.Name)
	infra.PositionCalculatorRepo.GrantPullPush(positionDeploy)
	positionDeploy.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: jsii.Strings("lambda:UpdateFunctionCode", "lambda:GetFunction", "lambda:GetFunctionConfiguration"),
		Resources: &[]*string{stack.FormatArn(&awscdk.ArnComponents{
			Service:      jsii.String("lambda"),
			Resource:     jsii.String("function"),
			ResourceName: jsii.String(config.PositionCalculatorFunctionName),
			ArnFormat:    awscdk.ArnFormat_COLON_RESOURCE_NAME,
		})},
	}))

	executorDeploy := githubci.NewDeployRole(stack, "OrderExecutorDeployRole", config.OrderExecutorRepo, env.Name)
	infra.OrderExecutorRepo.GrantPullPush(executorDeploy)
	executorDeploy.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: jsii.Strings("ecs:UpdateService", "ecs:DescribeServices"),
		Resources: &[]*string{stack.FormatArn(&awscdk.ArnComponents{
			Service:      jsii.String("ecs"),
			Resource:     jsii.String("service"),
			ResourceName: jsii.String(config.OrderExecutorCluster + "/" + config.OrderExecutorService),
		})},
	}))

	layer.Output(stack, "PositionCalculatorDeployRoleArn", positionDeploy.RoleArn())
	layer.Output(stack, "OrderExecutorDeployRoleArn", executorDeploy.RoleArn())

	return &Foundation{Stack: stack, PositionCalculatorRole: position, OrderExecutorTaskRole: executor}
}

// Package positioncalculator: "vỏ" của Lambda position-calculator + rule nhận ForecastGenerated.
// Code chạy trong Lambda thuộc repo solar-position-calculator (container image trong ECR);
// infra chỉ trỏ vào tag của env, app CI đưa image mới vào bằng update-function-code.
package positioncalculator

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecr"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-shared/domain"
)

type Props struct {
	FunctionName string
	Repository   awsecr.IRepository
	ImageTag     string
	Table        awsdynamodb.ITable
	Bus          awsevents.IEventBus
	Role         awsiam.IRole // từ layer foundation
	LogRetention awslogs.RetentionDays
}

type PositionCalculator struct {
	constructs.Construct
	Function        awslambda.IFunction
	DeadLetterQueue awssqs.IQueue // event không giao được cho Lambda sau khi retry
}

func New(scope constructs.Construct, id string, props *Props) *PositionCalculator {
	c := constructs.NewConstruct(scope, &id)

	logs := awslogs.NewLogGroup(c, jsii.String("Logs"), &awslogs.LogGroupProps{
		Retention:     props.LogRetention,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})
	fn := awslambda.NewDockerImageFunction(c, jsii.String("Function"), &awslambda.DockerImageFunctionProps{
		FunctionName: jsii.String(props.FunctionName),
		Code: awslambda.DockerImageCode_FromEcr(props.Repository, &awslambda.EcrImageCodeProps{
			TagOrDigest: jsii.String(props.ImageTag),
		}),
		Architecture: awslambda.Architecture_X86_64(),
		MemorySize:   jsii.Number(256),
		Timeout:      awscdk.Duration_Seconds(jsii.Number(30)),
		Tracing:      awslambda.Tracing_ACTIVE,
		Role:         props.Role,
		LogGroup:     logs,
		Environment: &map[string]*string{
			"TABLE_NAME":     props.Table.TableName(),
			"EVENT_BUS_NAME": props.Bus.EventBusName(),
		},
	})

	dlq := awssqs.NewQueue(c, jsii.String("Dlq"), &awssqs.QueueProps{
		RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
		Encryption:      awssqs.QueueEncryption_SQS_MANAGED,
		EnforceSSL:      jsii.Bool(true),
	})
	awsevents.NewRule(c, jsii.String("ForecastRule"), &awsevents.RuleProps{
		EventBus: props.Bus,
		EventPattern: &awsevents.EventPattern{
			Source:     jsii.Strings(domain.SourceForecast),
			DetailType: jsii.Strings(domain.ForecastGenerated),
		},
		Targets: &[]awsevents.IRuleTarget{
			awseventstargets.NewLambdaFunction(fn, &awseventstargets.LambdaFunctionProps{
				RetryAttempts:   jsii.Number(2),
				DeadLetterQueue: dlq,
			}),
		},
	})

	return &PositionCalculator{Construct: c, Function: fn, DeadLetterQueue: dlq}
}

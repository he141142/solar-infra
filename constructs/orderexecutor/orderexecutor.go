// Package orderexecutor: SQS hàng đợi lệnh + "vỏ" ECS Fargate service đặt lệnh lên sàn.
// Một service phục vụ mọi zone (zone nằm trong từng TradeIntent).
// Code chạy trong container thuộc repo solar-order-executor; infra chỉ trỏ vào tag của env,
// app CI đưa image mới vào bằng update-service --force-new-deployment.
package orderexecutor

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapplicationautoscaling"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecr"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-shared/domain"
)

type Props struct {
	ClusterName    string
	ServiceName    string
	Repository     awsecr.IRepository
	ImageTag       string
	Vpc            awsec2.IVpc
	Table          awsdynamodb.ITable
	Bus            awsevents.IEventBus
	ExchangeSecret awssecretsmanager.ISecret
	TaskRole       awsiam.IRole // từ layer foundation
	LogRetention   awslogs.RetentionDays
	MaxTasks       float64
}

type OrderExecutor struct {
	constructs.Construct
	Queue           awssqs.IQueue
	DeadLetterQueue awssqs.IQueue
	Service         awsecs.FargateService
}

func New(scope constructs.Construct, id string, props *Props) *OrderExecutor {
	c := constructs.NewConstruct(scope, &id)

	dlq := awssqs.NewQueue(c, jsii.String("Dlq"), &awssqs.QueueProps{
		RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
		Encryption:      awssqs.QueueEncryption_SQS_MANAGED,
		EnforceSSL:      jsii.Bool(true),
	})
	queue := awssqs.NewQueue(c, jsii.String("Queue"), &awssqs.QueueProps{
		VisibilityTimeout: awscdk.Duration_Seconds(jsii.Number(60)),
		Encryption:        awssqs.QueueEncryption_SQS_MANAGED,
		EnforceSSL:        jsii.Bool(true),
		DeadLetterQueue:   &awssqs.DeadLetterQueue{Queue: dlq, MaxReceiveCount: jsii.Number(3)},
	})
	awsevents.NewRule(c, jsii.String("TradeIntentRule"), &awsevents.RuleProps{
		EventBus: props.Bus,
		EventPattern: &awsevents.EventPattern{
			Source:     jsii.Strings(domain.SourcePosition),
			DetailType: jsii.Strings(domain.DeficitDetected, domain.SurplusDetected),
		},
		Targets: &[]awsevents.IRuleTarget{awseventstargets.NewSqsQueue(queue, nil)},
	})

	cluster := awsecs.NewCluster(c, jsii.String("Cluster"), &awsecs.ClusterProps{
		ClusterName:         jsii.String(props.ClusterName),
		Vpc:                 props.Vpc,
		ContainerInsightsV2: awsecs.ContainerInsights_ENABLED,
	})
	taskDef := awsecs.NewFargateTaskDefinition(c, jsii.String("Task"), &awsecs.FargateTaskDefinitionProps{
		Cpu:            jsii.Number(256),
		MemoryLimitMiB: jsii.Number(512),
		TaskRole:       props.TaskRole,
		RuntimePlatform: &awsecs.RuntimePlatform{
			CpuArchitecture:       awsecs.CpuArchitecture_X86_64(),
			OperatingSystemFamily: awsecs.OperatingSystemFamily_LINUX(),
		},
	})
	taskDef.AddContainer(jsii.String("executor"), &awsecs.ContainerDefinitionOptions{
		Image: awsecs.ContainerImage_FromEcrRepository(props.Repository, jsii.String(props.ImageTag)),
		Logging: awsecs.LogDrivers_AwsLogs(&awsecs.AwsLogDriverProps{
			StreamPrefix: jsii.String("order-executor"),
			LogRetention: props.LogRetention,
		}),
		Environment: &map[string]*string{
			"QUEUE_URL":      queue.QueueUrl(),
			"TABLE_NAME":     props.Table.TableName(),
			"EVENT_BUS_NAME": props.Bus.EventBusName(),
		},
		// Execution role (do ECS tạo trong layer app) được cấp quyền đọc secret để inject vào container.
		Secrets: &map[string]awsecs.Secret{
			"EXCHANGE_MTLS": awsecs.Secret_FromSecretsManager(props.ExchangeSecret, nil),
		},
	})
	// Queue thuộc layer app nên quyền trên queue cũng cấp ở đây.
	queue.GrantConsumeMessages(props.TaskRole)

	service := awsecs.NewFargateService(c, jsii.String("Service"), &awsecs.FargateServiceProps{
		ServiceName:       jsii.String(props.ServiceName),
		Cluster:           cluster,
		TaskDefinition:    taskDef,
		DesiredCount:      jsii.Number(1),
		MinHealthyPercent: jsii.Number(100), // deploy không làm gián đoạn việc đặt lệnh
		VpcSubnets:        &awsec2.SubnetSelection{SubnetType: awsec2.SubnetType_PRIVATE_WITH_EGRESS},
		CircuitBreaker:    &awsecs.DeploymentCircuitBreaker{Rollback: jsii.Bool(true)},
	})
	// Scale theo độ dài hàng đợi (giờ cao điểm nhiều khách thiếu điện cùng lúc).
	scaling := service.AutoScaleTaskCount(&awsapplicationautoscaling.EnableScalingProps{
		MinCapacity: jsii.Number(1),
		MaxCapacity: jsii.Number(props.MaxTasks),
	})
	scaling.ScaleOnMetric(jsii.String("QueueDepth"), &awsapplicationautoscaling.BasicStepScalingPolicyProps{
		Metric:         queue.MetricApproximateNumberOfMessagesVisible(nil),
		AdjustmentType: awsapplicationautoscaling.AdjustmentType_CHANGE_IN_CAPACITY,
		ScalingSteps: &[]*awsapplicationautoscaling.ScalingInterval{
			{Upper: jsii.Number(10), Change: jsii.Number(-1)},
			{Lower: jsii.Number(100), Change: jsii.Number(1)},
			{Lower: jsii.Number(500), Change: jsii.Number(2)},
		},
	})

	return &OrderExecutor{Construct: c, Queue: queue, DeadLetterQueue: dlq, Service: service}
}

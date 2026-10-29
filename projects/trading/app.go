package trading

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatchactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssnssubscriptions"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
	"github.com/he141142/solar-infra/constructs/orderexecutor"
	"github.com/he141142/solar-infra/constructs/positioncalculator"
	"github.com/he141142/solar-infra/projects/layer"
)

// App: workload + wiring sự kiện + alarm. Đổi mỗi lần release, xoá/tạo lại không mất dữ liệu.
type App struct {
	Stack      awscdk.Stack
	Position   *positioncalculator.PositionCalculator
	Orders     *orderexecutor.OrderExecutor
	AlarmTopic awssns.ITopic
}

func newApp(stage constructs.Construct, env config.Env, infra *Infrastructure, found *Foundation) *App {
	stack := layer.NewStack(stage, layer.App, env, "Trading - workloads")

	position := positioncalculator.New(stack, "PositionCalculator", &positioncalculator.Props{
		FunctionName: config.PositionCalculatorFunctionName,
		Repository:   infra.PositionCalculatorRepo,
		ImageTag:     env.ImageTag(),
		Table:        infra.Table,
		Bus:          infra.Bus,
		Role:         layer.ImportRole(stack, "PositionCalculatorRole", found.PositionCalculatorRole),
		LogRetention: env.LogRetention,
	})
	orders := orderexecutor.New(stack, "OrderExecutor", &orderexecutor.Props{
		ClusterName:    config.OrderExecutorCluster,
		ServiceName:    config.OrderExecutorService,
		Repository:     infra.OrderExecutorRepo,
		ImageTag:       env.ImageTag(),
		Vpc:            infra.Vpc,
		Table:          infra.Table,
		Bus:            infra.Bus,
		ExchangeSecret: infra.ExchangeSecret,
		TaskRole:       layer.ImportRole(stack, "OrderExecutorTaskRole", found.OrderExecutorTaskRole),
		LogRetention:   env.LogRetention,
		MaxTasks:       env.MaxExecutorTasks,
	})
	topic := newAlarms(stack, env, position, orders)

	layer.Output(stack, "OrdersQueueUrl", orders.Queue.QueueUrl())
	layer.Output(stack, "OrderExecutorCluster", orders.Service.Cluster().ClusterName())
	layer.Output(stack, "OrderExecutorService", orders.Service.ServiceName())

	return &App{Stack: stack, Position: position, Orders: orders, AlarmTopic: topic}
}

// newAlarms: alarm cho các điểm có thể làm mất/chậm lệnh, gửi về SNS.
func newAlarms(stack awscdk.Stack, env config.Env, position *positioncalculator.PositionCalculator, orders *orderexecutor.OrderExecutor) awssns.ITopic {
	topic := awssns.NewTopic(stack, jsii.String("AlarmTopic"), &awssns.TopicProps{EnforceSSL: jsii.Bool(true)})
	if env.AlarmEmail != "" {
		topic.AddSubscription(awssnssubscriptions.NewEmailSubscription(jsii.String(env.AlarmEmail), nil))
	}
	action := awscloudwatchactions.NewSnsAction(topic)

	alarm := func(id, desc string, metric awscloudwatch.IMetric, threshold float64) {
		a := awscloudwatch.NewAlarm(stack, jsii.String(id), &awscloudwatch.AlarmProps{
			AlarmDescription:   jsii.String(desc),
			Metric:             metric,
			Threshold:          jsii.Number(threshold),
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
		a.AddAlarmAction(action)
		a.AddOkAction(action)
	}
	alarm("ForecastDlqNotEmpty", "Forecast events could not be delivered to position-calculator",
		position.DeadLetterQueue.MetricApproximateNumberOfMessagesVisible(nil), 0)
	alarm("PositionCalculatorErrors", "position-calculator Lambda is failing",
		position.Function.MetricErrors(nil), 0)
	alarm("OrdersDlqNotEmpty", "Trade intents failed 3 times and were not sent to the exchange",
		orders.DeadLetterQueue.MetricApproximateNumberOfMessagesVisible(nil), 0)
	alarm("OrdersBacklogOld", "Oldest trade intent waiting > 5 minutes (15-minute trading window at risk)",
		orders.Queue.MetricApproximateAgeOfOldestMessage(nil), 300)
	alarm("OrderExecutorCpuHigh", "order-executor CPU > 80%",
		orders.Service.MetricCpuUtilization(nil), 80)

	return topic
}

// Package eventbus: project bus sự kiện dùng chung.
//
//	infrastructure  central bus "solar-events" + archive + quyền publish cho account producer
//	app             subscriptions: forward event sang bus của account consumer
package eventbus

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
	"github.com/he141142/solar-infra/projects/layer"
	"github.com/he141142/solar-shared/domain"
)

type EventBus struct {
	Infrastructure awscdk.Stack
	App            awscdk.Stack
	Bus            awsevents.IEventBus
}

func Build(stage constructs.Construct, env config.Env) *EventBus {
	// ---------- infrastructure ----------
	infra := layer.NewStack(stage, layer.Infrastructure, env, "Event bus - central bus")
	bus := awsevents.NewEventBus(infra, jsii.String("CentralBus"), &awsevents.EventBusProps{
		EventBusName: jsii.String(config.CentralBusName),
	})
	bus.Archive(jsii.String("Archive"), &awsevents.BaseArchiveProps{
		EventPattern: &awsevents.EventPattern{Account: &[]*string{infra.Account()}},
		Retention:    awscdk.Duration_Days(jsii.Number(30)),
	})
	// Account nào được publish lên central bus.
	producers := []struct{ sid, account string }{
		{"AllowForecastPublish", env.Accounts.Forecast},
		{"AllowTradingPublish", env.Accounts.Trading},
	}
	for _, p := range producers {
		bus.AddToResourcePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Sid:        jsii.String(p.sid),
			Principals: &[]awsiam.IPrincipal{awsiam.NewAccountPrincipal(config.AccountOrCurrent(p.account))},
			Actions:    jsii.Strings("events:PutEvents"),
			Resources:  &[]*string{bus.EventBusArn()},
		}))
	}
	layer.Output(infra, "CentralBusArn", bus.EventBusArn())

	// ---------- app: subscriptions ----------
	app := layer.NewStack(stage, layer.App, env, "Event bus - subscriptions")
	dlq := awssqs.NewQueue(app, jsii.String("ForwardDlq"), &awssqs.QueueProps{
		RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
		Encryption:      awssqs.QueueEncryption_SQS_MANAGED,
		EnforceSSL:      jsii.Bool(true),
	})
	tradingBus := awsevents.EventBus_FromEventBusArn(app, jsii.String("TradingBus"), env.TradingBusArn())
	awsevents.NewRule(app, jsii.String("ForecastToTrading"), &awsevents.RuleProps{
		Description: jsii.String("Forward forecasts to the trading account"),
		EventBus:    bus,
		EventPattern: &awsevents.EventPattern{
			Source:     jsii.Strings(domain.SourceForecast),
			DetailType: jsii.Strings(domain.ForecastGenerated),
		},
		Targets: &[]awsevents.IRuleTarget{
			awseventstargets.NewEventBus(tradingBus, &awseventstargets.EventBusProps{DeadLetterQueue: dlq}),
		},
	})

	return &EventBus{Infrastructure: infra, App: app, Bus: bus}
}

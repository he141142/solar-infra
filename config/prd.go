package config

import "github.com/aws/aws-cdk-go/awscdk/v2/awslogs"

// prd: mỗi project một account, HA (NAT mỗi AZ), giữ dữ liệu.
var prd = Env{
	Name:   "prd",
	Region: "ap-southeast-1",
	Accounts: Accounts{ // TODO: điền account ID production
		EventBus: "",
		Trading:  "",
		Forecast: "",
	},
	MaxAzs:           3,
	NatGateways:      3,
	Production:       true,
	LogRetention:     awslogs.RetentionDays_THREE_MONTHS,
	AlarmEmail:       "", // TODO: email/on-call nhận alarm
	MaxExecutorTasks: 4,
	Exchange:         Exchange{Endpoint: "https://exchange.example.com/api/v1", Mode: "live"},
	Zones:            zones,
}

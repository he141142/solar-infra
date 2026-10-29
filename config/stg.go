package config

import "github.com/aws/aws-cdk-go/awscdk/v2/awslogs"

// stg: giống prd về cấu trúc (account riêng, giữ dữ liệu), nhỏ hơn về quy mô, nối sàn test.
var stg = Env{
	Name:   "stg",
	Region: "ap-southeast-1",
	Accounts: Accounts{ // TODO: điền account ID staging
		EventBus: "",
		Trading:  "",
		Forecast: "",
	},
	MaxAzs:           2,
	NatGateways:      1,
	Production:       true,
	LogRetention:     awslogs.RetentionDays_ONE_MONTH,
	MaxExecutorTasks: 2,
	Exchange:         Exchange{Endpoint: "https://sandbox.exchange.example.com/api/v1", Mode: "live"},
	Zones:            zones,
}

package config

import "github.com/aws/aws-cdk-go/awscdk/v2/awslogs"

var zones = []Zone{
	{Name: "north", DeficitThresholdKwh: 5, MaxBuyPrice: 3200, MinSellPrice: 1600, PlantCapacityKwh: 800},
	{Name: "central", DeficitThresholdKwh: 5, MaxBuyPrice: 3000, MinSellPrice: 1500, PlantCapacityKwh: 400},
	{Name: "south", DeficitThresholdKwh: 5, MaxBuyPrice: 3400, MinSellPrice: 1700, PlantCapacityKwh: 1000},
}

// dev: rẻ, xoá được. Cho phép mọi project dùng chung account của credentials hiện tại.
var dev = Env{
	Name:             "dev",
	Region:           "ap-southeast-1",
	MaxAzs:           2,
	NatGateways:      1,
	LogRetention:     awslogs.RetentionDays_ONE_WEEK,
	MaxExecutorTasks: 2,
	Exchange:         Exchange{Endpoint: "https://exchange.example.com/api/v1", Mode: "mock"},
	Zones:            zones,
}

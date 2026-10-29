// Package config chứa cấu hình hạ tầng theo môi trường.
//
// Mô hình (theo AWS multi-account strategy):
//   - Mỗi project (domain) × env là một AWS account: eventbus-dev, trading-prd, ...
//   - Trong project, hạ tầng chia layer theo volatility: infrastructure -> foundation -> app.
//   - Zone (Bắc/Trung/Nam) là DỮ LIỆU của project trading, không phải hạ tầng.
package config

import (
	"fmt"
	"os"

	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
)

// Zone là khu vực kinh doanh. Được đẩy vào Parameter Store của project trading.
type Zone struct {
	Name                string
	DeficitThresholdKwh float64 // chênh lệch nhỏ hơn ngưỡng thì bỏ qua
	MaxBuyPrice         float64 // VND/kWh - giá trần khi mua trên sàn
	MinSellPrice        float64 // VND/kWh - giá sàn khi bán phần dư
	PlantCapacityKwh    float64 // công suất nhà máy của mình cho zone, mỗi khung 15 phút
}

// Accounts: AWS account của từng project. "" = account của credentials đang dùng (chỉ cho non-production).
type Accounts struct {
	EventBus string
	Trading  string
	Forecast string
}

type Exchange struct {
	Endpoint string
	Mode     string // mock | live
}

type Env struct {
	Name             string
	Region           string
	Accounts         Accounts
	MaxAzs           float64
	NatGateways      float64
	Production       bool // RETAIN dữ liệu, PITR, deletion/termination protection, VPC flow logs
	LogRetention     awslogs.RetentionDays
	AlarmEmail       string // "" = không gửi email, alarm vẫn được tạo
	MaxExecutorTasks float64
	Exchange         Exchange
	Zones            []Zone
}

var envs = map[string]Env{
	dev.Name: dev,
	stg.Name: stg,
	prd.Name: prd,
}

func Get(name string) (Env, error) {
	e, ok := envs[name]
	if !ok {
		return Env{}, fmt.Errorf("unknown env %q (expected dev|stg|prd)", name)
	}
	if err := e.Validate(); err != nil {
		return Env{}, err
	}
	return e, nil
}

func (e Env) Validate() error {
	if !e.Production {
		return nil
	}
	a := e.Accounts
	if a.EventBus == "" || a.Trading == "" || a.Forecast == "" {
		return fmt.Errorf("env %q: every project needs its own AWS account in production", e.Name)
	}
	if a.EventBus == a.Trading || a.EventBus == a.Forecast || a.Trading == a.Forecast {
		return fmt.Errorf("env %q: projects must not share an AWS account in production", e.Name)
	}
	return nil
}

// ResolveAccount trả về account đã cấu hình, nếu trống thì lấy từ CLI (CDK_DEFAULT_ACCOUNT).
func ResolveAccount(account string) string {
	if account != "" {
		return account
	}
	return os.Getenv("CDK_DEFAULT_ACCOUNT")
}

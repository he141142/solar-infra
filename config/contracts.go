package config

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
)

// Hợp đồng giữa các project. Project ở account khác nhau không truyền construct cho nhau được,
// nên tham chiếu qua tên/ARN cố định.
const (
	CentralBusName = "solar-events"  // project eventbus: bus dùng chung toàn hệ thống
	TradingBusName = "solar-trading" // project trading: bus nội bộ, nhận event được forward từ central
)

// Hợp đồng deploy giữa repo hạ tầng và repo ứng dụng (infra tạo "vỏ", app CI đưa image vào).
// Tên cố định để workflow GitHub Actions của app biết deploy vào đâu; đổi ở đây phải đổi cả workflow.
const (
	GitHubOwner = "he141142"

	InfraRepo = "solar-infra"

	PositionCalculatorRepo         = "solar-position-calculator" // GitHub repo
	PositionCalculatorEcrRepo      = "solar/position-calculator"
	PositionCalculatorFunctionName = "solar-position-calculator"

	OrderExecutorRepo    = "solar-order-executor" // GitHub repo
	OrderExecutorEcrRepo = "solar/order-executor"
	OrderExecutorCluster = "solar-trading"
	OrderExecutorService = "order-executor"
)

// ImageTag: workload chạy image có tag = tên env (":dev", ":prd"). App CI push ":<git-sha>" rồi dời tag env
// sang image mới và gọi update. CDK luôn khai báo cùng một tag nên deploy lại infra không kéo image về bản cũ.
func (e Env) ImageTag() string { return e.Name }

func (e Env) CentralBusArn() *string { return busArn(e, e.Accounts.EventBus, CentralBusName) }
func (e Env) TradingBusArn() *string { return busArn(e, e.Accounts.Trading, TradingBusName) }

// AccountOrCurrent: account cố định của project, hoặc AWS::AccountId khi dev chạy chung một account.
func AccountOrCurrent(account string) *string {
	if a := ResolveAccount(account); a != "" {
		return jsii.String(a)
	}
	return awscdk.Aws_ACCOUNT_ID()
}

func busArn(e Env, account, name string) *string {
	return awscdk.Fn_Join(jsii.String(""), &[]*string{
		jsii.String("arn:aws:events:" + e.Region + ":"),
		AccountOrCurrent(account),
		jsii.String(":event-bus/" + name),
	})
}

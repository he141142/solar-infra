// Package trading: project giao dịch năng lượng.
// Một bộ hạ tầng phục vụ mọi zone; zone là dữ liệu (Parameter Store, key DynamoDB, trường trong event).
//
//	central bus --forward--> trading bus --ForecastGenerated--> Lambda position-calculator
//	Lambda --DeficitDetected/SurplusDetected--> trading bus --> SQS --> ECS order-executor --mTLS--> sàn
package trading

import (
	"github.com/aws/constructs-go/constructs/v10"

	"github.com/he141142/solar-infra/config"
)

type Trading struct {
	Infrastructure *Infrastructure
	Foundation     *Foundation
	App            *App
}

func Build(stage constructs.Construct, env config.Env) *Trading {
	infra := newInfrastructure(stage, env)
	found := newFoundation(stage, env, infra)
	app := newApp(stage, env, infra, found)
	return &Trading{Infrastructure: infra, Foundation: found, App: app}
}

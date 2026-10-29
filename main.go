package main

import (
	"fmt"
	"os"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
	"github.com/he141142/solar-infra/projects/cicd"
	"github.com/he141142/solar-infra/projects/eventbus"
	"github.com/he141142/solar-infra/projects/forecast"
	"github.com/he141142/solar-infra/projects/layer"
	"github.com/he141142/solar-infra/projects/trading"
)

// cdk deploy -c env=dev "trading-dev/*"             mọi layer của 1 project
// cdk deploy -c env=dev "trading-dev/app"           chỉ layer app
// cdk deploy -c env=dev -c project=trading --all    lọc theo project
func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)
	env, err := config.Get(contextString(app, "env", "dev"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	only := contextString(app, "project", "")

	projects := []struct {
		name    string
		account string
		build   func(constructs.Construct, config.Env)
	}{
		{"eventbus", env.Accounts.EventBus, func(s constructs.Construct, e config.Env) { eventbus.Build(s, e) }},
		{"trading", env.Accounts.Trading, func(s constructs.Construct, e config.Env) { trading.Build(s, e) }},
		{"forecast", env.Accounts.Forecast, func(s constructs.Construct, e config.Env) { forecast.Build(s, e) }},
	}
	for _, p := range projects {
		if only == "" || only == p.name {
			p.build(layer.NewStage(app, p.name, env, p.account), env)
		}
	}

	// cicd: một stage cho mỗi AWS account khác nhau (dev dùng chung account -> chỉ một stage "cicd-dev").
	if only == "" || only == "cicd" {
		accounts := map[string]string{} // account -> project đầu tiên dùng account đó
		for _, p := range projects {
			if _, ok := accounts[config.ResolveAccount(p.account)]; !ok {
				accounts[config.ResolveAccount(p.account)] = p.name
			}
		}
		for _, p := range projects {
			acct := config.ResolveAccount(p.account)
			if accounts[acct] != p.name {
				continue
			}
			name := "cicd"
			if len(accounts) > 1 {
				name = "cicd-" + p.name
			}
			cicd.Build(layer.NewStage(app, name, env, p.account), env)
		}
	}

	app.Synth(nil)
}

func contextString(app awscdk.App, key, fallback string) string {
	if v, ok := app.Node().TryGetContext(jsii.String(key)).(string); ok && v != "" {
		return v
	}
	return fallback
}

package trading

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
	"github.com/he141142/solar-infra/projects/layer"
)

func testEnv(production bool) config.Env {
	return config.Env{
		Name: "test", Region: "ap-southeast-1",
		Accounts: config.Accounts{EventBus: "111111111111", Trading: "222222222222", Forecast: "333333333333"},
		MaxAzs:   2, NatGateways: 1, Production: production, MaxExecutorTasks: 2,
		LogRetention: awslogs.RetentionDays_ONE_WEEK,
		Exchange:     config.Exchange{Endpoint: "https://exchange.test", Mode: "mock"},
		Zones: []config.Zone{
			{Name: "north", DeficitThresholdKwh: 5, MaxBuyPrice: 3200, MinSellPrice: 1600, PlantCapacityKwh: 800},
			{Name: "south", DeficitThresholdKwh: 5, MaxBuyPrice: 3400, MinSellPrice: 1700, PlantCapacityKwh: 1000},
		},
	}
}

func build(production bool) *Trading {
	env := testEnv(production)
	return Build(layer.NewStage(awscdk.NewApp(nil), "trading", env, env.Accounts.Trading), env)
}

func TestDevDataIsDisposable(t *testing.T) {
	tmpl := assertions.Template_FromStack(build(false).Infrastructure.Stack, nil)
	tmpl.HasResource(jsii.String("AWS::DynamoDB::Table"), map[string]any{"DeletionPolicy": "Delete"})
}

func TestProdDataIsProtected(t *testing.T) {
	tr := build(true)
	tmpl := assertions.Template_FromStack(tr.Infrastructure.Stack, nil)

	tmpl.HasResource(jsii.String("AWS::DynamoDB::Table"), map[string]any{
		"DeletionPolicy": "Retain",
		"Properties": map[string]any{
			"DeletionProtectionEnabled":        true,
			"PointInTimeRecoverySpecification": map[string]any{"PointInTimeRecoveryEnabled": true},
		},
	})
	tmpl.HasResource(jsii.String("AWS::SecretsManager::Secret"), map[string]any{"DeletionPolicy": "Retain"})
	if !*tr.Infrastructure.Stack.TerminationProtection() || !*tr.Foundation.Stack.TerminationProtection() {
		t.Error("infrastructure and foundation must have termination protection in production")
	}
	if *tr.App.Stack.TerminationProtection() {
		t.Error("app layer must stay freely deployable")
	}
}

// Zone là dữ liệu: mỗi zone chỉ thêm parameter, không thêm hạ tầng.
func TestZonesAreDataNotInfrastructure(t *testing.T) {
	tr := build(false)
	tmpl := assertions.Template_FromStack(tr.Infrastructure.Stack, nil)

	tmpl.ResourceCountIs(jsii.String("AWS::SSM::Parameter"), jsii.Number(2*4+2)) // 2 zone x 4 + exchange x 2
	tmpl.ResourceCountIs(jsii.String("AWS::DynamoDB::Table"), jsii.Number(1))
	tmpl.HasResourceProperties(jsii.String("AWS::SSM::Parameter"), map[string]any{
		"Name": "/solar-trading/zones/south/max-buy-price", "Value": "3400",
	})
}

func TestCentralBusMayForwardIntoTradingBus(t *testing.T) {
	tmpl := assertions.Template_FromStack(build(false).Infrastructure.Stack, nil)
	policies := tmpl.FindResources(jsii.String("AWS::Events::EventBusPolicy"), nil)
	b, err := json.Marshal(policies)
	if err != nil {
		t.Fatal(err)
	}
	// Principal là Fn::Join("arn:aws:iam::", <account eventbus>, ":root").
	if len(*policies) != 1 || !strings.Contains(string(b), "111111111111") {
		t.Errorf("trading bus must allow the eventbus account to put events, got %s", b)
	}
}

func TestLayersDependOneWay(t *testing.T) {
	tr := build(false)
	deps := func(s awscdk.Stack) map[string]bool {
		m := map[string]bool{}
		for _, d := range *s.Dependencies() {
			m[*d.StackName()] = true
		}
		return m
	}
	infra, found, app := tr.Infrastructure.Stack, tr.Foundation.Stack, tr.App.Stack
	assertions.Template_FromStack(app, nil) // synth để CDK tính dependency

	if len(deps(infra)) != 0 {
		t.Errorf("infrastructure must not depend on other layers, got %v", deps(infra))
	}
	if d := deps(found); !d[*infra.StackName()] || d[*app.StackName()] {
		t.Errorf("foundation must depend only on infrastructure, got %v", d)
	}
	if d := deps(app); !d[*found.StackName()] {
		t.Errorf("app must depend on foundation, got %v", d)
	}
}

func TestAppWiring(t *testing.T) {
	tmpl := assertions.Template_FromStack(build(false).App.Stack, nil)

	tmpl.ResourceCountIs(jsii.String("AWS::Events::Rule"), jsii.Number(2))
	tmpl.ResourceCountIs(jsii.String("AWS::ECS::Service"), jsii.Number(1))
	// Lambda là container image do app CI đưa vào; infra chỉ giữ tên cố định để CI tìm được.
	tmpl.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]any{
		"FunctionName":  "solar-position-calculator",
		"PackageType":   "Image",
		"TracingConfig": map[string]any{"Mode": "Active"},
	})
	tmpl.HasResourceProperties(jsii.String("AWS::ECS::Service"), map[string]any{"ServiceName": "order-executor"})
	// Mọi queue đều mã hoá.
	tmpl.AllResourcesProperties(jsii.String("AWS::SQS::Queue"), map[string]any{"SqsManagedSseEnabled": true})
	// 5 alarm nghiệp vụ + 2 alarm do step scaling tạo.
	tmpl.ResourceCountIs(jsii.String("AWS::CloudWatch::Alarm"), jsii.Number(7))
}

// Workload chạy tag của env (":test"), không phải tag cố định của một build -> deploy lại infra không kéo về image cũ.
func TestWorkloadsRunEnvImageTag(t *testing.T) {
	tmpl := assertions.Template_FromStack(build(false).App.Stack, nil)
	b, err := json.Marshal(tmpl.ToJSON())
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), ":test"); n < 2 {
		t.Errorf("expected Lambda and ECS to reference image tag :test, found %d references", n)
	}
}

// Mỗi repo app chỉ assume được role của chính nó, và chỉ từ đúng GitHub environment.
func TestCIRolesTrustOneRepoAndEnvironment(t *testing.T) {
	tmpl := assertions.Template_FromStack(build(false).Foundation.Stack, nil)
	for _, repo := range []string{"solar-position-calculator", "solar-order-executor"} {
		tmpl.HasResourceProperties(jsii.String("AWS::IAM::Role"), map[string]any{
			"AssumeRolePolicyDocument": map[string]any{
				"Statement": []any{map[string]any{
					"Action": "sts:AssumeRoleWithWebIdentity",
					"Condition": map[string]any{"StringEquals": map[string]any{
						"token.actions.githubusercontent.com:sub": "repo:he141142/" + repo + ":environment:test",
						"token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
					}},
				}},
			},
		})
	}
}

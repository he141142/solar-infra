# solar-infra

Infrastructure (AWS CDK, Go) for the solar energy trading POC. **This repo only holds resource definitions**; the code that runs inside them lives in separate repos, each with its own release cycle.

## Repositories
| Repo | Contents | Release |
|---|---|---|
| **solar-infra** (this repo) | VPC, DynamoDB, EventBridge, SQS, ECR, IAM, the Lambda/ECS/EC2 "shells", alarms | `infra.yml`: PR → synth; `main` → dev; manual run → stg/prd |
| [solar-shared](https://github.com/he141142/solar-shared) | Event contract, param paths, shared helpers | Semver tag; each repo pins its version |
| [solar-position-calculator](https://github.com/he141142/solar-position-calculator) | Lambda code (container image) | Build → ECR → `update-function-code` |
| [solar-order-executor](https://github.com/he141142/solar-order-executor) | ECS worker code | Build → ECR → `update-service` |

**Infra ↔ app contract** (`config/contracts.go`): ECR repo names, Lambda function name, ECS cluster/service names. Workloads always point at the image tag `:<env>`. App CI pushes `:<git-sha>`, moves `:<env>`, then calls update. Redeploying infra still declares `:<env>`, so it never rolls an app back to an old image.

## Infrastructure model
- **Project (domain) × env = one AWS account** (`trading-prd`, `eventbus-dev`, ...). dev may share a single account.
- Each project is split into **layers by volatility**, deployed in order with one-way dependencies: `infrastructure` → `foundation` (IAM) → `app`.
- **Zones are data** (Parameter Store `/solar-trading/zones/<zone>/*`, DynamoDB keys, a `zone` field in events), not infrastructure.

```
config/        dev.go / stg.go / prd.go, contracts.go
projects/      layer/ (conventions) · cicd/ · eventbus/ · trading/ · forecast/
constructs/    network · parameters · githubci · positioncalculator · orderexecutor · forecastengine
```

| Stage | infrastructure | foundation | app |
|---|---|---|---|
| `cicd-<env>` (once per account) | — | GitHub OIDC provider + `InfraDeployRole` | — |
| `trading-<env>` | VPC, DynamoDB, ECR ×2, params, secret, trading bus | Workload roles + **deploy roles for the 2 app repos** | Lambda, ECS, SQS, rules, alarms |
| `eventbus-<env>` | Central bus + producer permissions | — | Forward rule → trading bus |
| `forecast-<env>` | VPC | EC2 role | EC2 forecast engine (mock) |

## Bootstrap a new environment (dev example, done once)
```bash
# 0. Local tooling: go env -w GOTOOLCHAIN=auto   (CDK needs Go >= 1.26)
# 1. CDK bootstrap + CI/CD (OIDC + InfraDeployRole)
npx aws-cdk@2 bootstrap
npx aws-cdk@2 deploy -c project=cicd "cicd-dev/*"
# 2. Durable layers of trading: creates ECR + deploy roles for the app repos
npx aws-cdk@2 deploy -c project=trading "trading-dev/infrastructure" "trading-dev/foundation"
# 3. Run the workflow in solar-position-calculator and solar-order-executor once
#    -> pushes :dev images to ECR (the update step is skipped because the workload doesn't exist yet)
# 4. Workloads + the other projects
npx aws-cdk@2 deploy -c project=trading "trading-dev/app"
npx aws-cdk@2 deploy -c project=eventbus "eventbus-dev/*"
npx aws-cdk@2 deploy -c project=forecast "forecast-dev/*"
```
From then on: infra changes → PR into this repo; code changes → push to the app repo. The two flows don't touch each other.

## One-time GitHub setup
| Repo | Environment `dev`/`stg`/`prd` → secret | Value (stack output) |
|---|---|---|
| solar-infra | `AWS_INFRA_ROLE_TRADING`, `AWS_INFRA_ROLE_EVENTBUS`, `AWS_INFRA_ROLE_FORECAST` | `InfraDeployRoleArn` of `cicd-*` (dev: the same value for all 3) |
| solar-position-calculator | `AWS_DEPLOY_ROLE_ARN` | `PositionCalculatorDeployRoleArn` of `trading-<env>-foundation` |
| solar-order-executor | `AWS_DEPLOY_ROLE_ARN` | `OrderExecutorDeployRoleArn` of `trading-<env>-foundation` |

- If `solar-shared` is private, add secret `GO_MODULES_TOKEN` to the 3 consumer repos.
- Enable required reviewers on the `stg` and `prd` environments.
- Roles only trust `repo:he141142/<repo>:environment:<env>`: another repo, or a job without the right environment, cannot assume them.

stg/prd: fill in `Accounts` in `config/stg.go` / `config/prd.go` (one account per project; synth refuses if they are missing or shared), then bootstrap each account the same way.

## Local development
The `E:\alexd\lfs\go.work` file (outside every repo, never committed) makes the repos use `solar-shared` straight from disk. CI uses the tagged version.

```bash
go test ./...                                  # infra tests: data protection, one-way layers, zones as data, CI trust
npx aws-cdk@2 synth -c env=dev "**"
npx aws-cdk@2 diff  -c project=trading "trading-dev/*"
```

## Manual testing / operations
```bash
# Publish a forecast to the central bus (like the forecast engine does)
aws events put-events --entries '[{"Source":"solar.forecast","DetailType":"ForecastGenerated","EventBusName":"solar-events",
  "Detail":"{\"customerId\":\"SCHOOL-001\",\"zone\":\"north\",\"interval\":\"2026-10-29T10:00:00Z\",\"forecastProductionKwh\":50,\"forecastConsumptionKwh\":1200,\"model\":\"manual\"}"}]'
# Change a zone's config (takes effect within ~5 minutes, no deploy needed)
aws ssm put-parameter --name /solar-trading/zones/north/max-buy-price --value 3300 --overwrite
# Connect to the real exchange
aws secretsmanager put-secret-value --secret-id <ExchangeSecretArn> --secret-string file://exchange-mtls.json
aws ssm put-parameter --name /solar-trading/exchange/mode --value live --overwrite
```

## Cleanup (dev)
Reverse order: `forecast-dev` → `eventbus-dev` → `trading-dev` → `cicd-dev`. In dev, ECR is emptied and deleted along with the stack. Costs to watch: 2 NAT Gateways (trading + forecast), EC2 t3.small, Fargate.

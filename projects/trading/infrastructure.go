package trading

import (
	"strconv"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecr"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-infra/config"
	"github.com/he141142/solar-infra/constructs/network"
	"github.com/he141142/solar-infra/constructs/parameters"
	"github.com/he141142/solar-infra/projects/layer"
	"github.com/he141142/solar-shared/domain"
)

// Infrastructure: tài nguyên bền vững. Production giữ lại khi xoá stack, bật PITR + deletion protection.
type Infrastructure struct {
	Stack          awscdk.Stack
	Vpc            awsec2.IVpc
	Table          awsdynamodb.ITable
	Bus            awsevents.IEventBus
	ExchangeSecret awssecretsmanager.ISecret

	// Nơi app CI push image. Nằm ở layer bền vững vì image tồn tại lâu hơn mọi lần deploy app.
	PositionCalculatorRepo awsecr.IRepository
	OrderExecutorRepo      awsecr.IRepository
}

func newInfrastructure(stage constructs.Construct, env config.Env) *Infrastructure {
	stack := layer.NewStack(stage, layer.Infrastructure, env, "Trading - durable resources")
	removal := awscdk.RemovalPolicy_DESTROY
	if env.Production {
		removal = awscdk.RemovalPolicy_RETAIN
	}

	vpc := network.NewVpc(stack, "Vpc", env)

	positionRepo := newRepository(stack, "PositionCalculatorRepo", config.PositionCalculatorEcrRepo, env, removal)
	executorRepo := newRepository(stack, "OrderExecutorRepo", config.OrderExecutorEcrRepo, env, removal)

	// Single-table, zone nằm trong key:
	//   CUSTOMER#<id>  / POSITION#<interval>   vị thế dự báo của khách hàng (có thuộc tính zone)
	//   ZONE#<zone>    / PLANT_POOL#<interval> phần công suất nhà máy đã cấp cho zone
	//   ORDER#<intent> / META                  lệnh trên sàn (GSI1: ORDERS#<status> / interval)
	table := awsdynamodb.NewTable(stack, jsii.String("TradingTable"), &awsdynamodb.TableProps{
		PartitionKey:        &awsdynamodb.Attribute{Name: jsii.String("PK"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:             &awsdynamodb.Attribute{Name: jsii.String("SK"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:         awsdynamodb.BillingMode_PAY_PER_REQUEST,
		TimeToLiveAttribute: jsii.String("ttl"),
		DeletionProtection:  jsii.Bool(env.Production),
		PointInTimeRecoverySpecification: &awsdynamodb.PointInTimeRecoverySpecification{
			PointInTimeRecoveryEnabled: jsii.Bool(env.Production),
		},
		RemovalPolicy: removal,
	})
	table.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
		IndexName:    jsii.String("GSI1"),
		PartitionKey: &awsdynamodb.Attribute{Name: jsii.String("GSI1PK"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: jsii.String("GSI1SK"), Type: awsdynamodb.AttributeType_STRING},
	})

	// Zone là dữ liệu: thêm/bớt zone = thêm/bớt nhánh parameter, không thêm hạ tầng.
	for _, z := range env.Zones {
		parameters.New(stack, "Zone-"+z.Name, domain.ZoneParamPath(z.Name), map[string]string{
			domain.ParamDeficitThreshold: ftoa(z.DeficitThresholdKwh),
			domain.ParamMaxBuyPrice:      ftoa(z.MaxBuyPrice),
			domain.ParamMinSellPrice:     ftoa(z.MinSellPrice),
			domain.ParamPlantCapacity:    ftoa(z.PlantCapacityKwh),
		})
	}
	parameters.New(stack, "Exchange", domain.ExchangeParamPath, map[string]string{
		domain.ParamExchangeEndpoint: env.Exchange.Endpoint,
		domain.ParamExchangeMode:     env.Exchange.Mode,
	})

	// Cert/key mTLS + API key của sàn. Nạp giá trị thật sau khi deploy (xem README).
	secret := awssecretsmanager.NewSecret(stack, jsii.String("ExchangeMtls"), &awssecretsmanager.SecretProps{
		Description: jsii.String("Client cert/key + CA for the energy exchange API"),
		GenerateSecretString: &awssecretsmanager.SecretStringGenerator{
			SecretStringTemplate: jsii.String(`{"certPem":"","keyPem":"","caPem":""}`),
			GenerateStringKey:    jsii.String("apiKey"),
			ExcludePunctuation:   jsii.Bool(true),
		},
		RemovalPolicy: removal,
	})

	// Bus nội bộ của trading; central bus (account eventbus) được phép forward event vào.
	bus := awsevents.NewEventBus(stack, jsii.String("Bus"), &awsevents.EventBusProps{
		EventBusName: jsii.String(config.TradingBusName),
	})
	bus.AddToResourcePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Sid:        jsii.String("AllowCentralBusForward"),
		Principals: &[]awsiam.IPrincipal{awsiam.NewAccountPrincipal(config.AccountOrCurrent(env.Accounts.EventBus))},
		Actions:    jsii.Strings("events:PutEvents"),
		Resources:  &[]*string{bus.EventBusArn()},
	}))
	bus.Archive(jsii.String("Archive"), &awsevents.BaseArchiveProps{
		EventPattern: &awsevents.EventPattern{Account: &[]*string{stack.Account()}},
		Retention:    awscdk.Duration_Days(jsii.Number(30)),
	})

	layer.Output(stack, "TableName", table.TableName())
	layer.Output(stack, "TradingBusArn", bus.EventBusArn())
	layer.Output(stack, "ExchangeSecretArn", secret.SecretArn())

	return &Infrastructure{
		Stack: stack, Vpc: vpc, Table: table, Bus: bus, ExchangeSecret: secret,
		PositionCalculatorRepo: positionRepo, OrderExecutorRepo: executorRepo,
	}
}

// newRepository: ECR repo cho một app. Tag mutable vì tag env (":dev") được dời sang image mới;
// tag ":<git-sha>" thì không bao giờ ghi đè nên vẫn truy vết/rollback được.
func newRepository(stack awscdk.Stack, id, name string, env config.Env, removal awscdk.RemovalPolicy) awsecr.IRepository {
	repo := awsecr.NewRepository(stack, jsii.String(id), &awsecr.RepositoryProps{
		RepositoryName:     jsii.String(name),
		ImageScanOnPush:    jsii.Bool(true),
		ImageTagMutability: awsecr.TagMutability_MUTABLE,
		RemovalPolicy:      removal,
		EmptyOnDelete:      jsii.Bool(!env.Production),
	})
	repo.AddLifecycleRule(&awsecr.LifecycleRule{
		Description:   jsii.String("Keep the 30 most recent images"),
		TagStatus:     awsecr.TagStatus_ANY,
		MaxImageCount: jsii.Number(30),
	})
	layer.Output(stack, id+"Uri", repo.RepositoryUri())
	return repo
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// Package forecastengine: EC2 chạy AI forecast, phát ForecastGenerated lên central bus mỗi 15 phút.
// POC: script mock sinh số ngẫu nhiên. Production: thay bằng model thật đóng gói sẵn (AMI/container).
package forecastengine

import (
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/he141142/solar-shared/domain"
)

type Props struct {
	Vpc           awsec2.IVpc
	Role          awsiam.IRole // từ layer foundation
	CentralBusArn *string      // bus ở account eventbus
}

type ForecastEngine struct {
	constructs.Construct
	Instance awsec2.Instance
}

func New(scope constructs.Construct, id string, props *Props) *ForecastEngine {
	c := constructs.NewConstruct(scope, &id)

	userData := awsec2.UserData_ForLinux(nil)
	userData.AddCommands(jsii.String(strings.NewReplacer(
		"__REGION__", *awscdk.Stack_Of(c).Region(),
		"__BUS__", *props.CentralBusArn, // cross-account put-events phải dùng ARN
		"__SOURCE__", domain.SourceForecast,
		"__DETAIL_TYPE__", domain.ForecastGenerated,
	).Replace(userDataTmpl)))

	// Không mở port nào; truy cập qua SSM Session Manager.
	instance := awsec2.NewInstance(c, jsii.String("Instance"), &awsec2.InstanceProps{
		Vpc:                       props.Vpc,
		VpcSubnets:                &awsec2.SubnetSelection{SubnetType: awsec2.SubnetType_PRIVATE_WITH_EGRESS},
		InstanceType:              awsec2.InstanceType_Of(awsec2.InstanceClass_BURSTABLE3, awsec2.InstanceSize_SMALL),
		MachineImage:              awsec2.MachineImage_LatestAmazonLinux2023(nil),
		Role:                      props.Role,
		UserData:                  userData,
		UserDataCausesReplacement: jsii.Bool(true),
		RequireImdsv2:             jsii.Bool(true),
		BlockDevices: &[]*awsec2.BlockDevice{{
			DeviceName: jsii.String("/dev/xvda"),
			Volume:     awsec2.BlockDeviceVolume_Ebs(jsii.Number(20), &awsec2.EbsDeviceOptions{Encrypted: jsii.Bool(true)}),
		}},
	})
	return &ForecastEngine{Construct: c, Instance: instance}
}

// Khách hàng mẫu "zone:customer" - zone là dữ liệu trong event, không phải hạ tầng.
const userDataTmpl = `dnf install -y jq
mkdir -p /opt/forecast
cat > /opt/forecast/run.sh <<'EOF'
#!/bin/bash
# MOCK AI forecast: thay bằng model thật. Phát forecast cho khung 15 phút kế tiếp.
set -euo pipefail
NEXT=$(( ($(date -u +%s) / 900 + 1) * 900 ))
INTERVAL=$(date -u -d "@$NEXT" +%Y-%m-%dT%H:%M:00Z)
for ENTRY in north:SCHOOL-001 north:HOUSE-001 central:ORG-001 south:HOUSE-002; do
  ZONE=${ENTRY%%:*}
  CUSTOMER=${ENTRY#*:}
  PROD=$((RANDOM % 400))
  CONS=$((RANDOM % 500))
  DETAIL=$(jq -cn --arg c "$CUSTOMER" --arg z "$ZONE" --arg i "$INTERVAL" --argjson p "$PROD" --argjson k "$CONS" \
    '{customerId:$c, zone:$z, interval:$i, forecastProductionKwh:$p, forecastConsumptionKwh:$k, model:"mock-v0"}')
  aws events put-events --region "__REGION__" --entries "$(jq -cn --arg d "$DETAIL" \
    '[{Source:"__SOURCE__", DetailType:"__DETAIL_TYPE__", EventBusName:"__BUS__", Detail:$d}]')"
done
EOF
chmod +x /opt/forecast/run.sh
cat > /etc/systemd/system/forecast.service <<'EOF'
[Unit]
Description=Solar forecast publisher
[Service]
Type=oneshot
ExecStart=/opt/forecast/run.sh
EOF
cat > /etc/systemd/system/forecast.timer <<'EOF'
[Timer]
OnCalendar=*:0/15
Persistent=true
[Install]
WantedBy=timers.target
EOF
systemctl daemon-reload
systemctl enable --now forecast.timer`

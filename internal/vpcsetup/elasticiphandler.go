package vpcsetup

import (
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/awscost"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
)

//
//
// @Parameters
//
//
// @Returns
//
//
func SetupElasticIpHandler(ec2Client *ec2utils.Ec2Manger,
                           stateConfig *AwsEnv,
                           appConfig *conf.AppConfig,
                           yamlUpdates map[string]string,
                           outStruct *VpcBootstrapOutput,
                           location string, costErr *error,
                           costMan *awscost.AwsCostManager) (
                           string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-elastic-ip",
    }

    // Check to see if Elastic IP exists, otherwise create one
    eipId, err := ec2Client.ElasticIpProvision(1 * time.Minute,
                                               stateConfig.AwsEnv.EipId,
                                               tags)
    if err != nil {
        return "", err
    }

    // If a Elastic IP was created, add ID to yaml updates map
    if eipId != "" {
        yamlUpdates["aws_env.eip_id"] = eipId
    // Otherwise use the one from YAML since it was found
    } else {
        eipId = stateConfig.AwsEnv.EipId
    }

    outStruct.EipId = eipId

    filterMap := map[string]string{
        "location": location,
    }

    // Add the elastic IP to cost manager
    _ = costMan.AddCostResourceToManagerHandler("elastic_ip", filterMap, costErr)
    return eipId, nil
}

package aws

import (
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/ssoadmin"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceAwsSsoInstance() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceAwsSsoInstanceRead,

		Schema: map[string]*schema.Schema{
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"identity_store_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceAwsSsoInstanceRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ssoadminconn

	log.Printf("[DEBUG] Reading AWS SSO Instances")
	instances := []*ssoadmin.InstanceMetadata{}
	err := conn.ListInstancesPages(&ssoadmin.ListInstancesInput{}, func(page *ssoadmin.ListInstancesOutput, lastPage bool) bool {
		if page != nil && len(page.Instances) != 0 {
			instances = append(instances, page.Instances...)
		}
		return !lastPage
	})
	if err != nil {
		return fmt.Errorf("error getting AWS SSO Instances: %w", err)
	}

	if len(instances) == 0 {
		return fmt.Errorf("error getting AWS SSO Instances: no instance found")
	}

	if len(instances) > 1 {
		return fmt.Errorf("Found multiple AWS SSO Instances. Not sure which one to use. %s", instances)
	}

	instance := instances[0]
	log.Printf("[DEBUG] Received AWS SSO Instance: %s", aws.StringValue(instance.InstanceArn))

	d.SetId(aws.StringValue(instance.InstanceArn))
	d.Set("arn", instance.InstanceArn)
	d.Set("identity_store_id", instance.IdentityStoreId)

	return nil
}

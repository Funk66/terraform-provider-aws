// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package codestarnotifications

import (
	"context"
	"log"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codestarnotifications"
	"github.com/aws/aws-sdk-go-v2/service/codestarnotifications/types"
	"github.com/hashicorp/aws-sdk-go-base/v2/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/enum"
	"github.com/hashicorp/terraform-provider-aws/internal/errs"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	"github.com/hashicorp/terraform-provider-aws/internal/flex"
	tfslices "github.com/hashicorp/terraform-provider-aws/internal/slices"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// @SDKResource("aws_codestarnotifications_notification_rule", name="Notification Rule")
// @Tags(identifierAttribute="id")
func resourceNotificationRule() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceNotificationRuleCreate,
		ReadWithoutTimeout:   resourceNotificationRuleRead,
		UpdateWithoutTimeout: resourceNotificationRuleUpdate,
		DeleteWithoutTimeout: resourceNotificationRuleDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"detail_type": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateDiagFunc: enum.Validate[types.DetailType](),
			},
			"event_type_ids": {
				Type:     schema.TypeSet,
				Required: true,
				MinItems: 1,
				MaxItems: 200,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.All(
					validation.StringLenBetween(1, 64),
					validation.StringMatch(regexp.MustCompile(`^[A-Za-z0-9\-_ ]+$`), "must be one or more alphanumeric, hyphen, underscore or space characters"),
				),
			},
			"resource": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: verify.ValidARN,
			},
			"status": {
				Type:             schema.TypeString,
				Optional:         true,
				Default:          types.NotificationRuleStatusEnabled,
				ValidateDiagFunc: enum.Validate[types.NotificationRuleStatus](),
			},
			names.AttrTags:    tftags.TagsSchema(),
			names.AttrTagsAll: tftags.TagsSchemaComputed(),
			"target": {
				Type:     schema.TypeSet,
				Optional: true,
				MaxItems: 10,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"address": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: verify.ValidARN,
						},
						"status": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"type": {
							Type:     schema.TypeString,
							Default:  "SNS",
							Optional: true,
						},
					},
				},
			},
		},

		CustomizeDiff: verify.SetTagsDiff,
	}
}

func resourceNotificationRuleCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).CodeStarNotificationsClient(ctx)

	name := d.Get("name").(string)
	input := &codestarnotifications.CreateNotificationRuleInput{
		DetailType:   types.DetailType(d.Get("detail_type").(string)),
		EventTypeIds: flex.ExpandStringValueSet(d.Get("event_type_ids").(*schema.Set)),
		Name:         aws.String(name),
		Resource:     aws.String(d.Get("resource").(string)),
		Status:       types.NotificationRuleStatus(d.Get("status").(string)),
		Tags:         getTagsIn(ctx),
		Targets:      expandNotificationRuleTargets(d.Get("target").(*schema.Set).List()),
	}

	output, err := conn.CreateNotificationRule(ctx, input)

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "creating CodeStar Notification Rule (%s): %s", name, err)
	}

	d.SetId(aws.ToString(output.Arn))

	return append(diags, resourceNotificationRuleRead(ctx, d, meta)...)
}

func resourceNotificationRuleRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).CodeStarNotificationsClient(ctx)

	rule, err := findNotificationRuleByARN(ctx, conn, d.Id())

	if !d.IsNewResource() && tfresource.NotFound(err) {
		log.Printf("[WARN] CodeStar Notification Rule (%s) not found, removing from state", d.Id())
		d.SetId("")
		return diags
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "reading CodeStar Notification Rule (%s): %s", d.Id(), err)
	}

	d.Set("arn", rule.Arn)
	d.Set("detail_type", rule.DetailType)
	eventTypeIDs := tfslices.ApplyToAll(rule.EventTypes, func(v types.EventTypeSummary) string {
		return aws.ToString(v.EventTypeId)
	})
	d.Set("event_type_ids", eventTypeIDs)
	d.Set("name", rule.Name)
	d.Set("resource", rule.Resource)
	d.Set("status", rule.Status)

	targets := make([]map[string]interface{}, 0, len(rule.Targets))
	for _, t := range rule.Targets {
		targets = append(targets, map[string]interface{}{
			"address": aws.ToString(t.TargetAddress),
			"type":    aws.ToString(t.TargetType),
			"status":  t.TargetStatus,
		})
	}
	if err = d.Set("target", targets); err != nil {
		return sdkdiag.AppendErrorf(diags, "setting target: %s", err)
	}

	setTagsOut(ctx, rule.Tags)

	return diags
}

func resourceNotificationRuleUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).CodeStarNotificationsClient(ctx)

	input := &codestarnotifications.UpdateNotificationRuleInput{
		Arn:          aws.String(d.Id()),
		DetailType:   types.DetailType(d.Get("detail_type").(string)),
		EventTypeIds: flex.ExpandStringValueSet(d.Get("event_type_ids").(*schema.Set)),
		Name:         aws.String(d.Get("name").(string)),
		Status:       types.NotificationRuleStatus(d.Get("status").(string)),
		Targets:      expandNotificationRuleTargets(d.Get("target").(*schema.Set).List()),
	}

	if _, err := conn.UpdateNotificationRule(ctx, input); err != nil {
		return sdkdiag.AppendErrorf(diags, "updating CodeStar Notification Rule (%s): %s", d.Id(), err)
	}

	if d.HasChange("target") {
		o, n := d.GetChange("target")
		if err := cleanupNotificationRuleTargets(ctx, conn, o.(*schema.Set), n.(*schema.Set)); err != nil {
			return sdkdiag.AppendErrorf(diags, "deleting CodeStar Notification Rule (%s) targets: %s", d.Id(), err)
		}
	}

	return append(diags, resourceNotificationRuleRead(ctx, d, meta)...)
}

func resourceNotificationRuleDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).CodeStarNotificationsClient(ctx)

	log.Printf("[DEBUG] Deleting CodeStar Notification Rule: %s", d.Id())
	_, err := conn.DeleteNotificationRule(ctx, &codestarnotifications.DeleteNotificationRuleInput{
		Arn: aws.String(d.Id()),
	})

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "deleting CodeStar Notification Rule (%s): %s", d.Id(), err)
	}

	if err = cleanupNotificationRuleTargets(ctx, conn, d.Get("target").(*schema.Set), nil); err != nil {
		return sdkdiag.AppendErrorf(diags, "deleting CodeStar Notification Rule (%s) targets: %s", d.Id(), err)
	}

	return diags
}

func findNotificationRuleByARN(ctx context.Context, conn *codestarnotifications.Client, arn string) (*codestarnotifications.DescribeNotificationRuleOutput, error) {
	input := &codestarnotifications.DescribeNotificationRuleInput{
		Arn: aws.String(arn),
	}

	output, err := conn.DescribeNotificationRule(ctx, input)

	if errs.IsA[*types.ResourceNotFoundException](err) {
		return nil, &retry.NotFoundError{
			LastError:   err,
			LastRequest: input,
		}
	}

	if err != nil {
		return nil, err
	}

	if output == nil {
		return nil, tfresource.NewEmptyResultError(input)
	}

	return output, nil
}

// cleanupNotificationRuleTargets tries to remove unused notification targets. AWS API does not
// provide expicit way for creating targets, they are created on first subscription. Here we are trying to remove all
// unused targets which were unsubscribed from this notification rule.
func cleanupNotificationRuleTargets(ctx context.Context, conn *codestarnotifications.Client, oldVal, newVal *schema.Set) error {
	const (
		notificationRuleErrorSubscribed = "The target cannot be deleted because it is subscribed to one or more notification rules."
		targetSubscriptionTimeout       = 30 * time.Second
	)
	removedTargets := oldVal
	if newVal != nil {
		removedTargets = oldVal.Difference(newVal)
	}

	for _, targetRaw := range removedTargets.List() {
		target, ok := targetRaw.(map[string]interface{})

		if !ok {
			continue
		}

		input := &codestarnotifications.DeleteTargetInput{
			ForceUnsubscribeAll: false,
			TargetAddress:       aws.String(target["address"].(string)),
		}

		_, err := tfresource.RetryWhenAWSErrMessageContainsV2(ctx, targetSubscriptionTimeout, func() (interface{}, error) {
			return conn.DeleteTarget(ctx, input)
		}, "ValidationException", notificationRuleErrorSubscribed)

		// Treat target deletion as best effort.
		if tfawserr.ErrMessageContains(err, "ValidationException", notificationRuleErrorSubscribed) {
			continue
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func expandNotificationRuleTargets(targetsData []interface{}) []types.Target {
	targets := make([]types.Target, 0, len(targetsData))
	for _, t := range targetsData {
		target := t.(map[string]interface{})
		targets = append(targets, types.Target{
			TargetAddress: aws.String(target["address"].(string)),
			TargetType:    aws.String(target["type"].(string)),
		})
	}
	return targets
}

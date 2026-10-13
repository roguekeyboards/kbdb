package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/rogueserenity/kbdb/internal/moderation"
	"github.com/rogueserenity/kbdb/internal/repository/dynamo"
)

// stack is one environment's resources, looked up by their logical IDs in
// template.yaml.
type stack struct {
	cfg      aws.Config
	bucket   string
	logGroup string
	images   *moderation.Images
}

func resolveStack(ctx context.Context, cfg aws.Config, name string) (*stack, error) {
	cfn := cloudformation.NewFromConfig(cfg)
	ids := map[string]string{
		"ImagesBucket":             "",
		"ModerateFunctionLogGroup": "",
		"SwitchTable":              "",
		"KeyboardTable":            "",
		"KeycapSetTable":           "",
		"BuildTable":               "",
		"ProfileTable":             "",
		"ProfileUsernameTable":     "",
	}
	for logical := range ids {
		out, err := cfn.DescribeStackResource(ctx, &cloudformation.DescribeStackResourceInput{
			StackName:         &name,
			LogicalResourceId: aws.String(logical),
		})
		if err != nil {
			return nil, fmt.Errorf("looking up %s in stack %s: %w", logical, name, err)
		}
		ids[logical] = aws.ToString(out.StackResourceDetail.PhysicalResourceId)
	}

	db := dynamodb.NewFromConfig(cfg)
	return &stack{
		cfg:      cfg,
		bucket:   ids["ImagesBucket"],
		logGroup: ids["ModerateFunctionLogGroup"],
		images: &moderation.Images{
			Keyboards:  dynamo.NewKeyboardRepository(db, ids["KeyboardTable"]),
			Builds:     dynamo.NewBuildRepository(db, ids["BuildTable"]),
			Switches:   dynamo.NewSwitchRepository(db, ids["SwitchTable"]),
			KeycapSets: dynamo.NewKeycapSetRepository(db, ids["KeycapSetTable"]),
			Profiles:   dynamo.NewProfileRepository(db, ids["ProfileTable"], ids["ProfileUsernameTable"]),
		},
	}, nil
}

func (st *stack) overrides() *moderation.Overrides {
	return moderation.NewOverrides(s3.NewFromConfig(st.cfg), st.images, st.bucket)
}

package main

import (
	"context"
	"log"

	"github.com/alecthomas/kong"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

func main() {
	var cli CLI
	kctx := kong.Parse(&cli,
		kong.Name("mykeebs-moderation"),
		kong.Description("Review and override image moderation decisions in one kbdb stack."),
		kong.UsageOnError(),
	)

	ctx := context.Background()
	opts := []func(*awsconfig.LoadOptions) error{}
	if cli.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cli.Region))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		log.Fatalf("loading AWS config: %v", err)
	}
	st, err := resolveStack(ctx, awsCfg, cli.Stack)
	if err != nil {
		log.Fatal(err)
	}

	kctx.BindTo(ctx, (*context.Context)(nil))
	kctx.FatalIfErrorf(kctx.Run(st))
}

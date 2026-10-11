// Command api is the kbdb API server. It is a plain net/http server with no
// Lambda-specific code — in production it runs behind the aws-lambda-web-adapter
// extension, which translates API Gateway events into real HTTP requests
// against this process.
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/rogueserenity/kbdb/internal/auth"
	"github.com/rogueserenity/kbdb/internal/repository/dynamo"
	imagestore "github.com/rogueserenity/kbdb/internal/repository/s3"
	"github.com/rogueserenity/kbdb/internal/router"
	"github.com/rogueserenity/kbdb/internal/uploadlimit"
)

// Version is set at build time via -ldflags "-X main.Version=...", derived
// from `git describe --tags --always --dirty` (see functions/api/Dockerfile).
// Defaults to "dev" when built without that flag (e.g. `go build` directly).
var Version = "dev"

// port must match AWS_LWA_PORT in template.yaml; not 8080 since sam local
// start-api's Runtime Interface Emulator already binds that port in-container
// (github.com/aws/aws-lambda-web-adapter#125).
const port = "8000"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	var cfg Config
	kong.Parse(&cfg)

	ctx := context.Background()
	verifier, err := auth.NewVerifier(ctx, cfg.OIDCIssuerURL, cfg.OIDCAudience)
	if err != nil {
		log.Fatalf("initializing token verifier: %v", err)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("loading AWS config: %v", err)
	}

	dynamoClient := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if cfg.DynamoDBEndpointURL != "" {
			o.BaseEndpoint = &cfg.DynamoDBEndpointURL
		}
	})
	switchRepo := dynamo.NewSwitchRepository(dynamoClient, cfg.SwitchTableName)
	keyboardRepo := dynamo.NewKeyboardRepository(dynamoClient, cfg.KeyboardTableName)
	keycapSetRepo := dynamo.NewKeycapSetRepository(dynamoClient, cfg.KeycapSetTableName)
	buildRepo := dynamo.NewBuildRepository(dynamoClient, cfg.BuildTableName)
	profileRepo := dynamo.NewProfileRepository(dynamoClient, cfg.ProfileTableName, cfg.ProfileUsernameTableName)
	uploads := uploadlimit.New(dynamo.NewUploadCounter(dynamoClient, cfg.UploadCounterTableName), cfg.DailyUploadLimit)

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3EndpointURL != "" {
			o.BaseEndpoint = &cfg.S3EndpointURL
			// LocalStack needs path-style addressing (bucket.s3.amazonaws.com
			// virtual-hosted-style doesn't resolve against a non-AWS
			// endpoint); real S3 doesn't.
			o.UsePathStyle = true
		}
	})
	// All entities' images currently live in the same bucket.
	presignClient := s3.NewPresignClient(s3Client)

	// Presigned GETs are signed by an assumed role rather than the ambient
	// Lambda credentials - see Config.PresignRoleARN. CredentialsCache keeps
	// this to one AssumeRole per session, not one per signature.
	presignCreds := awsCfg.Credentials
	if cfg.PresignRoleARN != "" {
		presignCreds = aws.NewCredentialsCache(
			stscreds.NewAssumeRoleProvider(sts.NewFromConfig(awsCfg), cfg.PresignRoleARN,
				func(o *stscreds.AssumeRoleOptions) {
					o.RoleSessionName = "kbdb-presign"
					o.Duration = cfg.PresignSessionDuration
				}),
		)
	}
	keycapKitImageStore := imagestore.NewKeycapKitImageStore(s3Client, presignClient, cfg.ImagesBucketName, presignCreds)
	buildImageStore := imagestore.NewBuildImageStore(s3Client, presignClient, cfg.ImagesBucketName, presignCreds)
	keyboardImageStore := imagestore.NewKeyboardImageStore(s3Client, presignClient, cfg.ImagesBucketName, presignCreds)
	switchImageStore := imagestore.NewSwitchImageStore(s3Client, presignClient, cfg.ImagesBucketName, presignCreds)
	profileImageStore := imagestore.NewProfileImageStore(s3Client, presignClient, cfg.ImagesBucketName, presignCreds)

	handler := router.New(verifier, switchRepo, switchImageStore, keyboardRepo, keyboardImageStore, keycapSetRepo, keycapKitImageStore, buildRepo, buildImageStore, profileRepo, profileImageStore, uploads, cfg.OIDCIssuerURL, cfg.IDPConsentPublicToken, Version, strings.Split(cfg.LogoutReturnOrigins, ","))

	// ReadHeaderTimeout bounds a slow/malicious client independently of
	// Lambda's own per-invocation timeout.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("starting server", "port", port, "version", Version)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

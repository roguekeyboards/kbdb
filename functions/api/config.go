package main

import "time"

// Config is populated from environment variables via Kong. There are no CLI
// flags for this service — it's a Lambda entrypoint, not a CLI tool — but
// Kong's struct-tag env binding, defaults, and required-field validation are
// useful regardless of whether flag parsing is ever exercised.
type Config struct {
	OIDCIssuerURL string `env:"OIDC_ISSUER_URL" required:""`
	OIDCAudience  string `env:"OIDC_AUDIENCE" required:""`
	// The IdP's browser-SDK public token, rendered into the GET /authorize
	// consent page - see internal/consent.
	IDPConsentPublicToken string `env:"IDP_CONSENT_PUBLIC_TOKEN" required:""`
	// Comma-separated origins GET /logout may redirect back to - see
	// internal/consent.
	LogoutReturnOrigins string `env:"LOGOUT_RETURN_ORIGINS" required:""`
	ImagesBucketName    string `env:"IMAGES_BUCKET_NAME" required:""`
	// Role assumed solely to sign presigned GET URLs. Lambda's own
	// credentials carry no expiry the SDK can see, so a URL signed with them
	// can't be bounded to their real lifetime; an assumed role returns an
	// explicit Expiration. Optional: empty falls back to the ambient
	// credentials (sam local, and any deploy without the role).
	PresignRoleARN string `env:"PRESIGN_ROLE_ARN"`

	// Session length requested when assuming PresignRoleARN, and so the
	// ceiling on a presigned GET URL's life. 1h is the hard maximum here:
	// Lambda's execution role is itself assumed, so assuming another role
	// from it is role chaining, which STS caps at 1h whatever the target
	// role's MaxSessionDuration says.
	PresignSessionDuration   time.Duration `env:"PRESIGN_SESSION_DURATION" default:"1h"`
	SwitchTableName          string        `env:"SWITCH_TABLE_NAME" required:""`
	KeyboardTableName        string        `env:"KEYBOARD_TABLE_NAME" required:""`
	KeycapSetTableName       string        `env:"KEYCAP_SET_TABLE_NAME" required:""`
	BuildTableName           string        `env:"BUILD_TABLE_NAME" required:""`
	ProfileTableName         string        `env:"PROFILE_TABLE_NAME" required:""`
	ProfileUsernameTableName string        `env:"PROFILE_USERNAME_TABLE_NAME" required:""`
	UploadCounterTableName   string        `env:"UPLOAD_COUNTER_TABLE_NAME" required:""`
	// Upload URLs issued per UTC day across every user - see
	// internal/uploadlimit.
	DailyUploadLimit int64 `env:"DAILY_UPLOAD_LIMIT" required:""`

	// Empty in real deployments; set locally to point at LocalStack.
	DynamoDBEndpointURL string `env:"DYNAMODB_ENDPOINT_URL"`

	// Empty in real deployments; set locally to point at LocalStack.
	S3EndpointURL string `env:"S3_ENDPOINT_URL"`
}

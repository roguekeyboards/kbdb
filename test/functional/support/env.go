package support

import "os"

// BaseURL returns the API's base URL under test, e.g. "http://127.0.0.1:3000"
// for a local sam local start-api instance or the real deployed kbdb-dev API
// Gateway URL. Configurable via KBDB_API_BASE_URL.
func BaseURL() string {
	if v := os.Getenv("KBDB_API_BASE_URL"); v != "" {
		return v
	}
	return "http://127.0.0.1:3000"
}

// OidcIssuer is the iss claim every minted test token must carry, matching the
// deployed authorizer's configured issuer.
func OidcIssuer() string {
	return os.Getenv("KBDB_OIDC_ISSUER")
}

// OidcAudience is the aud claim every minted test token must carry.
func OidcAudience() string {
	return os.Getenv("KBDB_OIDC_AUDIENCE")
}

// OidcSigningKeyPath is the PEM oidc-testkit-gen wrote pre-deploy; the suite
// loads it once and signs every token against it.
func OidcSigningKeyPath() string {
	return os.Getenv("KBDB_OIDC_SIGNING_KEY_PATH")
}

// SwitchTableName returns the DynamoDB table name backing
// GET /users/{userId}/switches, so specs can seed fixture rows directly.
func SwitchTableName() string {
	return os.Getenv("KBDB_SWITCH_TABLE_NAME")
}

// KeyboardTableName returns the DynamoDB table name backing
// GET /users/{userId}/keyboards, so specs can seed fixture rows directly.
func KeyboardTableName() string {
	return os.Getenv("KBDB_KEYBOARD_TABLE_NAME")
}

// KeycapSetTableName returns the DynamoDB table name backing
// GET /users/{userId}/keycap-sets, so specs can seed fixture rows directly.
func KeycapSetTableName() string {
	return os.Getenv("KBDB_KEYCAP_SET_TABLE_NAME")
}

// BuildTableName returns the DynamoDB table name backing
// POST /users/{userId}/builds, so specs can clean up fixture rows directly.
func BuildTableName() string {
	return os.Getenv("KBDB_BUILD_TABLE_NAME")
}

// ProfileTableName returns the DynamoDB table name backing
// GET /v1/profile/{identifier}, so specs can seed fixture rows directly.
func ProfileTableName() string {
	return os.Getenv("KBDB_PROFILE_TABLE_NAME")
}

// ProfileUsernameTableName returns the DynamoDB table name holding
// { username -> user_id } claim items, so specs can seed the claim needed
// to resolve a profile by username.
func ProfileUsernameTableName() string {
	return os.Getenv("KBDB_PROFILE_USERNAME_TABLE_NAME")
}

// UploadCounterTableName returns the DynamoDB table name holding the
// per-day image upload counts, so specs can put a day at its limit.
func UploadCounterTableName() string {
	return os.Getenv("KBDB_UPLOAD_COUNTER_TABLE_NAME")
}

// ImagesBucketName returns the bucket uploaded images are stored in.
func ImagesBucketName() string {
	return os.Getenv("KBDB_IMAGES_BUCKET_NAME")
}

// AIMockConfigPath returns the host path of the file floci reads canned
// Rekognition responses from, or "" against real AWS, which has no such
// file.
func AIMockConfigPath() string {
	return os.Getenv("KBDB_AI_MOCK_CONFIG")
}

// S3EndpointURL returns the S3 endpoint specs should talk to. Empty uses
// default AWS endpoint resolution.
func S3EndpointURL() string {
	return os.Getenv("KBDB_S3_ENDPOINT_URL")
}

// DynamoDBEndpointURL returns the DynamoDB endpoint specs should talk to.
// Empty uses default AWS endpoint resolution.
func DynamoDBEndpointURL() string {
	return os.Getenv("KBDB_DYNAMODB_ENDPOINT_URL")
}

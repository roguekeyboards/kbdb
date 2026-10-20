package middleware

import (
	"encoding/json"
	"net/http"
)

// authorizerRequestContext is the minimal shape this package needs from the
// X-Amzn-Request-Context header aws-lambda-web-adapter forwards on every
// request — the JSON-serialized form of the original Lambda proxy
// integration event's requestContext object (confirmed via direct read of
// awslabs/aws-lambda-web-adapter's source, Adapter::fetch_response;
// undocumented in the adapter's own README). Only the fields this package
// actually reads are declared — see AWS's HTTP API JWT authorizer docs for
// the full shape:
// https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-jwt-authorizer.html
type authorizerRequestContext struct {
	Authorizer struct {
		JWT struct {
			Claims map[string]string `json:"claims"`
		} `json:"jwt"`
	} `json:"authorizer"`
}

// authorizerSubject extracts the "sub" claim API Gateway's native JWT
// authorizer already verified, from the X-Amzn-Request-Context header
// aws-lambda-web-adapter forwards. Returns ("", false) if the header is
// absent (e.g. a route with no gateway authorizer, or local dev outside
// sam local start-api/API Gateway entirely) or doesn't contain a sub claim -
// both are valid, non-error states the caller must handle, not something
// this function itself decides is wrong.
func authorizerSubject(r *http.Request) (string, bool) {
	raw := r.Header.Get("X-Amzn-Request-Context")
	if raw == "" {
		return "", false
	}

	var rc authorizerRequestContext
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		return "", false
	}

	sub, ok := rc.Authorizer.JWT.Claims["sub"]
	if !ok || sub == "" {
		return "", false
	}
	return sub, true
}

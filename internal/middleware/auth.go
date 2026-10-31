package middleware

import (
	"net/http"

	"github.com/roguekeyboards/mykeebs-api/internal/auth"
	ctxpkg "github.com/roguekeyboards/mykeebs-api/internal/ctx"
	logpkg "github.com/roguekeyboards/mykeebs-api/internal/log"
	"github.com/roguekeyboards/mykeebs-api/internal/problem"
)

// OptionalAuth verifies the bearer token if one is present, for routes that
// allow anonymous callers (e.g. reads on items whose visibility may permit
// public or authenticated-but-not-owner access — see
// [github.com/roguekeyboards/mykeebs-api/internal/authz]). A missing token is not
// an error and the request proceeds with no user ID on its context; a
// present-but-invalid token is still rejected with 401, since silently
// treating it as anonymous would hide a real client error.
//
// Required-auth routes no longer have an equivalent in-process middleware:
// they rely solely on API Gateway's native JWT authorizer (see
// internal/gatewayroutes), which verifies the identical token before the
// request ever reaches this process. An earlier version re-verified
// in-process too, as deliberate defense-in-depth; that was re-examined and
// dropped as redundant — the native authorizer is an AWS-verified check of
// the same token, on the same class of mechanism.
func OptionalAuth(verifier *auth.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken, ok := auth.BearerToken(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			authedReq, err := authenticate(r, verifier, rawToken)
			if err != nil {
				problem.Unauthorized(w, "invalid token")
				return
			}

			next.ServeHTTP(w, authedReq)
		})
	}
}

// RequireAuth verifies the bearer token in-process, unlike
// RequireAuthorizerIdentity below which trusts API Gateway's native
// authorizer already did so. Only /mcp uses this: the MCP spec requires a
// 401 there to carry a WWW-Authenticate header naming the RFC 9728 metadata
// URL, and neither API Gateway's native authorizer nor a custom Lambda
// authorizer can add that header to their rejection - both reject before
// Lambda ever runs, confirmed against a live deploy and AWS's own
// authorizer response-format docs. internal/gatewayroutes gives /mcp no
// gateway authorizer accordingly.
//
// metadataPath is built per-request from the request's scheme/Host, same
// as internal/mcp/server.go's metadataHandler does for its "resource"
// field.
func RequireAuth(verifier *auth.Verifier, metadataPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken, ok := auth.BearerToken(r)
			if !ok {
				unauthorizedWithMetadata(w, r, metadataPath, "missing bearer token")
				return
			}

			authedReq, err := authenticate(r, verifier, rawToken)
			if err != nil {
				unauthorizedWithMetadata(w, r, metadataPath, "invalid token")
				return
			}

			next.ServeHTTP(w, authedReq)
		})
	}
}

// unauthorizedWithMetadata writes a 401 carrying a WWW-Authenticate header
// per RFC 9728 section 5.1, so an MCP client with no token yet can discover
// the Protected Resource Metadata document from the rejection itself
// (see RequireAuth).
func unauthorizedWithMetadata(w http.ResponseWriter, r *http.Request, metadataPath, detail string) {
	metadataURL := schemeOf(r) + "://" + r.Host + metadataPath
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+metadataURL+`"`)
	problem.Unauthorized(w, detail)
}

// schemeOf mirrors internal/mcp/server.go's identical helper - both need
// the original client-facing scheme, which the aws-lambda-web-adapter proxy
// hop would otherwise hide behind a plain http:// Host.
func schemeOf(r *http.Request) string {
	if fp := r.Header.Get("X-Forwarded-Proto"); fp != "" {
		return fp
	}
	return "https"
}

// RequireAuthorizerIdentity reads the caller's identity from the
// X-Amzn-Request-Context header API Gateway's native JWT authorizer
// populates via aws-lambda-web-adapter (see authorizer_context.go) and
// writes it into ctxpkg/logpkg, the same target OptionalAuth's
// authenticate() writes to. Unlike OptionalAuth, a missing/unparseable
// identity here IS an error (500, not 401) - reaching this middleware at
// all means the route's Auth block has no NONE override, so API Gateway
// already rejected any request without a valid token before it got here;
// if this middleware still finds no identity, that's a misconfiguration
// (e.g. this route wired to the wrong authorizer, or running outside API
// Gateway/aws-lambda-web-adapter entirely), not a legitimate
// unauthenticated request to reject with 401.
func RequireAuthorizerIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub, ok := authorizerSubject(r)
		if !ok {
			logpkg.FromContext(r.Context()).Error("required-auth route reached with no verified identity from API Gateway authorizer")
			problem.Internal(w, "server misconfiguration")
			return
		}

		ctx := ctxpkg.WithUserID(r.Context(), sub)
		l := logpkg.WithUserID(logpkg.FromContext(ctx), sub)
		ctx = logpkg.WithLogger(ctx, l)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authenticate verifies rawToken and returns r with its context updated to
// carry the verified caller's user ID and request-scoped logger.
func authenticate(r *http.Request, verifier *auth.Verifier, rawToken string) (*http.Request, error) {
	claims, err := verifier.VerifyToken(r.Context(), rawToken)
	if err != nil {
		// Warn, not Error: an individual invalid/expired token from one
		// client is expected traffic, not a bug - still worth a trace to
		// spot a misconfigured client or repeated probing.
		logpkg.FromContext(r.Context()).Warn("token verification failed", logpkg.Error, err)
		return nil, err
	}

	ctx := ctxpkg.WithUserID(r.Context(), claims.Subject)
	l := logpkg.WithUserID(logpkg.FromContext(ctx), claims.Subject)
	ctx = logpkg.WithLogger(ctx, l)

	return r.WithContext(ctx), nil
}

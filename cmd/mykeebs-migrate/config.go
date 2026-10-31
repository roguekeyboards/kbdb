package main

// CLI is the mykeebs-migrate command tree, parsed by Kong. Every option binds to
// an env var (MYKEEBS_-prefixed, matching the functional-test convention in
// test/functional/support/env.go) as well as a flag, so it can be driven
// either way.
type CLI struct {
	Login   LoginCmd   `cmd:"" help:"Obtain an access token via a browser OAuth flow (Discord) and cache it."`
	Dump    DumpCmd    `cmd:"" help:"Download your profile and every entity you own, plus their images, into a local directory."`
	Restore RestoreCmd `cmd:"" help:"Recreate a dump's entities and images in the target environment."`
	Verify  VerifyCmd  `cmd:"" help:"Check a restored environment against the dump (fields and image hashes)."`
}

// LoginCmd runs the OAuth 2.0 authorization-code + PKCE flow against the IdP
// the issuer advertises, capturing the resulting access token to the on-disk
// cache used by the other subcommands.
type LoginCmd struct {
	Issuer   string `env:"MYKEEBS_OIDC_ISSUER_BASE_URL" required:"" help:"OIDC issuer base URL, e.g. https://auth.jay.mykeebs.dev."`
	ClientID string `env:"MYKEEBS_OIDC_CLIENT_ID" help:"OAuth client ID. If unset and the issuer supports RFC 7591, one is registered dynamically and cached."`
	Port     int    `default:"8765" help:"Localhost port for the redirect listener. Must match a registered redirect URI; 8765 is the one mykeebs-api provisions."`
}

// DumpCmd walks the profile and every collection the token's subject owns and
// writes a directory-per-item tree under Out.
type DumpCmd struct {
	BaseURL     string `env:"MYKEEBS_API_BASE_URL" required:"" help:"API base URL, e.g. https://jay.api.mykeebs.dev."`
	Token       string `env:"MYKEEBS_AUTH_TOKEN" help:"Bearer token. Overrides the login cache. If unset, the login cache for --issuer is used. Ignored with --impersonate."`
	Issuer      string `env:"MYKEEBS_OIDC_ISSUER_BASE_URL" help:"Issuer whose login-cache entry to use when --token is unset. With --impersonate, the issuer the impersonation token is exchanged through."`
	Impersonate bool   `help:"Dump the account a Stytch dashboard 'Impersonate user' opens, instead of your own. Needs --issuer and --public-token."`
	PublicToken string `env:"MYKEEBS_IDP_PUBLIC_TOKEN" help:"The Stytch project's public token (IdpConsentPublicToken in samconfig.toml), for --impersonate."`
	Port        int    `default:"8765" help:"Localhost port for the --impersonate redirect listener. Must match the Stytch project's default Login redirect URL."`
	Out         string `required:"" type:"path" help:"Output directory for the dump."`
}

// RestoreCmd recreates the entities and images in a dump directory in the
// environment BaseURL points at, as the restore token's subject.
type RestoreCmd struct {
	BaseURL string `env:"MYKEEBS_API_BASE_URL" required:"" help:"Target API base URL."`
	Token   string `env:"MYKEEBS_AUTH_TOKEN" help:"Bearer token for the target. Overrides the login cache."`
	Issuer  string `env:"MYKEEBS_OIDC_ISSUER_BASE_URL" help:"Issuer whose login-cache entry to use when --token is unset."`
	In      string `required:"" type:"path" help:"Dump directory produced by 'mykeebs-migrate dump'."`
}

// VerifyCmd compares a dump directory against a restored environment using the
// id-map.json the restore wrote.
type VerifyCmd struct {
	BaseURL string `env:"MYKEEBS_API_BASE_URL" required:"" help:"Target API base URL (the one restored into)."`
	Token   string `env:"MYKEEBS_AUTH_TOKEN" help:"Bearer token for the target. Overrides the login cache."`
	Issuer  string `env:"MYKEEBS_OIDC_ISSUER_BASE_URL" help:"Issuer whose login-cache entry to use when --token is unset."`
	In      string `required:"" type:"path" help:"Dump directory, containing the id-map.json a prior restore wrote."`
}

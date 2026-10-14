# Contributing

## Setup

Tool versions (Go, AWS CLI, SAM CLI, mockery, golangci-lint) are pinned via [mise](https://mise.jdx.dev/):

```sh
mise install
mise activate  # add to your shell profile so `go`, `sam`, `aws`, etc. resolve directly
```

`sam build` requires Docker running locally. On macOS with Docker Desktop, make sure `/Applications/Docker.app/Contents/Resources/bin` is on your `PATH` — otherwise `sam build`/`docker push` fail with a credential-store error.

## Everyday commands

```sh
mise run lint             # golangci-lint + actionlint + shellcheck
mise run test             # unit tests
mise run build            # sam build
mise run gen              # regenerate mocks, OpenAPI types, and go.mod/go.sum after changing an interface or api/openapi.yaml
mise run check-generated  # run gen and fail if it changed anything (what CI runs)
```

Single-test invocations use the underlying tools directly:

```sh
go test ./... -run TestVerifyTokenSuite -v   # a single suite
go test ./internal/auth/... -v               # a single package
sam validate --lint
```

## Running the app locally

```sh
mise run func-setup    # deploys template.yaml to floci + generates local OIDC signing material
# export the KBDB_* vars func-setup prints, then:
mise run func-test     # runs the functional test suite against it
mise run func-teardown # tears it all down
```

`func-setup` deploys kbdb's real `template.yaml` to [floci](https://github.com/floci-io/floci) (a local AWS emulator) via a genuine `sam deploy` — not `sam local start-api`, which never emulates API Gateway's native JWT authorizer, so it couldn't exercise any auth-required write route (see `internal/middleware.RequireAuthorizerIdentity`). It also runs [`oidc-testkit-gen`](https://github.com/rogueserenity/oidc-testkit) to produce the signing key + discovery doc + JWKS. All `KBDB_*` values land in `.env.floci`, which `func-test` sources automatically.

Most iteration just needs `mise run func-test` again — Go code and test changes are picked up automatically. Restart (`mise run func-teardown && mise run func-setup`) after editing `template.yaml` or `docker-compose.floci.yml`.

## Backing up / moving a user's data (`kbdb-migrate`)

`cmd/kbdb-migrate` is a standalone CLI that dumps every entity a user owns (keyboards, switches, keycap sets, builds) plus every S3 image to a local directory, and restores such a dump into another environment through the same public REST API. Use it to take a verifiable backup before a data-model change, or to move a user's data between environments. It uses **no AWS credentials** — image bytes move over presigned URLs — and never touches lookups (`scripts/sync-lookups.sh` owns those; the dump captures `lookups/lookups.json` for diffing only). It also skips the **profile**: the username is globally unique and identity-bound, so it can't be recreated under a different account — set profiles up per account by hand.

```sh
mise run migrate-build          # -> bin/kbdb-migrate

# 1. Get a token for the SOURCE environment (opens a browser: Discord or email OTP).
bin/kbdb-migrate login --issuer https://auth.jay.mykeebs.dev

# 2. Dump.
bin/kbdb-migrate dump --base-url https://api.jay.mykeebs.dev --out ./dump

# 3. Get a token for the TARGET (may be a different account/environment), then restore.
bin/kbdb-migrate login --issuer <target issuer>
bin/kbdb-migrate restore --base-url <target base url> --in ./dump

# 4. Check the restore against the dump.
bin/kbdb-migrate verify --base-url <target base url> --in ./dump
```

`login` runs a standard OAuth 2.0 authorization-code + PKCE flow and binds a **fixed** localhost port (`8765`) for the redirect, because IdP redirect URIs are exact-match. For a **dev** Stytch project this is already provisioned (redirect `http://localhost:8765/authorize.html`, SDK domain `http://localhost:8765`, and dynamic client registration is enabled so no client ID is needed). For a **prod** (Stytch Live) project, someone must first add that redirect URI and SDK domain in the Stytch dashboard and provision an OAuth client, then pass `--client-id` (or set `KBDB_OIDC_CLIENT_ID`). The token is cached under `~/.config/kbdb-migrate/` keyed by issuer host; `dump`/`restore`/`verify` also accept `--token` / `KBDB_AUTH_TOKEN` directly.

**Moving data between two accounts on the same issuer** (step 1 and step 3 are different people): the second `login` opens the browser with the first account's IdP session still live, so it lands on the consent screen already "Signed in" as the wrong account. Click **"Not you? Use a different account"** on that page to drop the session and sign in as the target account.

Restore always **creates new** items (new server-generated IDs), recording an old→new `id-map.json` in the dump directory; builds are restored last with their keyboard/switch/keycap-kit references remapped through that map, and a re-run resumes from wherever a failure stopped.

## Reviewing moderation decisions (`kbdb-moderation`)

There's no admin UI for image moderation. `cmd/kbdb-moderation` reviews and overrides decisions in one environment's stack, run through these tasks with an admin AWS profile (`AWS_PROFILE=kbdb-dev-admin`, etc.). `env` defaults to `dev-$(whoami)`, as for `deploy`.

```sh
mise run moderation-rejections [env] [--since 7d]   # rejections awaiting review (--all: every rejection and takedown, and how it was reviewed)
mise run moderation-view [env] <key>                # open the image in a browser, even a rejected one
mise run moderation-uphold [env] <key>              # the rejection stands; takes it off the list
mise run moderation-approve [env] <key>             # override a rejection; the image no longer expires
mise run moderation-reject [env] <key> [reason]     # take down an approved image (reason defaults to "manual")
```

`<key>` is the image's object key, as `moderation-rejections` prints it, or a link to the image: to take down an image reported on the site, copy its image address and pass that. A link's path is the key, so an expired link works too. Approve and reject are conditional writes: approve only acts on a rejected image, reject only on an approved one, and either fails if the item no longer holds that key. Running one again after it failed partway is safe. Approve refuses an image whose file has already expired.

A manual reject tags the file to expire like the moderation function's rejections, but the lifecycle rule counts from upload, so an image older than the expiry period is deleted at the rule's next nightly run: treat a takedown as final. Download links the API handed out before the takedown keep working until they expire, within an hour; a presigned URL can't be revoked.

`moderation-rejections` is the review queue: work through it with `view`, then `uphold` or `approve`. Each of `uphold`, `approve` and `reject` writes a review line to the moderation function's log group (stream `operator-reviews`), and a rejection with a later review drops off the list. `--all` lists every rejection and takedown in the window, with its latest review, which is also the record of what was decided and when. The logs are kept 60 days (so `--since` goes up to `60d`), longer than any rejected file, which expires after 30. If recording a review fails after the action succeeded, the action stands; run the same command again to record it.

## Deploying to AWS

Each environment is a section in `samconfig.toml` — stack name, artifact bucket, ECR repo, region and every template parameter — deployed with three tasks:

```sh
mise run setup [env]     # one-time: creates the env's ECR repo stack, then deploys
mise run deploy [env]    # deploy your current code to it
mise run teardown [env]  # delete the stack, the ECR repo stack and the repo itself
```

`env` defaults to `dev-$(whoami)`. Developers each get their own isolated stack, so nobody can break anyone else's testing. To add yours, copy the `[dev-jay.deploy.parameters]` section to `[dev-<your-name>.deploy.parameters]` and change its values: `stack_name` must be `kbdb-dev-<your-name>`, and the ECR repo in `image_repositories` must be `kbdb-api-<stack_name>` (the [scoped dev policy](#giving-a-developer-scoped-access-no-admin-needed) only covers those names). None of the values are secret; commit the section. Want a second stack? Add a second section, e.g. `dev-<your-name>-test`.

Put every template parameter in the section's `parameter_overrides`. Passing `--parameter-overrides` on the command line replaces that list wholesale rather than merging with it, so the scripts never do.

Before changing anything, each task checks that your active AWS credentials belong to the account in the section's `s3_bucket` (`kbdb-sam-artifacts-<account-id>`) and stops if not. `teardown` refuses `prod` outright.

You just need an active, authenticated AWS session — any profile works, there's no required profile name. This project's own maintainer setup happens to use a profile named `AWS_PROFILE=kbdb-dev-admin` (see [AWS accounts](#aws-accounts) below), but that's just this project's convention, not a requirement. **If you're forking this repo**, use any profile authenticated to your own AWS account, with either admin access or the [scoped dev policy](#giving-a-developer-scoped-access-no-admin-needed) attached.

### IdP OIDC config (`OidcIssuerBaseUrl`/`OidcAudience`/`IdpConsentPublicToken`)

kbdb is IdP-agnostic (see [Identity provider requirements](README.md#identity-provider-requirements)); this project is currently run against a Stytch **Test** project, and the parenthetical guidance below reflects that.

- `OidcIssuerBaseUrl` — your IdP project's OIDC issuer base URL (for a Stytch Test project: `https://test.stytch.com/v1/public/{project_id}`).
- `OidcAudience` — the `aud` claim on every access token, REST and MCP alike: typically your IdP project ID. Some IdPs (Stytch included) put the project ID in `aud` regardless of client type, so one value covers both flows — no separate MCP audience needed.
- `IdpConsentPublicToken` — your IdP's browser-SDK public token (for Stytch: dashboard → your project → API keys → Public token). Rendered client-side into the `GET /authorize` consent page to construct the IdP SDK client - safe to embed client-side by design, but still varies per stack.

Test is for dev/personal stacks only — never point a dev stack at a live/production IdP project.

### CORS (`CorsAllowOrigins`)

A comma-separated list of browser origins (each scheme + host + port) allowed to call your stack's `HttpApi` cross-origin, e.g. `http://localhost:5173,https://jay.mykeebs.dev` to cover both a local frontend dev server and its deployed counterpart. Without this, browser preflight (`OPTIONS`) requests 404 before the authorizer is ever reached — API Gateway auto-generates CORS `OPTIONS` routes and exempts them from `DefaultAuthorizer` only when `HttpApi.Properties.CorsConfiguration` is set.

### Logout return origins (`LogoutReturnOrigins`)

A comma-separated list of browser origins `GET /logout` is allowed to redirect back to via its `return_to` param (see `internal/consent`), same format as `CorsAllowOrigins` above. `/logout` revokes the IdP session on this stack's own origin (something `mykeebs-web`'s `signOut()` can't do itself, since the session lives on a different origin than `mykeebs-web`), then redirects to `return_to` — restricted to this allowlist so it isn't an open redirect.

### Custom domain (`api.<your-name>.mykeebs.dev`)

Optional. `template.yaml`'s `CustomDomainName`/`CustomDomainCertificateArn` parameters (both default to empty) map a stack to a stable domain instead of its default `execute-api.amazonaws.com` URL. `<your-name>.mykeebs.dev` itself is intentionally left free (e.g. for a future web UI hosted elsewhere, like Render) - `api.` is a real subdomain label in front of your name, not a wildcard-coverable suffix of it, so `*.mykeebs.dev` does **not** match `api.jay.mykeebs.dev` (wildcards only cover one label deep). Each developer needs their own certificate for their own `api.<name>.mykeebs.dev`, not one shared wildcard.

DNS for `mykeebs.dev` is hosted on Cloudflare, not Route53, so the ACM certificate can't self-validate the way it would with a Route53-hosted zone — it needs a one-time manual step instead. There's nothing to automate or renew afterward: once issued, a DNS-validated ACM certificate auto-renews forever as long as its validation CNAME record stays in place.

Each developer who wants this does it once for their own name:

1. **Request a certificate for your own subdomain**, region-matched to where you deploy (`api.<name>.mykeebs.dev` needs a *regional* API Gateway custom domain cert, unlike CloudFront/edge certs which require `us-east-1`):
   ```sh
   aws acm request-certificate --domain-name 'api.<your-name>.mykeebs.dev' \
     --validation-method DNS --region us-east-2 --profile kbdb-dev-admin
   ```
2. **Read the validation CNAME** ACM wants:
   ```sh
   aws acm describe-certificate --certificate-arn <arn-from-step-1> \
     --region us-east-2 --profile kbdb-dev-admin \
     --query 'Certificate.DomainValidationOptions[0].ResourceRecord'
   ```
3. **Create that CNAME in the `mykeebs.dev` Cloudflare zone**, DNS-only (not proxied) — via the Cloudflare dashboard, or the API/MCP tooling if you have it available.
4. **Wait for issuance**:
   ```sh
   aws acm wait certificate-validated --certificate-arn <arn-from-step-1> \
     --region us-east-2 --profile kbdb-dev-admin
   ```
5. **Add `CustomDomainName=api.<your-name>.mykeebs.dev` and `CustomDomainCertificateArn=<arn-from-step-1>`** to your section's `parameter_overrides` in `samconfig.toml` (see `[dev-jay.deploy.parameters]`). Leaving both out (the default) means no custom domain.
6. **Deploy** (`mise run deploy`), then read the stack's `ApiCustomDomainTarget` output and create a matching `api.<your-name>` CNAME in the `mykeebs.dev` Cloudflare zone (DNS-only, not proxied), pointed at that value. This one is a normal, static record — created once, not rewritten on later deploys.

### AWS accounts

Three accounts, one SSO session (`aws configure sso` once, then `aws sso login --profile <profile>` to refresh):

| Profile | Account | Purpose |
|---|---|---|
| `mgmt-admin` | `rogueserenity-management` | Administrative only, no workloads |
| `kbdb-dev-admin` | `kbdb-dev` | Hosts each developer's own stack |
| `kbdb-ci-admin` | `kbdb-ci` | CI's per-PR stacks |

There's no default AWS profile — commands will fail with `NoCredentials` unless `AWS_PROFILE` is set.

### First-time account bootstrap

New AWS account, never deployed to before? One-time steps, done once per account by whoever's setting it up (not needed for everyday `deploy`). **If you're forking this repo to deploy to your own account, you only need step 1** — steps 3-5 exist for this project's own separate CI account and don't apply to a single-account personal deploy; `.github/workflows/` is entirely specific to this project's own CI and isn't something you need to set up or replicate.

1. **Artifact bucket** (needed by everyone): `aws cloudformation deploy --template-file bootstrap/artifact-bucket.yaml --stack-name kbdb-bootstrap --profile <profile>`. Its name is `kbdb-sam-artifacts-<your-account-id>`; that's the `s3_bucket` value in your `samconfig.toml` section.
2. **ECR repo**: each environment's repo is created by `mise run setup` — nothing manual to do. For `kbdb-ci`'s shared repo, see [ECR repo for `kbdb-ci`](#ecr-repo-for-kbdb-ci) below.
3. **Cost budget** (only needed for accounts without their own app stack, e.g. `kbdb-ci`): `aws cloudformation deploy --template-file bootstrap/cost-budget.yaml --stack-name kbdb-cost-budget --profile <profile>`.
4. **(`kbdb-ci` only) GitHub Actions OIDC role**, so CI can authenticate to AWS: `aws cloudformation deploy --template-file bootstrap/ci-oidc-role.yaml --stack-name kbdb-ci-oidc --capabilities CAPABILITY_NAMED_IAM --profile kbdb-ci-admin`.
5. **(`kbdb-ci` only) JWKS bucket**, so CI's functional-test job can publish a publicly-fetchable OIDC discovery document + JWKS (generated by `oidc-testkit-gen`) for its per-PR stacks' native JWT authorizer to verify against: `aws cloudformation deploy --template-file bootstrap/jwks-bucket.yaml --stack-name kbdb-bootstrap-jwks --profile kbdb-ci-admin`. Then set the resulting `JWKSBucketName` output as the `JWKS_BUCKET_NAME` GitHub Actions repo variable (Settings → Secrets and variables → Actions → Variables) — `ci.yml`'s functional-test job fails at its "Generate OIDC signing material" step until that's set.

After all bootstraps, ordinary `sam deploy` calls (and, for `kbdb-ci`, CI's own workflow) work.

#### ECR repo for `kbdb-ci`

`sam deploy` pushes the function's image before it creates or updates the stack, so the repo can't live in `template.yaml` — it's in its own permanent stack from `bootstrap/ecr-repo.yaml`. `mise run setup` deploys one per environment. CI's per-PR stacks all share one repo, deployed by hand, once:

```sh
aws cloudformation deploy --template-file bootstrap/ecr-repo.yaml \
  --stack-name kbdb-ecr-bootstrap --profile kbdb-ci-admin
```

Its defaults (`RepositoryName=kbdb-api`, `ExpireBy=age`) are CI's: images expire 2 days after push, since nothing there is long-lived.

### Giving a developer scoped access (no admin needed)

Once the account is bootstrapped, day-to-day `setup`/`deploy`/`teardown` of a `kbdb-dev-*` stack don't need admin access — a much narrower policy covers exactly what those three scripts do. This is the setup for a fork you're deploying to your own AWS account, or for adding a teammate without handing them broad permissions:

1. **Account admin, once**: deploy the scoped policy.
   ```sh
   aws cloudformation deploy --template-file bootstrap/dev-user-policy.yaml \
     --stack-name kbdb-dev-user-policy --capabilities CAPABILITY_NAMED_IAM --profile <admin-profile>
   ```
   Attach the resulting policy (output as `DevPolicyArn`) to each developer's IAM user, group, or role:
   ```sh
   aws iam attach-user-policy --user-name <dev> --policy-arn <DevPolicyArn>
   ```
2. **Each developer**: with that policy attached (and no other permissions needed), run `mise run setup`, `deploy`, `teardown` as usual.

This policy is scoped to `kbdb-dev-*`-named stacks/resources only — it can't touch anything outside a developer's own stack, and can't grant itself broader access. It was verified by running the full `setup` → `deploy` → `teardown` cycle as a real IAM user with only this policy attached and nothing else.

## Testing strategy

Two test layers, kept separate — don't mix them:

- **Unit tests** (`*_test.go` next to the code they test): `testify/suite` + `mockery`-generated mocks. No real infra, no network calls.
- **Functional tests** (`test/functional/features/`): Ginkgo + Gomega, driving real HTTP/MCP calls against a running stack (local via `func-setup`, or a real deployed stack via `KBDB_API_BASE_URL`).

Functional specs follow a consistent BDD shape: `Describe` the subject (and, for multi-action resources, a nested `Describe` per action) → `Context("given <precondition>")` → `When("<action>")` → one `It` with a `By(...)` per assertion. Build the actual request inside the innermost `BeforeEach`, not an outer one — Ginkgo's context is scoped to the node that received it.

Functional tests run no auth server. [`oidc-testkit`](https://github.com/rogueserenity/oidc-testkit) generates a signing key + discovery doc + JWKS pre-deploy; the JSON is published where the deployed authorizer and `auth.NewVerifier` can fetch it (floci's S3 locally, the public `kbdb-jwks` bucket in CI), and the suite signs its own tokens via `pkg/oidctest` (see `test/functional/support/api/token.go`).

## Conventions

- **Commit messages and PR titles** follow [Conventional Commits](https://www.conventionalcommits.org/): `type(scope): subject` (e.g. `fix(ci): scope IAM permissions to account/region`). Common types: `feat`, `fix`, `chore`, `docs`, `test`, `ci`, `refactor`.
- **Changes to deployed behavior use `fix(...)` or `feat(...)`, never `chore(...)`.** Only commits that cut a release reach prod, and `chore` doesn't. That includes hand-written changes to `template.yaml`. Renovate follows the same rule: Dockerfile image and Go toolchain bumps are `fix(deps)`, while CI actions, dev tools and lock file maintenance stay `chore(deps)`.
- **Mise tasks live in `scripts/`**, one `.sh` file per task, referenced from `mise.toml` via `file = "scripts/<name>.sh"` — even one-liners. Add a new task the same way.
- Package layout, mocking patterns, and other code-level conventions are documented in `CLAUDE.md`.

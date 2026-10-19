# Prod runbook

What to do when something goes wrong in production, and how to do the rare jobs that only happen in prod.

**This is a living document.** Whenever prod surprises us, add how to handle it next time: a new section if it's a new kind of problem, or a step in the section it belongs to. A procedure that turned out to be wrong gets fixed here, not just in someone's memory.

## What prod is

| | |
|---|---|
| API | `https://api.mykeebs.info` |
| Web UI | `https://mykeebs.info` (the separate `mykeebs-web` repo) |
| AWS account | `kbdb-prod` (`416860882648`), region `us-east-2` |
| Stack | `kbdb-prod`, defined by the `[prod]` section of `samconfig.toml` |
| Identity provider | the Stytch **Live** project, issuer `https://auth.mykeebs.info` |
| Alarm emails | `kbdb-alerts-prd@mykeebs.com` |
| Budget emails | `kbdb-budget-prd@mykeebs.com` |

Prod is deployed only by `.github/workflows/deploy-prod.yml`, which runs for every published release. Nobody deploys it by hand.

## Before you start

Everything below that touches AWS needs the prod admin profile:

```sh
aws sso login --sso-session kbdb
export AWS_PROFILE=kbdb-prod-admin AWS_REGION=us-east-2
```

The deploy role GitHub Actions uses is deliberately limited: it can't delete the stack, a table or the images bucket, and it can't read or write any data. Anything here that does one of those needs the admin profile.

## When an alarm email arrives

Each alarm fires when its metric is above zero for one 5-minute period, and returns to OK by itself after a quiet period. An alarm that's back to OK still deserves a look: something failed.

To see what the API function logged around a time (change the start and end to bracket the alarm's timestamp, which is in UTC):

```sh
START=$(date -j -u -f %Y-%m-%dT%H:%M:%S 2026-10-19T18:10:00 +%s)000
END=$(date -j -u -f %Y-%m-%dT%H:%M:%S 2026-10-19T18:20:00 +%s)000
aws logs filter-log-events --log-group-name /aws/lambda/kbdb-api-kbdb-prod \
  --start-time $START --end-time $END --output json |
  jq -r '.events[] | "\(.timestamp/1000 | strftime("%H:%M:%S")) \(.message)"'
```

Use `/aws/lambda/kbdb-moderate-kbdb-prod` for the moderation function. Every request logs a `request started` and a `request finished` line with its method, path and status, so a failed request is easy to spot by its path.

| Alarm | What it means | Where to start |
|---|---|---|
| `ApiFunctionErrorsAlarm` | The API function crashed or timed out on a request. Normal error responses (4xx, or a 5xx the code returns on purpose) don't count. | Search the logs for `ERROR` and `Status: timeout`. A timeout lasts exactly 10 seconds, the function's limit. |
| `HttpApi5xxAlarm` | API Gateway returned a 5xx to a caller. It usually fires together with the one above. | Same as above. If it fires alone, the function is returning a 5xx itself: look for `"status":5` in the logs. |
| `ModerateFunctionErrorsAlarm` | The moderation function failed on an upload. Lambda retries it. | The moderation function's logs. If retries keep failing, the event ends up in the dead-letter queue (next row). |
| `ModerationDLQAlarm` | An upload failed moderation on every retry, so the image stays pending. | [Failed images](moderation.md#failed-images) in the moderation guide: list the queue, fix the cause, redrive. |

## When a budget email arrives

The stack's budget is $10 a month, with emails at 80% and 100% of actual spend. Prod normally costs pennies, so a budget email means something is being used far more than it should be.

1. **Find what's costing money:** Cost Explorer in the `kbdb-prod` console, grouped by service, daily. (Each Cost Explorer API call costs $0.01, so prefer the console.)
2. **If it's Rekognition**, someone is uploading a lot of images: pause moderation (`mise run moderation-pause prod`; see the [moderation guide](moderation.md#pausing-moderation)). The daily upload limit (`DailyUploadLimit`, 500 by default) already caps this.
3. **If it's S3 data transfer**, someone is downloading images heavily through presigned links. Use the image switch below.
4. **If you need to stop spending right now**, use one of the emergency switches below. Both are changes made by hand outside the template, so undo them by hand when you're done, and check they're still in place after any deploy that happens meanwhile.

Emergency switches (until #440 automates the image one):

```sh
# Stop all API traffic: every request fails until you undo it.
aws lambda put-function-concurrency --function-name kbdb-api-kbdb-prod \
  --reserved-concurrent-executions 0
# Undo (15 is ApiReservedConcurrency in samconfig.toml's [prod] section):
aws lambda put-function-concurrency --function-name kbdb-api-kbdb-prod \
  --reserved-concurrent-executions 15
```

That stops new image links being issued too, but links already handed out keep working until they expire (up to an hour). To stop those immediately, deny the role that signs them:

```sh
ROLE=$(aws cloudformation describe-stack-resource --stack-name kbdb-prod \
  --logical-resource-id PresignSigningRole \
  --query StackResourceDetail.PhysicalResourceId --output text)
aws iam put-role-policy --role-name "$ROLE" --policy-name emergency-deny-images \
  --policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"s3:GetObject","Resource":"*"}]}'
# Undo:
aws iam delete-role-policy --role-name "$ROLE" --policy-name emergency-deny-images
```

## Releases and deploys

### How a change reaches prod

1. A PR merges to `main`.
2. `release.yml` runs semantic-release. A `fix` or `feat` commit cuts a new version tag and GitHub Release; other types (`chore`, `docs`, `ci`, `test`, `refactor`) don't.
3. Publishing the release starts `deploy-prod.yml`, which deploys that tag to the `kbdb-prod` stack and then smoke-tests the API.

So a change that should reach prod must be a `fix` or `feat` (see [Conventions](../CONTRIBUTING.md#conventions)).

### A release broke something

**We never roll back.** Merge a fix; its release deploys forward like any other. That keeps prod always running a released version of `main`, and it means nobody has to remember what was rolled back.

### A deploy failed

Open the failed `Deploy Prod` run in GitHub Actions.

- **If it failed for a transient reason** (an AWS hiccup, a timeout pulling an image, a smoke test that hit a cold start), click **Re-run failed jobs**. There's deliberately no manual trigger for this workflow.
- **If it failed because of the change itself**, CloudFormation has already rolled the stack back to the previous version, so prod is still serving. Fix forward with a new release.
- **If the stack is stuck**, see the next section.

### The stack is stuck in `UPDATE_ROLLBACK_FAILED`

This happens when a release that added a resource the deploy role can't remove (a table, for example) fails and CloudFormation can't roll back. Prod keeps serving the previous version, but no release can deploy until you fix it.

1. Find the resource that couldn't be rolled back:
   ```sh
   aws cloudformation describe-stack-events --stack-name kbdb-prod \
     --query "StackEvents[?ResourceStatus=='DELETE_FAILED'].[LogicalResourceId,PhysicalResourceId,ResourceStatusReason]" \
     --output table
   ```
2. Tell CloudFormation to skip it and finish the rollback:
   ```sh
   aws cloudformation continue-update-rollback --stack-name kbdb-prod \
     --resources-to-skip <LogicalResourceId>
   aws cloudformation wait stack-rollback-complete --stack-name kbdb-prod
   ```
3. Delete the leftover resource by hand (its name is the `PhysicalResourceId` from step 1). It isn't part of the stack anymore.
4. Re-run the failed deploy, or merge a fix.

### A merge didn't produce a release

GitHub occasionally drops a push event, so `release.yml` never starts. Check the Actions tab for a `Release` run on the merge commit. If there isn't one:

```sh
gh workflow run release.yml
```

It releases every releasable commit since the last tag, so running it late loses nothing.

## Data

The user-data tables have deletion protection and point-in-time recovery (PITR) on. The images bucket has no versioning, so a deleted image is gone.

### Restoring a table to an earlier point in time

PITR can restore a table as it was at any second in the last 35 days, but only into a **new** table. The API reads the stack's own table, so restoring means copying data back into it.

1. Find the live table's name (`SwitchTableName`, `KeyboardTableName`, `KeycapSetTableName`, `BuildTableName`, `ProfileTableName` or `ProfileUsernameTableName`):
   ```sh
   LIVE=$(aws cloudformation describe-stacks --stack-name kbdb-prod \
     --query "Stacks[0].Outputs[?OutputKey=='KeyboardTableName'].OutputValue" --output text)
   ```
2. Restore it as a new table, at a time in UTC, then wait:
   ```sh
   aws dynamodb restore-table-to-point-in-time --source-table-name "$LIVE" \
     --target-table-name "$LIVE-restore" --restore-date-time 2026-10-19T18:00:00Z
   aws dynamodb wait table-exists --table-name "$LIVE-restore"
   ```
3. Look at the restored table (in the console, or with `aws dynamodb scan`) and make sure it's what you want.
4. Copy its items back into the live table. This overwrites items with the same key and leaves everything else alone:
   ```sh
   aws dynamodb scan --table-name "$LIVE-restore" --output json | jq -c '.Items[]' |
     while read -r item; do
       aws dynamodb put-item --table-name "$LIVE" --item "$item"
     done
   ```
   If items created after the restore point should go too, delete them from the live table by key. Do that one by one, deliberately: there's no undo.
5. Check through the API (or MCP) that the data looks right, then delete the restored table:
   ```sh
   aws dynamodb delete-table --table-name "$LIVE-restore"
   ```

Images aren't part of the restore. An item restored from before an image was deleted points to a file that no longer exists, and that image shows as missing.

### Backing up images

The only copy of an image is in the images bucket. To keep an offline copy of one account's data, images included, take a `kbdb-migrate dump` of it now and then (see the next section). The dump is a directory you can keep anywhere.

### Running `kbdb-migrate` against prod

The Stytch Live project keeps the redirect URL `http://localhost:8765/authorize.html` and the SDK domain `http://localhost:8765` permanently, so `kbdb-migrate login` works against prod. Live has dynamic client registration on, so no client ID is needed.

```sh
mise run migrate-build
bin/kbdb-migrate login --issuer https://auth.mykeebs.info
bin/kbdb-migrate dump --issuer https://auth.mykeebs.info --base-url https://api.mykeebs.info --out ./prod-dump
```

After a `restore`, run `verify`. See [`kbdb-migrate`](../CONTRIBUTING.md#backing-up--moving-a-users-data-kbdb-migrate) for the full tool.

## Decommissioning prod

`mise run teardown prod` refuses on purpose. Taking prod down is a manual job, with the admin profile:

1. Take a `kbdb-migrate dump` of every account whose data you want to keep.
2. Turn off deletion protection on every table:
   ```sh
   for t in $(aws dynamodb list-tables --query 'TableNames[?starts_with(@, `kbdb-prod-`)]' --output text); do
     aws dynamodb update-table --table-name "$t" --no-deletion-protection-enabled >/dev/null
   done
   ```
3. Empty the images bucket, then delete the stack:
   ```sh
   aws s3 rm s3://kbdb-images-416860882648-kbdb-prod --recursive
   aws cloudformation delete-stack --stack-name kbdb-prod
   aws cloudformation wait stack-delete-complete --stack-name kbdb-prod
   ```
4. Delete the image repository and its stack (the repository is kept when its stack is deleted):
   ```sh
   aws cloudformation delete-stack --stack-name kbdb-ecr-bootstrap-kbdb-prod
   aws ecr delete-repository --repository-name kbdb-api-kbdb-prod --force
   ```
5. Stop the GitHub deploys by deleting the `kbdb-prod-oidc` stack, and delete the `api.mykeebs.info` CNAME in Cloudflare.
6. The remaining bootstrap stacks (`kbdb-bootstrap`, the artifact bucket, and `kbdb-stack-boundary`) cost nothing to keep. Delete them only if the account itself is going away.


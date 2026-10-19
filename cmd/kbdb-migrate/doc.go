// Command kbdb-migrate dumps every entity a user owns (keyboards, switches,
// keycap sets, builds) and every associated S3 image to a local directory, and
// restores such a dump into another environment through the same public REST
// API.
//
// It exists to take a complete, verifiable offline copy before a
// backwards-incompatible data-model change, and doubles as a way to move a
// user's data between environments (dev to prod, either direction).
//
// # Why cmd/
//
// The repo convention (see CLAUDE.md) is that the Lambda *function* entrypoint
// lives at functions/api/, deliberately not cmd/api/, because functions/api/
// signals "this is a Lambda function" in an AWS-specific codebase. That
// reasoning is specific to the deployed service. kbdb-migrate is a standalone
// operational CLI run from a developer's machine, which is exactly what
// golang-standards/project-layout's cmd/<name>/ is for, and what internal/ and
// test/ in this repo already follow. So this one command does live under cmd/.
//
// # Subcommands
//
//	kbdb-migrate login    - obtain an access token via browser OAuth (Discord / email OTP)
//	kbdb-migrate dump      - download every entity + image into --out
//	kbdb-migrate restore   - recreate a dump's entities + images in --base-url's environment
//	kbdb-migrate verify    - check a restore against the dump (fields + image hashes)
//
// # What it does not do
//
// It never touches lookups, which are a static catalog built into the API;
// dump captures lookups/lookups.json for reference and diffing only, and
// restore ignores it.
//
// It does not dump or restore the profile. A profile's username is globally
// unique and identity-bound: moving data between two accounts means both
// accounts exist, so the source username is already taken and can't be
// recreated under the target without renaming or colliding. Profiles are set
// up per account by hand.
//
// It uses no AWS credentials. Image bytes move over presigned URLs the REST
// API mints, so it works identically against any environment the caller can
// reach.
package main

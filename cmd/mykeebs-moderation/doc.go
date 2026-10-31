// Command mykeebs-moderation is the operator's tool for image moderation
// decisions: list recent rejections, look at an image, approve a
// rejected image, or take down an approved one. There's no admin UI; the
// mise moderation-* tasks run this with the operator's own AWS
// credentials (a mykeebs-api-*-admin profile) against one environment's stack.
//
// It lives under cmd/ for the reasons cmd/mykeebs-migrate's doc gives. The
// status writes go through internal/moderation, the same code the
// moderation function uses.
package main

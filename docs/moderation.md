# Image moderation: operator guide

How to review what the moderation function rejected, take down an image someone reported, and undo either decision. There's no admin UI; everything here is a `mise run moderation-*` task backed by `cmd/mykeebs-moderation`.

## How moderation works

1. A user uploads an image. It starts as **pending**: the owner sees it with a "pending review" badge, and nobody else sees it.
2. The moderation function sends it to Amazon Rekognition and marks it **approved** (visible to everyone) or **rejected** (the owner sees a placeholder with the rejection reason, and nobody else sees anything).
3. A rejected image's file is tagged `moderation=rejected`. The bucket's lifecycle rule deletes tagged files 30 days after **upload** (`RejectedImageExpirationDays`). The database entry stays, so the owner keeps seeing the placeholder.

Your job is to look at each rejection before its file is deleted: **uphold** it if it's right, **approve** it if it's a false positive. You can also **reject** (take down) an approved image after a report.

## Before you start

```sh
aws sso login --profile mykeebs-api-dev-admin   # or mykeebs-api-prod-admin
export AWS_PROFILE=mykeebs-api-dev-admin
```

Every task takes an optional environment first (`dev-jay`, `prod`, …). It defaults to `dev-$(whoami)`. Where a task takes a `<key>`, you can pass either the image's object key or a link to the image (see [Taking down a reported image](#taking-down-a-reported-image)).

## Working the review queue

```sh
mise run moderation-rejections [env]
```

lists every rejection still waiting for you, newest first:

```
TIME                 ENTITY  OWNER       REASON         CONFIDENCE  KEY
2026-10-12 10:00:00  switch  user-abc…   Explicit       91.2        switches/user-abc…/…/image/…
2026-10-11 09:00:00  profile user-def…   unprocessable  -           profiles/user-def…/avatar/…
```

- **REASON** is the Rekognition label that triggered the rejection; `not_webp` when the file isn't a WebP, whatever its content type said; `too_large` when its longest side is over 2560px; or `unprocessable` when the WebP doesn't decode (corrupt or animated) or Rekognition couldn't read it.
- **CONFIDENCE** is Rekognition's confidence in that label, in percent.

For each row:

1. **Look at it:** `mise run moderation-view [env] <key>` opens the image in your browser through a 5-minute link. This works even though the site won't show a rejected image.
2. **Decide:**
   - The rejection is right: `mise run moderation-uphold [env] <key>`. Nothing changes for the image; the row just leaves your queue. The file is still deleted on schedule.
   - It's a false positive: `mise run moderation-approve [env] <key>`. The image becomes visible to everyone and its file no longer expires.

The queue is empty when the list says `No rejections awaiting review in the last 31d.` Rows also drop off by themselves when the owner deletes or replaces the image, since there's nothing left to decide.

By default the list covers the expiry period plus a day, which is every rejected file not yet deleted. `--since 7d` (or `12h`) narrows it, up to `60d`.

## Taking down a reported image

When someone reports an image on the site:

1. Right-click the image and copy its address.
2. `mise run moderation-reject [env] '<the copied link>' [reason]`. Quote the link: it contains `&`.

The image is rejected at once. The owner sees the placeholder with the reason you gave (`manual` if you gave none), and nobody else sees the image. Links the site already handed out stop working within seconds, except in a browser that has already loaded the image. If the command fails after rejecting, run it again: a rerun finishes the takedown.

A takedown is final once its file is deleted. The lifecycle rule counts from **upload**, so a takedown of an image older than 30 days is deleted at the rule's next nightly run (midnight UTC). If you might want to undo it, do so the same day.

## Undoing a decision

| You did | To undo it | Possible until |
|---|---|---|
| uphold | `moderation-approve` | the file is deleted (30 days after upload) |
| approve | `moderation-reject` | always |
| reject (takedown) | `moderation-approve` | the file is deleted: 30 days after upload, or the next midnight UTC for an older image |

Approve refuses an image whose file is gone or past its expiry date (`the image's file has expired`). That image can't be brought back; the owner has to upload it again.

## The record of what was decided

```sh
mise run moderation-rejections [env] --all
```

lists every rejection and takedown in the window, with a **REVIEW** column showing what happened to it:

| REVIEW | Meaning |
|---|---|
| `awaiting review` | still in your queue |
| `upheld 2026-10-13 15:04:05` | you upheld it then |
| `approved 2026-10-13 15:04:05` | you approved it then |
| `takedown` | you took down an approved image |
| `takedown (spam) 2026-10-13 …` | you took down an image the moderation function had rejected and you'd approved |
| `takedown, then approved …` | you took it down, then reversed it |

Every uphold, approve and reject writes one line to the moderation function's log group (stream `operator-reviews`). The logs are kept 60 days, so that's how far back the record goes.

## When a command fails partway

Run the same command again. Every task checks the current state first, so a rerun finishes what the first run didn't.

A few messages that look like errors but aren't:

| Message | Meaning |
|---|---|
| `… was already approved; nothing changed.` | a previous run already did it |
| `… was already rejected, so nothing was recorded …` | the image is rejected already. For one still in your queue, use uphold instead. |
| `Its file is already gone.` | the takedown worked; there was no file left to tag |
| `no item holds this image` | the owner deleted or replaced it, or the key or link is for another environment |
| `… is pending, not rejected` | the moderation function hasn't decided yet; try again in a minute |

If an image is stuck as pending for more than a few minutes, the moderation function failed on it; see [Failed images](#failed-images).

## Pausing moderation

```sh
mise run moderation-pause [env]    # stop moderating; new uploads stay pending
mise run moderation-resume [env]   # start again; also reports how many events failed
```

Pause if moderation misbehaves or costs more than it should. While paused, uploads still succeed but stay pending: the owner sees them with the "pending review" badge, nobody else sees them, and the daily upload limit still applies. Lambda keeps retrying each upload's event for up to 6 hours (`MaximumEventAgeInSeconds` in `template.yaml`), so after a pause shorter than that everything catches up by itself once you resume. Events older than that land in the dead-letter queue. A deploy doesn't undo a pause.

## Failed images

When the moderation function fails on an image three times in a row (or the image waited out a pause longer than 6 hours), Lambda gives up and puts the event in the moderation **dead-letter queue**. The image stays pending until you rerun it. The queue keeps events for 14 days.

```sh
mise run moderation-dlq [env]                  # list what failed, and why
mise run moderation-redrive [env]              # rerun all of them
mise run moderation-redrive [env] --key <key>  # rerun one image
```

`moderation-dlq` shows each event's time, why Lambda gave up (`RetriesExhausted` or `EventAgeExceeded`), how many attempts it made, the image key and the last error. Listing doesn't remove anything.

`moderation-redrive` runs the moderation function on each event and removes it from the queue once it succeeds. It stops at the first one that fails again, and leaves that one and the rest queued, so fix the cause (the error column says what it was) before trying again. Redriving is always safe: an image that's no longer pending (deleted, replaced, or already decided) is skipped without calling Rekognition. A redrive while moderation is paused fails and leaves everything queued, so resume first.

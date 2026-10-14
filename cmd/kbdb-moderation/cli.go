package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/rogueserenity/kbdb/internal/imagekey"
	"github.com/rogueserenity/kbdb/internal/moderation"
)

// viewURLLifetime is how long a view URL works.
const viewURLLifetime = 5 * time.Minute

// CLI is the kbdb-moderation command tree, parsed by Kong.
type CLI struct {
	Stack  string `required:"" help:"CloudFormation stack name, e.g. kbdb-dev-jay."`
	Region string `env:"AWS_REGION" help:"AWS region of the stack."`

	Rejections RejectionsCmd `cmd:"" help:"List the moderation function's rejections still awaiting review, or with --all every rejection and takedown and how it was reviewed."`
	View       ViewCmd       `cmd:"" help:"Open an image in the browser, whatever its status."`
	Uphold     UpholdCmd     `cmd:"" help:"Record that a rejection was reviewed and stands. The image stays rejected and still expires."`
	Approve    ApproveCmd    `cmd:"" help:"Override a rejection: mark the image approved and stop it expiring."`
	Reject     RejectCmd     `cmd:"" help:"Take down an approved image: mark it rejected and tag it to expire. The expiry counts from upload, so an older image is deleted within a day."`
}

// RejectionsCmd lists rejections from the moderation function's logs.
type RejectionsCmd struct {
	Since string `help:"How far back to look: a number of days (7d) or a Go duration (12h). Defaults to the stack's RejectedImageExpirationDays plus a day, which covers every rejected file not yet deleted."`
	All   bool   `help:"Also list reviewed rejections and takedowns, with how each was reviewed."`
}

func (c *RejectionsCmd) Run(ctx context.Context, st *stack) error {
	if c.Since == "" {
		c.Since = defaultSince(st.expiryDays)
	}
	since, err := parseSince(c.Since)
	if err != nil {
		return err
	}
	rs, truncated, err := moderation.Rejections(ctx, cloudwatchlogs.NewFromConfig(st.cfg), st.images, st.logGroup, time.Now().Add(-since), c.All)
	if err != nil {
		return err
	}
	if len(rs) == 0 {
		if c.All {
			fmt.Printf("No rejections in the last %s.\n", c.Since)
		} else {
			fmt.Printf("No rejections awaiting review in the last %s.\n", c.Since)
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	header := "TIME\tENTITY\tOWNER\tREASON\tCONFIDENCE\tKEY"
	if c.All {
		header = "TIME\tENTITY\tOWNER\tREASON\tCONFIDENCE\tREVIEW\tKEY"
	}
	_, _ = fmt.Fprintln(w, header)
	for _, r := range rs {
		ref, _ := imagekey.Parse(r.Key)
		confidence := "-"
		if r.Confidence > 0 {
			confidence = strconv.FormatFloat(float64(r.Confidence), 'f', 1, 32)
		}
		row := []string{r.Time.Local().Format(time.DateTime), string(ref.Entity), ref.OwnerID, r.Reason, confidence}
		if c.All {
			row = append(row, reviewColumn(r))
		}
		_, _ = fmt.Fprintln(w, strings.Join(append(row, r.Key), "\t"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if truncated {
		fmt.Printf("The query hit its %d-line limit, so older rejections may be missing; narrow --since to see them.\n", moderation.RejectionsLimit)
	}
	return nil
}

// reviewColumn describes how r was reviewed.
func reviewColumn(r moderation.Rejection) string {
	switch {
	case r.Review == nil:
		return "awaiting review"
	case r.Manual && r.Review.Action == moderation.ActionRejected:
		return "takedown"
	case r.Manual:
		return "takedown, then " + r.Review.Action + " " + r.Review.Time.Local().Format(time.DateTime)
	case r.Review.Action == moderation.ActionRejected:
		return "takedown (" + r.Review.Reason + ") " + r.Review.Time.Local().Format(time.DateTime)
	}
	return r.Review.Action + " " + r.Review.Time.Local().Format(time.DateTime)
}

// logRetention is ModerateFunctionLogGroup's RetentionInDays in
// template.yaml: --since can't usefully reach further back.
const logRetention = 60 * 24 * time.Hour

// defaultSince covers every rejected file not yet deleted: the lifecycle
// rule deletes at the first midnight UTC after expiryDays, so a file can
// outlive that by up to a day. It's capped at logRetention.
func defaultSince(expiryDays int) string {
	return fmt.Sprintf("%dd", min(expiryDays+1, int(logRetention/(24*time.Hour))))
}

// parseSince reads a --since value: whole days as "7d", or anything
// time.ParseDuration accepts, up to logRetention.
func parseSince(s string) (time.Duration, error) {
	var d time.Duration
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 || n > int(logRetention/(24*time.Hour)) {
			return 0, fmt.Errorf("invalid --since %q: want 1d to 60d, or a duration", s)
		}
		d = time.Duration(n) * 24 * time.Hour
	} else {
		var err error
		if d, err = time.ParseDuration(s); err != nil || d <= 0 {
			return 0, fmt.Errorf("invalid --since %q: want 1d to 60d, or a duration", s)
		}
	}
	if d > logRetention {
		return 0, fmt.Errorf("invalid --since %q: decision logs are kept 60 days", s)
	}
	return d, nil
}

// ViewCmd presigns a short-lived GET for an image with the operator's
// credentials, which works even for a rejected image the API won't serve.
type ViewCmd struct {
	Key string `arg:"" help:"The image's object key, or any link to it."`
}

func (c *ViewCmd) Run(ctx context.Context, st *stack) error {
	key, err := keyArg(c.Key, st.bucket)
	if err != nil {
		return err
	}
	client := s3.NewFromConfig(st.cfg)
	if _, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &st.bucket, Key: &key}); err != nil {
		return fmt.Errorf("reading %q: %w", key, err)
	}
	req, err := s3.NewPresignClient(client).PresignGetObject(ctx,
		&s3.GetObjectInput{Bucket: &st.bucket, Key: &key},
		s3.WithPresignExpires(viewURLLifetime))
	if err != nil {
		return fmt.Errorf("presigning %q: %w", key, err)
	}
	fmt.Printf("Opening the image in your browser. The link works for %s:\n%s\n", viewURLLifetime, req.URL)
	openBrowser(ctx, req.URL)
	return nil
}

// UpholdCmd records that a rejection stands.
type UpholdCmd struct {
	Key string `arg:"" help:"The rejected image's object key, or any link to it."`
}

func (c *UpholdCmd) Run(ctx context.Context, st *stack) error {
	key, err := keyArg(c.Key, st.bucket)
	if err != nil {
		return err
	}
	if err := st.overrides().Uphold(ctx, key); err != nil {
		return err
	}
	fmt.Printf("Upheld the rejection of %s.\n", key)
	return nil
}

// ApproveCmd overrides a rejection.
type ApproveCmd struct {
	Key string `arg:"" help:"The rejected image's object key, or any link to it."`
}

func (c *ApproveCmd) Run(ctx context.Context, st *stack) error {
	key, err := keyArg(c.Key, st.bucket)
	if err != nil {
		return err
	}
	already, untagged, err := st.overrides().Approve(ctx, key)
	if err != nil {
		return err
	}
	switch {
	case already && untagged:
		fmt.Printf("%s was already approved; removed its rejected tag.\n", key)
		return nil
	case already:
		fmt.Printf("%s was already approved; nothing changed.\n", key)
		return nil
	}
	fmt.Printf("Approved %s.\n", key)
	return nil
}

// RejectCmd takes down an approved image.
type RejectCmd struct {
	Key    string `arg:"" help:"The approved image's object key, or any link to it, e.g. one copied from the site."`
	Reason string `arg:"" optional:"" default:"manual" help:"Rejection reason the owner sees."`
}

func (c *RejectCmd) Run(ctx context.Context, st *stack) error {
	key, err := keyArg(c.Key, st.bucket)
	if err != nil {
		return err
	}
	already, gone, err := st.overrides().Reject(ctx, key, c.Reason)
	if err != nil {
		return err
	}
	switch {
	case already:
		fmt.Printf("%s was already rejected, so nothing was recorded and its reason wasn't changed. To clear a rejection awaiting review, use uphold.\n", key)
	case gone:
		fmt.Printf("Rejected %s (%s). Its file is already gone.\n", key, c.Reason)
	default:
		fmt.Printf("Rejected %s (%s). Download links issued before now keep working until they expire, within an hour.\n", key, c.Reason)
	}
	return nil
}

// keyArg returns the object key an argument names: the key itself, or an
// image link such as the presigned GET the site loads it from, expired or
// not. A link's path is the key, after the bucket for a path-style one.
func keyArg(arg, bucket string) (string, error) {
	if !strings.HasPrefix(arg, "https://") && !strings.HasPrefix(arg, "http://") {
		return arg, nil
	}
	u, err := url.Parse(arg)
	if err != nil {
		return "", fmt.Errorf("invalid image link %q: %w", arg, err)
	}
	key := strings.TrimPrefix(u.Path, "/")
	return strings.TrimPrefix(key, bucket+"/"), nil
}

// openBrowser best-effort opens url in the system browser. Failures are
// ignored: the caller always prints the URL as a fallback.
func openBrowser(ctx context.Context, url string) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	_ = exec.CommandContext(ctx, name, args...).Start() //nolint:gosec // fixed launcher, our own URL.
}

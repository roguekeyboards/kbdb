package main

import (
	"context"
	"fmt"
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

	Rejections RejectionsCmd `cmd:"" help:"List the moderation function's recent rejections. Manual rejections aren't listed."`
	View       ViewCmd       `cmd:"" help:"Open an image in the browser, whatever its status."`
	Approve    ApproveCmd    `cmd:"" help:"Override a rejection: mark the image approved and stop it expiring."`
	Reject     RejectCmd     `cmd:"" help:"Take down an approved image: mark it rejected. Unlike the moderation function's rejections, the file is kept until it's approved again or its owner removes it."`
}

// RejectionsCmd lists rejections from the moderation function's logs.
type RejectionsCmd struct {
	Since string `default:"7d" help:"How far back to look: a number of days (7d) or a Go duration (12h)."`
}

func (c *RejectionsCmd) Run(ctx context.Context, st *stack) error {
	since, err := parseSince(c.Since)
	if err != nil {
		return err
	}
	rs, truncated, err := moderation.Rejections(ctx, cloudwatchlogs.NewFromConfig(st.cfg), st.logGroup, time.Now().Add(-since))
	if err != nil {
		return err
	}
	if len(rs) == 0 {
		fmt.Printf("No rejections in the last %s.\n", c.Since)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TIME\tENTITY\tOWNER\tREASON\tCONFIDENCE\tKEY")
	for _, r := range rs {
		ref, _ := imagekey.Parse(r.Key)
		confidence := "-"
		if r.Confidence > 0 {
			confidence = strconv.FormatFloat(float64(r.Confidence), 'f', 1, 32)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Time.Local().Format(time.DateTime), ref.Entity, ref.OwnerID, r.Reason, confidence, r.Key)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if truncated {
		fmt.Printf("The query hit its %d-line limit, so older rejections may be missing; narrow --since to see them.\n", moderation.RejectionsLimit)
	}
	return nil
}

// logRetention is ModerateFunctionLogGroup's RetentionInDays in
// template.yaml: --since can't usefully reach further back.
const logRetention = 60 * 24 * time.Hour

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
	Key string `arg:"" help:"The image's object key."`
}

func (c *ViewCmd) Run(ctx context.Context, st *stack) error {
	client := s3.NewFromConfig(st.cfg)
	if _, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &st.bucket, Key: &c.Key}); err != nil {
		return fmt.Errorf("reading %q: %w", c.Key, err)
	}
	req, err := s3.NewPresignClient(client).PresignGetObject(ctx,
		&s3.GetObjectInput{Bucket: &st.bucket, Key: &c.Key},
		s3.WithPresignExpires(viewURLLifetime))
	if err != nil {
		return fmt.Errorf("presigning %q: %w", c.Key, err)
	}
	fmt.Printf("Opening the image in your browser. The link works for %s:\n%s\n", viewURLLifetime, req.URL)
	openBrowser(ctx, req.URL)
	return nil
}

// ApproveCmd overrides a rejection.
type ApproveCmd struct {
	Key string `arg:"" help:"The rejected image's object key."`
}

func (c *ApproveCmd) Run(ctx context.Context, st *stack) error {
	already, untagged, err := st.overrides().Approve(ctx, c.Key)
	if err != nil {
		return err
	}
	switch {
	case already && untagged:
		fmt.Printf("%s was already approved; removed its rejected tag.\n", c.Key)
		return nil
	case already:
		fmt.Printf("%s was already approved; nothing changed.\n", c.Key)
		return nil
	}
	fmt.Printf("Approved %s.\n", c.Key)
	return nil
}

// RejectCmd takes down an approved image.
type RejectCmd struct {
	Key    string `arg:"" help:"The approved image's object key."`
	Reason string `arg:"" optional:"" default:"manual" help:"Rejection reason the owner sees."`
}

func (c *RejectCmd) Run(ctx context.Context, st *stack) error {
	already, err := st.overrides().Reject(ctx, c.Key, c.Reason)
	if err != nil {
		return err
	}
	if already {
		fmt.Printf("%s was already rejected; its reason is unchanged.\n", c.Key)
		return nil
	}
	fmt.Printf("Rejected %s (%s). Download links issued before now keep working until they expire, within an hour.\n", c.Key, c.Reason)
	return nil
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

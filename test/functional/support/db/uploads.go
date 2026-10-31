package db

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
)

// uploadDays is today and tomorrow (UTC), so a request that lands just
// after midnight still sees a full day.
func uploadDays() []string {
	today := time.Now().UTC()
	return []string{today.Format(time.DateOnly), today.Add(24 * time.Hour).Format(time.DateOnly)}
}

// FillDailyUploads puts the daily upload count past any limit, so the next
// upload-URL request is refused. Run it only from a Serial spec: every
// other spec's uploads would be refused too.
func FillDailyUploads(ctx context.Context) error {
	table := NewDynamoTable(ctx, support.UploadCounterTableName())
	var errs []error
	for _, day := range uploadDays() {
		errs = append(errs, table.PutItem(ctx, map[string]any{"day": day, "count": math.MaxInt32}))
	}
	return errors.Join(errs...)
}

// ClearDailyUploads removes the counts FillDailyUploads wrote.
func ClearDailyUploads(ctx context.Context) error {
	table := NewDynamoTable(ctx, support.UploadCounterTableName())
	var errs []error
	for _, day := range uploadDays() {
		errs = append(errs, table.DeleteItem(ctx, map[string]string{"day": day}))
	}
	return errors.Join(errs...)
}

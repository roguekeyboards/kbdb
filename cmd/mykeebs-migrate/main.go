package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
)

// Version is set at build time via -ldflags "-X main.Version=...". It is
// recorded in a dump's manifest.json so a restore run can tell which build
// produced the dump.
var Version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	var cli CLI
	kctx := kong.Parse(&cli,
		kong.Name("mykeebs-migrate"),
		kong.Description("Dump and restore a mykeebs-api user's entities and images via the public REST API."),
		kong.UsageOnError(),
		kong.BindTo(context.Background(), (*context.Context)(nil)),
	)

	kctx.FatalIfErrorf(kctx.Run())
}

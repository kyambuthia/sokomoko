package main

import (
	"log/slog"
	"os"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		slog.Error("sokomoko exited with an error", "error", err)
		os.Exit(1)
	}
}

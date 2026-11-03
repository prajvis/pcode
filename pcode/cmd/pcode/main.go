// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/prajvis/pcode/pcode/internal/app"
	"github.com/prajvis/pcode/pcode/internal/config"
)

// version is set by -ldflags "-X main.version=..." at release time.
var version = "0.1.1"

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	a := app.New(cfg)
	a.Version = version
	if err := a.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "pcode: %v\n", err)
		os.Exit(app.ExitCode(err))
	}
}

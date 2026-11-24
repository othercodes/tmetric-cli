// Command tmetric is an unofficial command-line interface for TMetric time
// tracking (https://tmetric.com).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	// The TMetric timezone must load on machines without a zone database,
	// such as Windows.
	_ "time/tzdata"

	"github.com/othercodes/tmetric-cli/internal/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(cmd.Execute(ctx))
}

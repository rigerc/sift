// sift scans workspaces and generates plans; it never installs skills.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rigerc/sift/cmd"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "sift: %v\n", err)
		return 1
	}
	return 0
}

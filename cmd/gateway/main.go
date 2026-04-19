// Command gateway is the single binary entrypoint for the go-ya AI Gateway.
// It exposes both the HTTP server and all administrative CLI commands.
//
// Usage:
//
//	gateway serve               # Start the HTTP gateway
//	gateway providers add ...   # Register a provider
//	gateway keys add ...        # Add an API key
//	gateway sessions list ...   # Inspect sessions
package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	gatewaycli "github.com/tnotstar/go-ya/internal/cli"
)

func main() {
	app := &cli.App{
		Name:    "gateway",
		Usage:   "AI Gateway — OpenAI-compatible LLM routing gateway",
		Version: "0.1.0",
		Commands: []*cli.Command{
			gatewaycli.ServeCommand(),
			gatewaycli.ProvidersCommand(),
			gatewaycli.KeysCommand(),
			gatewaycli.SessionsCommand(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

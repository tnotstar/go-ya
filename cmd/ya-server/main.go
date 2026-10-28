// Command `ya-server` is the single binary entrypoint for the go-ya AI Gateway.
// It exposes both the HTTP server and all administrative CLI commands.
//
// Usage:
//
//	ya-server serve               # Start the HTTP gateway
//	ya-server providers add ...   # Register a provider
//	ya-server keys add ...        # Add an API key
//	ya-server sessions list ...   # Inspect sessions
package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	yacli "github.com/tnotstar/go-ya/internal/cli"
)

func main() {
	app := &cli.App{
		Name:    "ya-server",
		Usage:   "OpenAI-compatible LLM routing gateway",
		Version: "0.1.0",
		Commands: []*cli.Command{
			yacli.ServeCommand(),
			yacli.ProvidersCommand(),
			yacli.KeysCommand(),
			yacli.SessionsCommand(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

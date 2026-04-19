package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/urfave/cli/v2"

	"github.com/tnotstar/go-ya/internal/domain"
	"github.com/tnotstar/go-ya/internal/persistence"
)

// ProvidersCommand returns the `providers` command group.
func ProvidersCommand() *cli.Command {
	return &cli.Command{
		Name:  "providers",
		Usage: "Manage upstream LLM providers",
		Subcommands: []*cli.Command{
			providersAddCommand(),
			providersListCommand(),
			providersEnableCommand(),
			providersDisableCommand(),
		},
	}
}

func providersAddCommand() *cli.Command {
	return &cli.Command{
		Name:  "add",
		Usage: "Register a new provider",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Required: true, Usage: "Unique provider name (e.g. 'openai')"},
			&cli.StringFlag{Name: "type", Required: true, Usage: "Provider type: openai|openrouter|gemini|abacus|llamacpp|mock"},
			&cli.StringFlag{Name: "base-url", Usage: "Override default base URL"},
			&cli.BoolFlag{Name: "local", Usage: "Mark as local provider (skips key encryption requirement)"},
			&cli.StringFlag{Name: "auth-header", Usage: "Custom auth header name (e.g. 'apiKey')"},
		},
		Action: func(c *cli.Context) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			repo := persistence.NewProviderRepo(db)

			ptype := domain.ProviderType(c.String("type"))
			baseURL := c.String("base-url")
			if baseURL == "" {
				baseURL = defaultBaseURL(ptype)
			}

			p := &domain.Provider{
				Name:           c.String("name"),
				Type:           ptype,
				BaseURL:        baseURL,
				Enabled:        true,
				IsLocal:        c.Bool("local"),
				AuthHeaderName: c.String("auth-header"),
			}

			if err := repo.Create(c.Context, p); err != nil {
				return fmt.Errorf("providers add: %w", err)
			}

			fmt.Printf("Provider %q created with ID: %s\n", p.Name, p.ID)
			return nil
		},
	}
}

func providersListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all registered providers",
		Action: func(c *cli.Context) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			repo := persistence.NewProviderRepo(db)

			providers, err := repo.ListAll(c.Context)
			if err != nil {
				return fmt.Errorf("providers list: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tTYPE\tENABLED\tBASE URL")
			for _, p := range providers {
				fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\n", p.ID, p.Name, p.Type, p.Enabled, p.BaseURL)
			}
			return w.Flush()
		},
	}
}

func providersEnableCommand() *cli.Command {
	return &cli.Command{
		Name:      "enable",
		Usage:     "Enable a provider by ID",
		ArgsUsage: "<provider-id>",
		Action: func(c *cli.Context) error {
			id := c.Args().First()
			if id == "" {
				return fmt.Errorf("provider ID is required")
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			return persistence.NewProviderRepo(db).SetEnabled(c.Context, id, true)
		},
	}
}

func providersDisableCommand() *cli.Command {
	return &cli.Command{
		Name:      "disable",
		Usage:     "Disable a provider by ID",
		ArgsUsage: "<provider-id>",
		Action: func(c *cli.Context) error {
			id := c.Args().First()
			if id == "" {
				return fmt.Errorf("provider ID is required")
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			return persistence.NewProviderRepo(db).SetEnabled(c.Context, id, false)
		},
	}
}

// defaultBaseURL returns the canonical base URL for a given provider type.
func defaultBaseURL(t domain.ProviderType) string {
	switch t {
	case domain.ProviderTypeOpenAI:
		return "https://api.openai.com/v1"
	case domain.ProviderTypeOpenRouter:
		return "https://openrouter.ai/api/v1"
	case domain.ProviderTypeGemini:
		return "https://generativelanguage.googleapis.com"
	case domain.ProviderTypeAbacus:
		return "https://api.abacus.ai/api/v1"
	case domain.ProviderTypeLlamaCPP:
		return "http://localhost:8080/v1"
	default:
		return ""
	}
}

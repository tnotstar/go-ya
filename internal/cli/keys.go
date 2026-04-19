package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v2"

	"github.com/tnotstar/go-ya/internal/config"
	"github.com/tnotstar/go-ya/internal/persistence"
	"github.com/tnotstar/go-ya/internal/services"
)

// KeysCommand returns the `keys` command group.
func KeysCommand() *cli.Command {
	return &cli.Command{
		Name:  "keys",
		Usage: "Manage provider API keys",
		Subcommands: []*cli.Command{
			keysAddCommand(),
			keysListCommand(),
			keysEnableCommand(),
			keysDisableCommand(),
			keysRotateCommand(),
		},
	}
}

func keysAddCommand() *cli.Command {
	return &cli.Command{
		Name:  "add",
		Usage: "Add an encrypted API key for a provider",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "provider", Required: true, Usage: "Provider name (e.g. 'openai')"},
			&cli.StringFlag{Name: "label", Required: true, Usage: "Human-readable label for this key"},
			&cli.StringFlag{Name: "key", Usage: "API key value (reads from stdin if omitted)"},
		},
		Action: func(c *cli.Context) error {
			rawKey := c.String("key")
			if rawKey == "" {
				rawKey = readKeyFromStdin()
			}
			if rawKey == "" {
				return fmt.Errorf("keys add: API key cannot be empty")
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			masterKey, err := cfg.MasterKeyBytes()
			if err != nil {
				return err
			}

			provRepo := persistence.NewProviderRepo(db)
			provider, err := provRepo.GetByName(c.Context, c.String("provider"))
			if err != nil {
				return fmt.Errorf("keys add: provider %q not found", c.String("provider"))
			}

			keyRepo := persistence.NewKeyRepo(db)
			keyStore := services.NewKeyStore(keyRepo, masterKey)

			if err := keyStore.AddKey(c.Context, provider.ID, c.String("label"), rawKey); err != nil {
				return fmt.Errorf("keys add: %w", err)
			}

			fmt.Printf("Key %q added for provider %q\n", c.String("label"), provider.Name)
			return nil
		},
	}
}

func keysListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List API keys for a provider",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "provider", Usage: "Provider name (optional; lists all if omitted)"},
		},
		Action: func(c *cli.Context) error {
			db, err := openDB()
			if err != nil {
				return err
			}

			var keys []any // will use tabwriter
			keyRepo := persistence.NewKeyRepo(db)

			if name := c.String("provider"); name != "" {
				provRepo := persistence.NewProviderRepo(db)
				p, err := provRepo.GetByName(c.Context, name)
				if err != nil {
					return fmt.Errorf("keys list: provider not found: %w", err)
				}
				k, err := keyRepo.ListByProvider(c.Context, p.ID)
				if err != nil {
					return err
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "ID\tLABEL\tENABLED\tLAST USED")
				for _, key := range k {
					lastUsed := "never"
					if key.LastUsedAt != nil {
						lastUsed = key.LastUsedAt.Format("2006-01-02 15:04:05")
					}
					fmt.Fprintf(w, "%s\t%s\t%v\t%s\n", key.ID, key.Label, key.Enabled, lastUsed)
				}
				_ = w.Flush()
				_ = keys
			} else {
				fmt.Println("Use --provider to filter keys by provider name.")
			}
			return nil
		},
	}
}

func keysEnableCommand() *cli.Command {
	return &cli.Command{
		Name:      "enable",
		Usage:     "Enable a key by ID",
		ArgsUsage: "<key-id>",
		Action: func(c *cli.Context) error {
			id := c.Args().First()
			if id == "" {
				return fmt.Errorf("key ID is required")
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			cfg, _ := config.Load()
			masterKey, _ := cfg.MasterKeyBytes()
			return services.NewKeyStore(persistence.NewKeyRepo(db), masterKey).EnableKey(c.Context, id)
		},
	}
}

func keysDisableCommand() *cli.Command {
	return &cli.Command{
		Name:      "disable",
		Usage:     "Disable a key by ID",
		ArgsUsage: "<key-id>",
		Action: func(c *cli.Context) error {
			id := c.Args().First()
			if id == "" {
				return fmt.Errorf("key ID is required")
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			cfg, _ := config.Load()
			masterKey, _ := cfg.MasterKeyBytes()
			return services.NewKeyStore(persistence.NewKeyRepo(db), masterKey).DisableKey(c.Context, id)
		},
	}
}

func keysRotateCommand() *cli.Command {
	return &cli.Command{
		Name:  "rotate",
		Usage: "Rotate a key: disable old and add new key atomically",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "id", Required: true, Usage: "ID of the key to rotate"},
			&cli.StringFlag{Name: "label", Required: true, Usage: "Label for the new key"},
			&cli.StringFlag{Name: "key", Usage: "New API key value (reads from stdin if omitted)"},
		},
		Action: func(c *cli.Context) error {
			newRawKey := c.String("key")
			if newRawKey == "" {
				newRawKey = readKeyFromStdin()
			}
			if newRawKey == "" {
				return fmt.Errorf("keys rotate: new key cannot be empty")
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			masterKey, err := cfg.MasterKeyBytes()
			if err != nil {
				return err
			}

			keyStore := services.NewKeyStore(persistence.NewKeyRepo(db), masterKey)
			if err := keyStore.RotateKey(c.Context, c.String("id"), c.String("label"), newRawKey); err != nil {
				return fmt.Errorf("keys rotate: %w", err)
			}

			fmt.Printf("Key %q rotated successfully.\n", c.String("id"))
			return nil
		},
	}
}

// readKeyFromStdin reads a single line from stdin (used for secure key input).
func readKeyFromStdin() string {
	fmt.Print("Enter API key: ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

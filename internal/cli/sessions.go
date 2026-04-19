package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/urfave/cli/v2"

	"github.com/tnotstar/go-ya/internal/persistence"
)

// SessionsCommand returns the `sessions` command group.
func SessionsCommand() *cli.Command {
	return &cli.Command{
		Name:  "sessions",
		Usage: "Inspect conversation sessions",
		Subcommands: []*cli.Command{
			sessionsListCommand(),
			sessionsShowCommand(),
		},
	}
}

func sessionsListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List sessions for a user",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "user", Required: true, Usage: "User ID to list sessions for"},
			&cli.IntFlag{Name: "limit", Value: 20, Usage: "Maximum number of sessions to show"},
		},
		Action: func(c *cli.Context) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			repo := persistence.NewSessionRepo(db)
			sessions, err := repo.ListByUser(c.Context, c.String("user"), c.Int("limit"))
			if err != nil {
				return fmt.Errorf("sessions list: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ID\tCLIENT\tLAST PROVIDER\tLAST MODEL\tCREATED")
			for _, s := range sessions {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					s.ID, s.ClientType, s.LastProvider, s.LastModel,
					s.CreatedAt.Format("2006-01-02 15:04:05"))
			}
			return w.Flush()
		},
	}
}

func sessionsShowCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show messages for a session",
		ArgsUsage: "<session-id>",
		Action: func(c *cli.Context) error {
			id := c.Args().First()
			if id == "" {
				return fmt.Errorf("session ID is required")
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			msgRepo := persistence.NewMessageRepo(db)
			msgs, err := msgRepo.ListBySession(c.Context, id)
			if err != nil {
				return fmt.Errorf("sessions show: %w", err)
			}

			for _, m := range msgs {
				fmt.Printf("[%s] %s: %s\n", m.CreatedAt.Format("15:04:05"), m.Role, m.Content)
			}
			return nil
		},
	}
}

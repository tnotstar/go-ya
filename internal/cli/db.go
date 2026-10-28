package cli

import (
	"fmt"

	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"

	"github.com/tnotstar/go-ya/internal/config"
	"github.com/tnotstar/go-ya/internal/persistence"
)

// openDB is a helper used by CLI commands to open the database using
// the same config source as the serve command.
func openDB() (*gorm.DB, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("cli: config: %w", err)
	}
	return persistence.Open(cfg.DBDriver, cfg.DBDSN, gormlog.Silent)
}

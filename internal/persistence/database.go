// Package persistence provides GORM-based database initialization and migration.
package persistence

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tnotstar/go-ya/internal/domain"
)

// Open initializes a GORM database connection for the given driver and DSN.
// Supported drivers: "sqlite", "postgres".
// AutoMigrate is called to ensure schema is current.
func Open(driver, dsn string, logLevel logger.LogLevel) (*gorm.DB, error) {
	cfg := &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	}

	var dialector gorm.Dialector
	switch driver {
	case "sqlite":
		// Enable WAL mode via pragmas in the DSN for better concurrency.
		if dsn != ":memory:" {
			dsn += "?_journal_mode=WAL&_foreign_keys=on"
		}
		dialector = sqlite.Open(dsn)
	case "postgres":
		dialector = postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("persistence: unsupported DB driver %q", driver)
	}

	db, err := gorm.Open(dialector, cfg)
	if err != nil {
		return nil, fmt.Errorf("persistence: open DB: %w", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("persistence: migrate: %w", err)
	}

	return db, nil
}

// migrate runs GORM AutoMigrate for all domain models.
// For production deployments with strict migration requirements, replace this
// with a versioned migration tool (e.g., golang-migrate) and disable AutoMigrate.
func migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&domain.Provider{},
		&domain.ProviderKey{},
		&domain.Session{},
		&domain.Message{},
	)
}

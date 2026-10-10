// Package store provides database (GORM) and Redis client initialization.
//
// It supports SQLite (modernc pure-Go driver, CGO_ENABLED=0) and MySQL
// with a unified factory pattern, and connection pool configuration.
package store

import (
	"fmt"
	"time"

	"github.com/go-api-gateway/gateway/internal/config"
	"github.com/go-api-gateway/gateway/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewDB creates a GORM database connection based on the configuration.
// Supports SQLite (pure Go modernc driver) and MySQL.
//
// Parameters:
//   - cfg: configuration store settings (driver, dsn, pool config)
//
// Returns:
//   - *gorm.DB: initialized database connection
//   - error: connection or migration error
func NewDB(cfg config.ConfigStoreConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch cfg.Driver {
	case "sqlite":
		// modernc.org/sqlite pure Go driver, no CGO required (P0-1)
		dialector = sqlite.Open(cfg.DSN)
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}

	// Configure connection pool
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 100
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 20
	}
	maxLifetime := cfg.ConnMaxLifetime
	if maxLifetime <= 0 {
		maxLifetime = 300 * time.Second
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(maxLifetime)

	// AutoMigrate creates all tables (local SQLite and production MySQL)
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	return db, nil
}

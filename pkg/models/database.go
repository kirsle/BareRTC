package models

import (
	"errors"
	"fmt"
	"time"

	"git.kirsle.net/apps/barertc/pkg/config"
	"git.kirsle.net/apps/barertc/pkg/log"
	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	DB                *gorm.DB
	ErrNotInitialized = errors.New("database is not initialized")
)

func AutoMigrate() {
	DB.AutoMigrate(
		&DirectMessage{},
	)
}

func Initialize(dbtype, connString string) error {

	var gormcfg = &gorm.Config{
		// Logger: logger.Default.LogMode(logger.Info),
	}

	switch dbtype {
	case "sqlite3":
		db, err := gorm.Open(sqlite.Open(connString), gormcfg)
		if err != nil {
			return fmt.Errorf("error opening SQLite DB: %w", err)
		}
		DB = db
	case "postgres":
		db, err := gorm.Open(postgres.Open(connString), gormcfg)
		if err != nil {
			return fmt.Errorf("error opening postgres DB: %w", err)
		}
		DB = db
	default:
		return errors.New("dbtype not supported: must be sqlite3 or postgres")
	}

	// Set connection pooling.
	if idle, open := config.Current.DirectMessageHistory.DatabaseMaxIdleConns, config.Current.DirectMessageHistory.DatabaseMaxOpenConns; idle > 0 && open > 0 {
		if sqlDB, err := DB.DB(); err != nil {
			log.Error("Error setting connection pooling: couldn't get SQL DB: %s", err)
		} else {
			log.Info("Setting DB connection pool to %d idle/%d open", idle, open)
			sqlDB.SetMaxIdleConns(idle)
			sqlDB.SetMaxOpenConns(open)
			sqlDB.SetConnMaxLifetime(time.Hour)
		}
	}

	AutoMigrate()

	// Run initializers.
	go func() {
		if err := (&DirectMessage{}).Init(); err != nil {
			log.Error("DirectMessage.Init: %s", err)
		}
	}()

	return nil
}

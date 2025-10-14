package migrations

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/uptrace/bun"
)

// MigrateOptions defines configuration options for migration
type MigrateOptions struct {
	// MigrationsDir is the directory containing migration files
	MigrationsDir string
	// AutoMigrate determines whether to run migrations automatically on startup
	AutoMigrate bool
	// SeedData determines whether to seed data (run all migrations) or just schema (skip seed data migrations)
	SeedData bool
}

// DefaultOptions returns the default migration options
func DefaultOptions() MigrateOptions {
	return MigrateOptions{
		MigrationsDir: "./migrations",
		AutoMigrate:   true,
		SeedData:      false, // By default don't seed in production
	}
}

// Runner handles database migrations
type Runner struct {
	bunDB    *bun.DB
	options  MigrateOptions
	migrator *migrate.Migrate
}

// NewRunner creates a new migration runner
func NewRunner(bunDB *bun.DB, opts MigrateOptions) *Runner {
	return &Runner{
		bunDB:   bunDB,
		options: opts,
	}
}

// Initialize prepares the migration system
func (r *Runner) Initialize() error {
	// Get the underlying sql.DB from Bun
	sqlDB := r.bunDB.DB

	// Create a postgres driver instance
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres migration driver: %w", err)
	}

	// Ensure migrations directory exists
	if _, err := os.Stat(r.options.MigrationsDir); os.IsNotExist(err) {
		return fmt.Errorf("migrations directory does not exist: %s", r.options.MigrationsDir)
	}

	// Create migration instance
	migrator, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", r.options.MigrationsDir),
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	r.migrator = migrator
	return nil
}

// RunMigrations runs all pending migrations
func (r *Runner) RunMigrations() error {
	if r.migrator == nil {
		if err := r.Initialize(); err != nil {
			return err
		}
	}

	// Run migrations
	if r.options.SeedData {
		// If seed data option is true, run all migrations
		log.Println("Running all migrations including seed data...")
		if err := r.migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("failed to run migrations: %w", err)
		}
	} else {
		// If seed data option is false, run only schema migrations (run up to but excluding any seed migrations)
		// We assume schema migrations come before seed migrations
		log.Println("Running schema migrations only...")
		version, dirty, err := r.migrator.Version()
		if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
			return fmt.Errorf("failed to get migration version: %w", err)
		}

		// If no migrations have been run yet, run the schema migration (first one)
		if errors.Is(err, migrate.ErrNilVersion) || version == 0 {
			if err := r.migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
				return fmt.Errorf("failed to run schema migration: %w", err)
			}
		} else if dirty {
			// Fix dirty migrations
			log.Println("Detected dirty migration, attempting to fix...")
			if err := r.migrator.Force(int(version)); err != nil {
				return fmt.Errorf("failed to fix dirty migration: %w", err)
			}
		}
	}

	version, _, err := r.migrator.Version()
	if err == nil {
		log.Printf("Current schema version: %d", version)
	} else if !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	return nil
}

// MigrateUp runs all pending migrations
func (r *Runner) MigrateUp() error {
	if r.migrator == nil {
		if err := r.Initialize(); err != nil {
			return err
		}
	}

	if err := r.migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration up failed: %w", err)
	}
	return nil
}

// MigrateDown rolls back all migrations
func (r *Runner) MigrateDown() error {
	if r.migrator == nil {
		if err := r.Initialize(); err != nil {
			return err
		}
	}

	if err := r.migrator.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration down failed: %w", err)
	}
	return nil
}

// MigrateTo migrates to a specific version
func (r *Runner) MigrateTo(version uint) error {
	if r.migrator == nil {
		if err := r.Initialize(); err != nil {
			return err
		}
	}

	currentVersion, _, err := r.migrator.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("failed to get current migration version: %w", err)
	}

	if errors.Is(err, migrate.ErrNilVersion) || currentVersion == 0 {
		// No migrations have been run yet
		if err := r.migrator.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migration to version %d failed: %w", version, err)
		}
	} else if currentVersion < version {
		// Migrate up to the specified version
		if err := r.migrator.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migration up to version %d failed: %w", version, err)
		}
	} else {
		// Migrate down to the specified version
		if err := r.migrator.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migration down to version %d failed: %w", version, err)
		}
	}
	return nil
}

// Close frees resources associated with the migrator
func (r *Runner) Close() error {
	if r.migrator != nil {
		sourceErr, databaseErr := r.migrator.Close()
		if sourceErr != nil {
			return fmt.Errorf("error closing migrator source: %w", sourceErr)
		}
		if databaseErr != nil {
			return fmt.Errorf("error closing migrator database: %w", databaseErr)
		}
	}
	return nil
}

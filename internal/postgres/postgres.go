// Package postgres owns the connection to PostgreSQL: applying migrations
// and handing back a ready-to-use connection pool plus the sqlc-generated
// Queries built on top of it.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect applies pending migrations and returns a connection pool together
// with sqlc Queries built on top of it. Callers must call pool.Close (via
// the returned io.Closer semantics: pool.Close()) when done.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, *sqlcgen.Queries, error) {
	if err := migrate_(databaseURL); err != nil {
		return nil, nil, fmt.Errorf("postgres: migrate: %w", err)
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("postgres: ping: %w", err)
	}

	return pool, sqlcgen.New(pool), nil
}

// migrate_ runs all pending "up" migrations using a short-lived database/sql
// connection (golang-migrate's pgx driver requires database/sql, unlike the
// pgxpool used for application queries).
func migrate_(databaseURL string) error {
	db, err := sql.Open("pgx/v5", databaseURL)
	if err != nil {
		return fmt.Errorf("open sql.DB: %w", err)
	}
	defer func() { _ = db.Close() }()

	dbDriver, err := migratepgx.WithInstance(db, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("build migrate driver: %w", err)
	}

	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("build migrate source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("build migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

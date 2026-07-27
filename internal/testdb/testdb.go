// Package testdb spins up a disposable PostgreSQL container for integration
// tests, so tests exercise the real schema and sqlc-generated queries
// instead of a mock.
package testdb

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"openrouter-with-deepswe/internal/postgres"
	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

var (
	once    sync.Once
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	initErr error
)

// New returns a pool and Queries backed by a single PostgreSQL container
// shared by every test in the process. The container is started lazily on
// first use and lives for the life of the test binary (testcontainers'
// Ryuk reaper terminates it once the test session ends), rather than one
// container per test: spinning up a fresh container per test call was
// observed to make a package's test suite take several minutes and made it
// prone to hitting the overall test timeout under load.
//
// Because the container -- and its data -- is shared across every test in
// the package, tests must use model IDs unique to themselves and assert on
// those specific IDs, not on the total row count, to stay independent of
// each other.
//
// It skips the calling test (via t.Skip) if a Docker daemon is not
// reachable.
func New(t *testing.T) (*pgxpool.Pool, *sqlcgen.Queries) {
	t.Helper()
	once.Do(func() {
		pool, queries, initErr = startContainer()
	})
	if initErr != nil {
		t.Skipf("testdb: docker unavailable, skipping integration test: %v", initErr)
	}
	return pool, queries
}

func startContainer() (*pgxpool.Pool, *sqlcgen.Queries, error) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("app"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("app"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, nil, err
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, nil, err
	}

	return postgres.Connect(ctx, connStr)
}

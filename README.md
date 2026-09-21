# openrouter-with-deepswe

A set of tools that cross-references the OpenRouter model catalog with
[DeepSWE](https://deepswe.datacurve.ai/) and
[Terminal-Bench 4.0](https://hub.harborframework.com/datasets/terminal-bench/terminal-bench/latest?leaderboard=4-0-0&tab=leaderboard)
benchmark scores, stores the result in PostgreSQL, and exposes it for querying/management over gRPC.

## Commands

### `cmd/openrouter` — model selection / price fetch batch

Fetches Text-to-Text capable models from OpenRouter's
[`/models`](https://openrouter.ai/api/v1/models), selects models that were
"registered within the last month" or are "marked as favorite", looks up
each model's [`/endpoints`](https://openrouter.ai/api/v1/models/{author}/{slug}/endpoints)
to find the provider with the cheapest weighted sum of input/output
prices, and upserts the result into the DB. The `favorite` / `hidden`
flags are never modified during upsert.

```bash
DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' go run ./cmd/openrouter
```

| Env var | Default | Description |
|---|---|---|
| `DATABASE_URL` | (required) | PostgreSQL connection string |
| `PRICE_WEIGHT_INPUT` | `3` | Weight of input price when determining the cheapest option |
| `PRICE_WEIGHT_OUTPUT` | `1` | Weight of output price when determining the cheapest option |
| `ENDPOINT_CONCURRENCY` | `4` | Number of concurrent requests to `/endpoints` |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1` | Base URL of the OpenRouter API |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | Destination for OTLP/gRPC traces |

### `cmd/grpc` — gRPC API server

Serves `ModelCatalogService` (defined in `proto/modelcatalog/v1/model_catalog.proto`).

- `SetFavorite` / `SetHidden`: explicitly set a model's favorite/hidden flag (a bool value, not a toggle)
- `ListModels`: lists models filtered by `FILTER_VISIBLE` (non-hidden, default), `FILTER_FAVORITE`
  (favorites regardless of visibility), or `FILTER_HIDDEN` (hidden only), including OpenRouter pricing
  and all DeepSWE and Terminal-Bench scores in separate `deepswe_scores` and `terminal_bench_scores` fields

```bash
DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' go run ./cmd/grpc
grpcurl -plaintext localhost:50051 list
```

| Env var | Default | Description |
|---|---|---|
| `DATABASE_URL` | (required) | PostgreSQL connection string |
| `GRPC_ADDR` | `:50051` | Listen address |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | Destination for OTLP/gRPC traces |

### `cmd/deepswe` — DeepSWE score fetch batch

For models that are not hidden but don't yet have a DeepSWE score, cross-references them against the
[DeepSWE live leaderboard](https://deepswe.datacurve.ai/artifacts/v1.1/leaderboard-live.json)
and registers scores for every matching harness/reasoning-effort combination into the DB.
Models not yet listed on the leaderboard are simply picked up again on the next run (no state tracking needed).

```bash
DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' go run ./cmd/deepswe
```

| Env var | Default | Description |
|---|---|---|
| `DATABASE_URL` | (required) | PostgreSQL connection string |
| `DEEPSWE_LEADERBOARD_URL` | `https://deepswe.datacurve.ai/artifacts/v1.1/leaderboard-live.json` | URL of the leaderboard JSON |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | Destination for OTLP/gRPC traces |

### `cmd/terminalbench` — Terminal-Bench score fetch batch

Fetches the public Terminal-Bench 4.0 leaderboard (`4-0-0`) through
[Harbor's leaderboard API](https://docs.harborframework.com/core-concepts/harbor-hub/leaderboards),
without browser automation or user credentials. Matches all visible OpenRouter models by
provider and normalized model identity, retaining every agent/reasoning-effort combination.
Unlike the DeepSWE batch, it refreshes matching scores on every run.
Unmatched models are skipped and retried on later runs.

Scores remain separate from DeepSWE: `accuracy` is a percentage (0–100), and
`accuracy_ci95_half_width` is the 95% confidence-interval half-width in percentage points.
Each score includes its leaderboard version, agent and reasoning effort; scores are not
averaged across agents or efforts. `ListModels`, `SetFavorite` and `SetHidden` return both benchmarks.
The additive database migration runs automatically at startup.

```bash
DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' go run ./cmd/terminalbench
```

| Env var | Default | Description |
|---|---|---|
| `DATABASE_URL` | (required) | PostgreSQL connection string |
| `TERMINALBENCH_LEADERBOARD_URL` | [Terminal-Bench 4.0 page](https://hub.harborframework.com/datasets/terminal-bench/terminal-bench/latest?leaderboard=4-0-0&tab=leaderboard) | Hub page URL; the `leaderboard` query selects the board, defaulting to `4-0-0`. A compatible API endpoint can be supplied for testing. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | Destination for OTLP/gRPC traces |

Run this batch after the OpenRouter import, alongside the existing DeepSWE batch.
Scheduling remains external to this repository.

## Development

### Code generation

Generated code from proto and SQL queries is committed to the repository (there is no code generation step in CI).
Regenerate it after making changes:

```bash
go tool buf generate    # proto/ -> gen/
go tool sqlc generate   # queries/ -> internal/postgres/sqlcgen/
```

### Testing

```bash
go test ./...
```

Some tests in `internal/postgres` and `internal/server` are integration tests that spin up a disposable
PostgreSQL container via [testcontainers-go](https://golang.testcontainers.org/). They are automatically
`t.Skip`ped in environments where Docker is unavailable. Tests within the same package share a single
container (starting a new one per test would be very slow).

### Manual verification

```bash
docker run -d --name orpg -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app -e POSTGRES_DB=app -p 5432:5432 postgres:17-alpine
export DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable'

go run ./cmd/openrouter
go run ./cmd/deepswe
go run ./cmd/terminalbench
go run ./cmd/grpc &

grpcurl -plaintext -d '{"filter":"FILTER_VISIBLE"}' localhost:50051 modelcatalog.v1.ModelCatalogService/ListModels
```

### Docker images

`docker/Dockerfile.base` builds all four binaries, and `docker/Dockerfile.{grpc,openrouter,deepswe,terminalbench}` each
repackage just their corresponding binary into a distroless image. To build locally, build the base image
first and pass its tag as `BASE_IMAGE`.

```bash
docker build -f docker/Dockerfile.base -t owd-base:local .
docker build -f docker/Dockerfile.grpc       --build-arg BASE_IMAGE=owd-base:local -t owd-grpc:local .
docker build -f docker/Dockerfile.openrouter --build-arg BASE_IMAGE=owd-base:local -t owd-openrouter:local .
docker build -f docker/Dockerfile.deepswe    --build-arg BASE_IMAGE=owd-base:local -t owd-deepswe:local .
docker build -f docker/Dockerfile.terminalbench --build-arg BASE_IMAGE=owd-base:local -t owd-terminalbench:local .
```

CI (`.github/workflows/build-and-push.yml`) pushes the base image once as `ghcr.io/<repo>/base`,
then packages the four binaries in parallel and pushes each as `ghcr.io/<repo>/{grpc,openrouter,deepswe,terminalbench}`.

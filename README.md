# openrouter-with-deepswe

A set of tools that cross-references the OpenRouter model catalog with
[DeepSWE](https://deepswe.datacurve.ai/) benchmark scores, stores the
result in PostgreSQL, and exposes it for querying/management over gRPC.

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
- `ListModels`: lists models filtered by `FILTER_VISIBLE` (non-hidden, default) or `FILTER_FAVORITE` (favorites only),
  including OpenRouter pricing and all DeepSWE scores

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
go run ./cmd/grpc &

grpcurl -plaintext -d '{"filter":"FILTER_VISIBLE"}' localhost:50051 modelcatalog.v1.ModelCatalogService/ListModels
```

### Docker images

`docker/Dockerfile.base` builds all three binaries, and `docker/Dockerfile.{grpc,openrouter,deepswe}` each
repackage just their corresponding binary into a distroless image. To build locally, build the base image
first and pass its tag as `BASE_IMAGE`.

```bash
docker build -f docker/Dockerfile.base -t owd-base:local .
docker build -f docker/Dockerfile.grpc       --build-arg BASE_IMAGE=owd-base:local -t owd-grpc:local .
docker build -f docker/Dockerfile.openrouter --build-arg BASE_IMAGE=owd-base:local -t owd-openrouter:local .
docker build -f docker/Dockerfile.deepswe    --build-arg BASE_IMAGE=owd-base:local -t owd-deepswe:local .
```

CI (`.github/workflows/build-and-push.yml`) pushes the base image once as `ghcr.io/<repo>/base`,
then builds the three binaries in parallel and pushes each as `ghcr.io/<repo>/{grpc,openrouter,deepswe}`.

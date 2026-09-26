# Kart Challenge — Backend

A Go HTTP API (standard library only, plus the `pgx` Postgres driver) for a
food-ordering front end, implementing [`api/openapi.yaml`](../api/openapi.yaml):
list products, fetch a product, and place orders with an optional promo code.
Promo codes are validated against three large coupon files (~313M codes) that
a separate importer preprocesses into Postgres.

> **Looking for internals?** Architecture, request and import flows,
> algorithms, the data model and design decisions are in the
> [Low-Level Design](docs/LLD.md). The change history with measurements is in
> [`CODE_LOG.md`](CODE_LOG.md). This README covers running, configuring and
> using the service.

## Contents

- [Quick start](#quick-start)
- [Prerequisites](#prerequisites)
- [Project layout](#project-layout)
- [Running](#running)
- [Configuration](#configuration)
- [API reference](#api-reference)
- [Promo codes](#promo-codes)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)

## Quick start

```sh
cd backend
docker compose up --build
```

Then, in another terminal:

```sh
curl http://localhost:8080/api/product
curl -X POST http://localhost:8080/api/order \
  -H 'Content-Type: application/json' -H 'api_key: apitest' \
  -d '{"couponCode":"HAPPYHRS","items":[{"productId":"1","quantity":2}]}'
```

Compose starts three services in order: **db** (Postgres 16) → **importer**
(loads the valid promo codes, then exits) → **api** (serves on `:8080`). The
first start imports the coupon files (about 50 s in Docker); later starts skip
the import, and the API is up in seconds.

## Prerequisites

| Requirement | Why |
|---|---|
| Docker with **at least 4 GB of memory** (Docker Desktop → Settings → Resources) | The importer holds all encoded codes in RAM; peak ≈ 2.65 GB. With less memory it is killed (exit code 137). |
| The three coupon files in `backend/data/` | `couponbase1.gz`, `couponbase2.gz`, `couponbase3.gz` (~1 GB each, git-ignored). Tiny samples with the same names are in `testdata/`. |
| Go 1.24+ | Only for running locally or running the tests. |
| Ports 8080 and 5432 free | API and Postgres. |

## Project layout

```
backend/
├── cmd/
│   ├── api/            HTTP server: config → wiring → serve → graceful shutdown
│   └── importer/       One-shot job: coupon .gz files → valid_codes table
├── internal/
│   ├── app/            Composition root (the only place dependencies are wired)
│   ├── config/         Environment loading and validation (both binaries)
│   ├── database/       pgx connection pool
│   ├── httpx/          JSON request/response helpers and the error body
│   ├── httpapi/        Router and middleware (request ID, logging, recovery, API key)
│   ├── product/        Product catalogue
│   ├── order/          Order placement
│   ├── coupon/         Promo code rules, Postgres lookup, importer pipeline
│   └── ingest/         Streaming gzip line reader used by the importer
├── migrations/         Reference SQL for the schema (the importer applies it itself)
├── testdata/           Tiny sample coupon files
├── data/               Real coupon files (git-ignored)
├── docs/LLD.md         Low-level design
├── CODE_LOG.md         Chronological change and measurement log
├── Dockerfile          Builds both binaries into one small image
└── docker-compose.yml  db → importer → api
```

## Running

### With Docker Compose (recommended)

```sh
docker compose up --build           # build and run everything in the foreground
docker compose up --build -d        # same, detached
docker compose logs -f api          # follow the API logs
docker compose stop                 # stop, keeping the database
docker compose down -v              # stop and wipe the database (forces a re-import)
```

Re-run only the import with a different validity rule (see
[Promo codes](#promo-codes)):

```sh
IMPORT_MIN_FILES=3 docker compose up importer
```

### Locally with Go (database in Docker)

```sh
docker compose up -d db
export DATABASE_URL='postgres://shop:shop@localhost:5432/shop?sslmode=disable'

DATA_DIR=./data go run ./cmd/importer     # or DATA_DIR=./testdata for the tiny samples
go run ./cmd/api
```

Running the importer natively also helps when Docker has less than 4 GB of
memory: the process can use the host's RAM, and it is faster (32 s natively
vs 50 s in Docker on the same laptop).

## Configuration

All settings come from environment variables. Both binaries load and validate
the same configuration and refuse to start (exit code 1, message on stderr)
when anything is invalid, listing every problem at once.

| Variable | Default | Used by | Meaning |
|---|---|---|---|
| `DATABASE_URL` | — (**required**) | api, importer | Postgres connection string |
| `API_KEY` | `apitest` | api | Value required in the `api_key` header for `POST /api/order`. Must be changed when `APP_ENV=production`. |
| `HTTP_ADDR` | `:8080` | api | Listen address (`host:port` or `:port`) |
| `APP_ENV` | `development` | api | `development`, `test` or `production` (production logs JSON) |
| `LOG_LEVEL` | `info` | api | `debug`, `info`, `warn` or `error` |
| `DATA_DIR` | `./data` | importer | Folder containing the three `.gz` files |
| `IMPORT_MIN_FILES` | `2` | importer | A code is valid if it appears in at least this many files (1–3) |

## API reference

Base URL `http://localhost:8080`. Request and response bodies are JSON. Every
response carries an `X-Request-ID` header (your own is echoed back if it is
well-formed), and the same ID appears in the server log line for the request.

| Method | Path | Auth | Success |
|---|---|---|---|
| `GET` | `/api/product` | — | `200`, array of products |
| `GET` | `/api/product/{productId}` | — | `200`, one product |
| `POST` | `/api/order` | `api_key` header | `200`, the placed order |
| `GET` | `/healthz` | — | `200 {"status":"ok"}` |

### Products

```json
{ "id": "1", "name": "Waffle with Berries", "price": 6.5, "category": "Waffle" }
```

- `GET /api/product` always returns an array (`[]` when empty), sorted by ID.
- `GET /api/product/{productId}` returns `400` unless `productId` is a
  positive integer, and `404` if there is no such product.

### Placing an order

```sh
curl -X POST http://localhost:8080/api/order \
  -H 'Content-Type: application/json' -H 'api_key: apitest' \
  -d '{
        "couponCode": "HAPPYHRS",
        "items": [
          { "productId": "1", "quantity": 2 },
          { "productId": "3", "quantity": 1 }
        ]
      }'
```

```json
{
  "id": "4d373a59-6da0-4f03-8c1b-8a6290d56407",
  "items": [
    { "productId": "1", "quantity": 2 },
    { "productId": "3", "quantity": 1 }
  ],
  "products": [
    { "id": "1", "name": "Waffle with Berries", "price": 6.5, "category": "Waffle" },
    { "id": "3", "name": "Macaron Mix of Five", "price": 8, "category": "Macaron" }
  ]
}
```

Request rules:

- `items` is required and must not be empty. Each item needs a `productId`
  and a `quantity` from 1 to 100.
- The same `productId` may appear more than once. Such lines are merged
  (quantities summed), and the merged total per product must not exceed 100.
- `couponCode` is optional; when present it must be a valid promo code.
- Unknown JSON fields are rejected, and the body is limited to 64 KB.
- `Content-Type` must be `application/json` (a `charset` parameter is fine).

### Errors

Every error has the same shape and uses the real HTTP status:

```json
{ "code": 422, "type": "unprocessable_entity", "message": "unknown product: \"99\"" }
```

| Status | `type` | When |
|---|---|---|
| 400 | `bad_request` | Invalid product ID; malformed or empty JSON, wrong field types, unknown fields, wrong `Content-Type`, body too large |
| 401 | `unauthorized` | `api_key` header missing or empty |
| 403 | `forbidden` | `api_key` header wrong |
| 404 | `not_found` | Product not found, or unknown route |
| 405 | `method_not_allowed` | Known path, wrong method (the `Allow` header lists the valid ones) |
| 422 | `unprocessable_entity` | Empty items, quantity out of range, merged total over 100, unknown product, invalid coupon |
| 500 | `internal_error` | Unexpected failure. Details are logged, never returned. |

## Promo codes

A code is valid when **both** hold:

1. it is 8–10 characters long, and
2. it appears in at least `IMPORT_MIN_FILES` (default 2) of the three coupon
   files.

With the default rule, the provided data yields exactly **8 valid codes**:
`BIRTHDAY`, `BUYGETON`, `FIFTYOFF`, `FREEZAAA`, `GNULINUX`, `HAPPYHRS`,
`OVER9000` and `SIXTYOFF`. Codes are case-sensitive (`happyhrs` is invalid).

The importer works out the valid set once and stores it; the API only looks
codes up. Changing `IMPORT_MIN_FILES` makes the next importer run re-import
automatically.

| Import on the full data | |
|---|---|
| Time | ~50 s in Docker, ~32 s natively |
| Peak memory | ~2.65 GB |
| Stored | 8 rows, ~24 kB (table + index) |

## Testing

```sh
go test ./...           # all unit tests
go test -race ./...     # with the race detector
go test -cover ./...    # coverage per package
go vet ./...
gofmt -l .              # should print nothing
staticcheck ./...       # if installed
```

The tests need no database or Docker: services are tested with fakes for
their interfaces, handlers with `net/http/httptest`, and the importer's
reading, encoding and merging with small in-memory gzip files. Coverage is
86–97 % for the HTTP and domain packages. The Postgres-backed code
(`coupon.Store`, the importer's database steps, `database`) is exercised by
the Docker Compose run rather than by unit tests.

A quick end-to-end check against a running stack:

```sh
curl -s -o /dev/null -w '%{http_code}\n' -X POST http://localhost:8080/api/order -H 'Content-Type: application/json' -H 'api_key: apitest' -d '{"couponCode":"HAPPYHRS","items":[{"productId":"1","quantity":1}]}'
curl -s -o /dev/null -w '%{http_code}\n' -X POST http://localhost:8080/api/order -H 'Content-Type: application/json' -H 'api_key: apitest' -d '{"couponCode":"SUPER100","items":[{"productId":"1","quantity":1}]}'
```

Expected output: `200`, then `422`.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `importer exited with code 137` | Out of memory. Give Docker at least 4 GB, or run the importer natively (see [Running](#running)). |
| `config: DATABASE_URL is required` | Set `DATABASE_URL`; Compose sets it for you. |
| `coupon file couponbase1.gz not found in DATA_DIR` | Put the files in `backend/data/`, or set `DATA_DIR=./testdata`. |
| `invalid code "…": only A-Z and 0-9 are allowed` | A coupon file contains a malformed code; the error names the file and line. |
| Importer logs `valid codes already imported, skipping` | Expected on later starts. To force a re-import, run `docker compose down -v` or change `IMPORT_MIN_FILES`. |
| `422 invalid coupon` for a code you expect to be valid | Codes are case-sensitive and must be 8–10 characters. Also check which `IMPORT_MIN_FILES` the last import used. |
| `listen on :8080: address already in use` | Another process has the port; stop it or set `HTTP_ADDR`. |
| `API_KEY must be changed from the default in production` | Set a real `API_KEY` when `APP_ENV=production`. |

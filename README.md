# Common

Shared Go packages used by the Helios services. This repository is for code that is independent of a product domain: configuration, database and Redis clients, logging, pagination, patch handling, mail, throttling, and small infrastructure helpers.

Authentication-specific code belongs in [`aegis-go`](https://github.com/heliantheon/aegis-go). Hermes gRPC contracts live in [`hermes`](https://github.com/heliantheon/hermes) under `proto/v1`.

## Install

```bash
go get github.com/heliantheon/common/pagination
```

Import the package you need directly:

```go
import "github.com/heliantheon/common/pagination"
```

## Development

```bash
make test
make lint
make tidy
```

The API query conventions implemented by `filter`, `pagination`, and `patch` are documented in [`docs/api-query-design.md`](docs/api-query-design.md).

## Package policy

- Packages must not depend on a Helios service or a product domain.
- Prefer a small package with an explicit API over a general-purpose utility bucket.
- Breaking changes require all known consumers to be migrated first.

# Common

This repository contains the domain-independent Go packages shared by Helios services.

## Boundaries

- Keep authentication and authorization SDK code in `heliantheon/aegis-go`.
- Keep protobuf contracts and generated clients in `heliantheon/proto`.
- Do not import a service repository from this module.
- Add a package only when it has at least one concrete cross-service use and no domain ownership.

## Commands

```bash
make test
make lint
make fmt
make tidy
```

## Verification

- Run `go test ./...` after every behavior change.
- Run `go mod tidy` when dependencies change and commit `go.mod` with `go.sum`.
- Update the relevant file under `docs/` when a shared API convention changes.

# Contributing

Thanks for helping improve `mlsgrid-mcp`.

## Before you open a PR

Run the local checks from the repository root:

```sh
make test
make test-integration
make verify-pin
```

- `make test` runs the unit test suite with `-race`.
- `make test-integration` runs the Docker-backed integration tests.
- `make verify-pin` checks that the vendored schema-contract migration still
  matches the upstream `mlsgrid-sync` pin.

## Refreshing the schema pin

When the vendored `mlsgrid-sync` migration changes, refresh the contract pin
deliberately:

1. Update the vendored migration files under `adapters/postgres/testdata/contract/`.
2. Update the coordinates recorded in `adapters/postgres/testdata/contract/PIN`.
3. Re-run `make test-integration`.
4. Re-run `make verify-pin`.

See [docs/RELEASING.md](docs/RELEASING.md) for the release-time checklist and
the notes about bumping the pin only when the upstream contract actually changes.

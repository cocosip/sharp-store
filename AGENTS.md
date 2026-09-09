# Repository Guidelines

## Project Structure

This is a Go 1.26 file-storage library (`github.com/cocosip/sharp-store`). The
root package owns the public abstractions and orchestration: `Factory`,
`Container`, configuration, tenant scope, keys, and backend registration.
Storage integrations are isolated in `backend/<provider>/` (for example,
`backend/filesystem` and `backend/s3`). The built-in configuration source is
`source/static`; applications own JSON, YAML, TOML, Viper, database, and remote
configuration parsing and adapt the result through `ConfigSource`. User-facing
configuration reference and examples belong in `docs/`.

Tests are colocated with their package as `*_test.go`. Keep provider-specific
tests beside that provider and root API/integration tests at the repository
root.

## Build, Test, and Development Commands

- `go test ./...` runs the complete unit and integration test suite.
- `go test ./backend/s3` runs one provider package while iterating.
- `go vet ./...` checks common Go correctness issues.
- `gofmt -w <changed-files>` formats edited Go files; run it before testing.

Avoid adding application binaries to this library. Validate changes through the
public package API and the affected backend/source package.

## Coding Style and Naming

Follow standard Go formatting and import grouping; use tabs as produced by
`gofmt`. Exported identifiers use `PascalCase`, unexported identifiers use
`camelCase`, and packages use short lowercase names. Name tests
`Test<Type>_<Behavior>` when a descriptive behavior name helps, such as
`TestContainer_SaveRejectsExistingObject`.

Keep cloud SDK dependencies confined to their respective `backend/` package.
Public interfaces should remain small and infrastructure-neutral; applications
provide persistence and cache adapters through `ConfigSource` and `ConfigCache`.

## Testing Guidelines

Add or update tests for every behavior change, including error and concurrency
paths when applicable. Prefer table-driven tests for provider configuration and
backend contracts. Do not require live cloud credentials in the default suite;
use fakes, local filesystem fixtures, or SDK test transports instead.

## Cache Directory
- The `.cache` directory is used for project build and package caching, and is already added to `.gitignore`. During testing and debugging, relevant caches can be configured under this directory. No additional cache directories should be created.

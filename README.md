# sharp-store

`sharp-store` is a Go 1.25 file-storage library. A `Factory` opens a public
`Container`; the container builds a tenant-aware object key and delegates file
operations to the configured backend. Configuration lookup and caching are
replaceable capabilities. `source/static` provides built-in in-memory
configuration, while an in-memory cache is used by default.

Supported backends are local filesystem, S3, AWS S3, MinIO, KS3, Azure Blob,
Aliyun OSS, and Huawei OBS. FastDFS is not included because this repository has
no verified Go client dependency for it.

## Install

```sh
go get github.com/cocosip/sharp-store
```

Only import the backend packages the application uses. The core package does
not import any cloud SDK or ORM package.

## API Boundaries

Provider `New` functions return `store.Backend`, and
`store.NewBackendRegistry` returns `store.BackendCatalog`. `store.NewFactory`
returns a concrete `*store.Factory`; applications that want a narrow dependency
can use the `store.ContainerFactory` interface. The factory depends only on
`store.ConfigSource`, `store.ConfigCache`, and
`store.BackendResolver`, so applications can adapt existing database, cache,
and tenant types without importing infrastructure into this library.

## Quick Start

```go
package main

import (
    "bytes"
    "context"

    store "github.com/cocosip/sharp-store"
    "github.com/cocosip/sharp-store/backend/filesystem"
    "github.com/cocosip/sharp-store/source/static"
)

func main() {
    configs := static.New(map[store.ContainerKey]store.ContainerConfig{
        "documents": store.NewContainerConfig(filesystem.Config{
            Root: "D:/data/documents",
        }),
    })
    backends := store.NewBackendRegistry(filesystem.New())
    factory, err := store.NewFactory(
        store.NewConfigOptions(configs),
        store.NewContainerOptions(backends),
    )
    if err != nil { panic(err) }

    tenant := store.DefaultTenantContext{ID: "tenant-1", Code: "acme", Name: "Acme"}
    container, err := factory.Open(context.Background(), "documents", tenant)
    if err != nil { panic(err) }
    _, err = container.Save(context.Background(), "reports/a.pdf", bytes.NewReader([]byte("data")), ".pdf", false)
    if err != nil { panic(err) }
}
```

`Save`, `Delete`, `Exists`, `Download`, `Get`, `GetOrNil`, and `AccessURL` are
all methods of `store.Container`. `Get` returns `store.ErrFileNotFound` for a
missing object; `GetOrNil` returns a nil reader instead. `Save` returns
`store.ErrFileExists` when `overwrite` is false and the object exists.

## Architecture

```text
ConfigCache.Get(name, tenant) ---- hit ----> ContainerConfig
             | miss                            |
             v                                 v
ConfigSource.Load(name, tenant) -> cache -> Factory.Open -> Container -> Backend
```

`ContainerConfig` and tenant information are snapshots. A container already
opened keeps its selected backend configuration; a new `Factory.Open` call uses
the current cached or source configuration.

## Concurrency

A `Container` is safe for concurrent use by multiple goroutines. One opened
container can perform independent `Save`, `Get`, `Download`, and other file
operations concurrently. The container snapshots its configuration and tenant
information when opened, so subsequent mutation of caller-owned data does not
affect its operations.

Callers retain ownership of mutable method inputs: do not share an `io.Reader`
or destination file path across concurrent calls unless the caller synchronizes
that access. Custom `Backend`, `NamingService`, and `KeyBuilder`
implementations registered with a factory must also be safe for concurrent use.
Operations for the same object key retain the atomicity and overwrite semantics
of the selected backend; `Container` does not serialize them.

## Configuration

Every backend exposes a typed `Config` that implements `store.BackendConfig`.
Convert it with `store.NewContainerConfig` before passing it to a source.

```go
config := store.NewContainerConfig(
    minio.NewConfig().
        WithBucket("archive").
        WithEndpoint("minio.example:9000").
        WithCredentials("access", "secret").
        WithSSL(true),
)

typed, err := minio.ParseConfig(config)
```

See [provider configuration](docs/providers.md) for every backend and
[configuration sources](docs/config-sources.md) for built-in memory
configuration and application-owned Viper, file, or database integration.

## Tenant Scope

Applications pass a `store.TenantContext` to every `Factory.Open` call.
sharp-store does not parse JWTs, request headers, or application tenant state.
The default key builder produces `{tenant-code-or-id}/{file-id}` for
`TenantScoped` configuration and `{file-id}` for `TenantShared` configuration.
`NamingService` and `KeyBuilder` remain replaceable container capabilities.

## External Integration

Configuration management remains outside sharp-store. A database-backed
application implements `ConfigSource` to load one configuration by container
name and tenant, and adapts its existing cache to `ConfigCache`. The same cache
instance is shared with the application's configuration-write service so a
successful database create/update calls `Set` and a successful delete calls
`Delete`. File-operation callers never manage configuration caches.

See [configuration sources](docs/config-sources.md) for complete external and
built-in initialization examples. Both use the same
`factory.Open(ctx, containerName, tenant)` runtime API.

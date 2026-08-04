# sharp-store

`sharp-store` is a Go 1.25 file-storage library. A `Factory` opens a public
`Container`; the container builds a tenant-aware object key and delegates file
operations to the configured backend. Configuration loading is independent of
storage: an application selects exactly one `store.ConfigSource` implementation
for a factory.

Supported backends are local filesystem, S3, AWS S3, MinIO, KS3, Azure Blob,
Aliyun OSS, and Huawei OBS. FastDFS is not included because this repository has
no verified Go client dependency for it.

## Install

```sh
go get github.com/cocosip/sharp-store
```

Only import the backend packages the application uses. The core package does
not import any cloud SDK or GORM package.

## API Boundaries

Construction functions return behavior-oriented interfaces: provider `New`
functions return `store.Backend`, `store.NewBackendRegistry` returns
`store.BackendCatalog`, and `store.NewFactory` returns `store.ContainerFactory`.
The management constructors follow the same rule (`management.Repository`,
`management.Manager`, and `store.ConfigSource`). This keeps applications
independent from a provider or repository implementation while leaving typed
provider `Config` structs as ordinary concrete Go data.

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
    factory, err := store.NewFactory(configs, backends)
    if err != nil { panic(err) }

    container, err := factory.OpenWithScope(context.Background(), "documents", store.Scope{
        Tenant: store.Tenant{ID: "tenant-1", Code: "acme"},
    })
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
ConfigSource.Load(key, scope) -> ContainerConfig -> Factory.Open -> Container
                                                              |
                                                  Backend.Save/Get/Delete/...
```

`ContainerConfig` is a snapshot. A container already opened keeps its selected
backend configuration; a new `Factory.Open` call observes a reloaded or updated
source.

## Configuration

Every backend exposes a typed `Config` that implements `store.BackendConfig`.
Convert it with `store.NewContainerConfig` before passing it to a source.

```go
config := store.NewContainerConfig(minio.Config{
    Bucket: "archive", Endpoint: "minio.example:9000",
    AccessKey: "access", SecretKey: "secret", UseSSL: true,
})
```

See [provider configuration](docs/providers.md) for every backend and
[configuration sources](docs/config-sources.md) for code, JSON, and GORM.

## Tenant Scope

The default key builder produces `{prefix}/{tenant-code-or-id}/{file-id}`. A
container with `TenantMode: store.TenantShared` clears tenant data before keys
are built. `ScopeResolver`, `NamingService`, and `KeyBuilder` are optional
factory dependencies for applications that need custom tenancy or names.

## Management

`management` provides a cache-aside `ConfigSource` backed by a small
`management.Repository` interface. `management/gorm` is optional and stores
only container configuration, never file bytes. Its schema migration belongs to
the host application. See [GORM integration](docs/config-sources.md#gorm).

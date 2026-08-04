# Configuration Sources

`Factory` accepts one active `store.ConfigSource`. The source decides where a
container configuration comes from. It can be replaced with any implementation;
the storage core does not know whether data came from code, JSON, GORM, Redis,
or a remote service.

## Code

Use `source/static` for immutable code-owned configuration. It does not perform
tenant selection; the same config is returned for every scope.

```go
configs := static.New(map[store.ContainerKey]store.ContainerConfig{
    "images": store.NewContainerConfig(filesystem.Config{Root: "D:/images"}),
})
factory, err := store.NewFactory(configs, backends)
```

## Configuration Files

`source/file` supports JSON, YAML, and TOML. `Open` loads the document
immediately; `Reload(ctx)` first parses a complete replacement snapshot, then
atomically publishes it. Backend values are always string maps.

Choose the decoder that matches the file format:

```go
configs, err := file.Open("configs/storage.yaml", file.YAMLDecoder{})
if err != nil { return err }
factory, err := store.NewFactory(configs, backends)
// Later: err = configs.Reload(ctx)
```

Use `file.JSONDecoder{}` for `.json`, `file.YAMLDecoder{}` for `.yaml` or
`.yml`, and `file.TOMLDecoder{}` for `.toml`. Each decoder returns the same
configuration model.

Complete, equivalent examples for every supported backend are available as
[`storage.example.json`](storage.example.json),
[`storage.example.yaml`](storage.example.yaml), and
[`storage.example.toml`](storage.example.toml). Replace all credential
placeholders before use; backend values, including booleans, are strings.

### JSON

```json
{
  "containers": {
    "images": {
      "backend": "minio",
      "tenantMode": 0,
      "values": {
        "bucket": "images",
        "endpoint": "minio.example:9000",
        "access_key": "access",
        "secret_key": "secret",
        "use_ssl": "true"
      }
    }
  }
}
```

JSON uses `tenantMode`.

### YAML

```yaml
containers:
  images:
    backend: minio
    tenant_mode: 0
    values:
      bucket: images
      endpoint: minio.example:9000
      access_key: access
      secret_key: secret
      use_ssl: "true"
```

### TOML

```toml
[containers.images]
backend = "minio"
tenant_mode = 0

[containers.images.values]
bucket = "images"
endpoint = "minio.example:9000"
access_key = "access"
secret_key = "secret"
use_ssl = "true"
```

YAML and TOML use `tenant_mode`. Use the provider constants (for example
`minio.BucketKey`) when generating configuration programmatically. Do not
commit real secrets.

For an unsupported file format, implement `file.Decoder`; only parsing needs
to be replaced, not the file loading, reload, or `ConfigSource` behavior.


## GORM

GORM support belongs to `management/gorm` and is optional. It persists
container configuration in one table; it never stores file content. The host
owns migration, including table naming and ordering.

`managementgorm.New` returns `management.Repository`; it intentionally does
not expose a GORM-specific repository object. Use the standalone
`managementgorm.TableName(options...)` helper together with the exported
`ContainerModel` during the host application's migration phase.

```go
options := managementgorm.Options{Table: "app_store_containers"}
if err := db.Table(managementgorm.TableName(options)).AutoMigrate(&managementgorm.ContainerModel{}); err != nil {
    return err
}
repo := managementgorm.New(db, options)

cache := management.NewMemoryCache()
configs := management.NewSource(repo, cache)
service := management.NewServiceWithValidator(repo, cache, backends)
factory, err := store.NewFactory(configs, backends)
```

Create or update rows through `service`. Validation delegates to the registered
backend before persistence. `management.Source` is cache-aside: it first looks
up `(tenant ID, container key)` in `Cache`, then calls the repository, and
caches a found record. `Service.Create`, `Update`, and `Delete` invalidate the
affected cache entry.

```go
container := management.Container{
    ID: "images-acme", TenantID: "tenant-acme", Key: "images", Title: "Images",
    Config: store.NewContainerConfig(minio.Config{
        Bucket: "acme-images", Endpoint: "minio.example:9000",
        AccessKey: "access", SecretKey: "secret", UseSSL: true,
    }),
}
if err := service.Create(ctx, container); err != nil { return err }
```

The bundled repository queries portable GORM APIs and keeps provider values as
JSON bytes. Applications using another database layer implement
`management.Repository` (for writes) or `management.Reader` (read-only source)
without importing GORM.

## Tenant Behavior

`Factory.Open(ctx, key)` obtains a scope through `FactoryOptions.Scopes`.
`OpenWithScope` is preferable when the caller already has explicit tenant data.
The bundled GORM repository performs an exact `(tenant_id, container_key)`
lookup. An empty tenant ID represents a host-level row; it does not fall back
from a tenant row to a host row. Implement that fallback in a custom `Reader`
if it is required by the application.

`TenantMode` controls object paths after configuration is loaded:

- `store.TenantScoped` (`0`, default): include tenant in the default key.
- `store.TenantShared` (`1`): remove tenant data before key generation.

`Scope.Prefix` is prepended to default keys. The default tenant segment uses
`Tenant.Code` when set, otherwise `Tenant.ID`.

## Custom Sources

Do not compose several sources into a hidden precedence chain. A factory has
one active `ConfigSource`; choose it when the application starts. Replacing
the source is the explicit switch between code configuration, a file, GORM,
Redis, a configuration center, or an HTTP service.

```go
type ConfigRecord struct {
    Backend    string
    TenantMode store.TenantMode
    Values     map[string]string
}

type ConfigClient interface {
    FindContainer(ctx context.Context, tenantID, key string) (ConfigRecord, bool, error)
}

type Source struct {
    client ConfigClient
}

func (s *Source) Load(
    ctx context.Context,
    key store.ContainerKey,
    scope store.Scope,
) (store.ContainerConfig, error) {
    record, found, err := s.client.FindContainer(ctx, scope.Tenant.ID, string(key))
    if err != nil {
        return store.ContainerConfig{}, err
    }
    if !found {
        return store.ContainerConfig{}, store.ErrContainerNotFound
    }
    return store.ContainerConfig{
        Backend: record.Backend,
        TenantMode: record.TenantMode,
        Values: record.Values,
    }, nil
}
```

```go
factory, err := store.NewFactory(&Source{client: client}, backends)
```

The source owns lookup policy. For example, a source may use an exact tenant
match, explicitly fall back to a host record, or reject host fallback. It must
return a complete `ContainerConfig`, including backend name and provider values.
It should return `store.ErrContainerNotFound` for an absent key.

For thousands of container records, do not cache them in `Factory`. Put a
bounded/cache-aside implementation inside the active source, keyed by the same
lookup identity used by the source, normally `(tenant ID, container key)`. On
configuration writes, invalidate that source cache. `management.Source` is the
built-in example of this pattern.

To load configuration from more than one external system, write one source
whose documented policy selects the authoritative result. For example, a
deployment can use a `RemoteSource` with an explicit fallback to `FileSource`
only when the remote record is absent. This remains one active source and keeps
fallback, cache invalidation, and tenant rules in one testable place.

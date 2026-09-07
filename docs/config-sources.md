# Configuration Sources

`Factory` accepts one active `store.ConfigSource`. The source decides where a
container configuration comes from. It can be replaced with any implementation;
the storage core does not know whether data came from code, JSON, a database,
Redis, or a remote service.

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


## Optional Dynamic Management

A database and the `management` package are not required. `source/static` and
`source/file` are complete `store.ConfigSource` implementations and can be
passed directly to `store.NewFactory` as shown above.

Applications that need dynamic configuration persistence can implement
`management.Reader` for read-only loading or `management.Repository` for
management writes. The adapter may use Gorm, Ent, `database/sql`, Redis, a
remote service, or another technology; `sharp-store` does not provide or
depend on those adapters.

The application supplies its repository when wiring dynamic configuration:

```go
func wireDynamicConfig(
    repository management.Repository,
    backends store.BackendCatalog,
) (store.ContainerFactory, management.Manager, error) {
    cache := management.NewMemoryCache()
    configs := management.NewSource(repository, cache)
    service := management.NewServiceWithValidator(repository, cache, backends)
    factory, err := store.NewFactory(configs, backends)
    return factory, service, err
}
```

`management.Source` is cache-aside: it first looks up `(tenant ID, container
key)` in `Cache`, then calls the repository and caches a found record.
`Service.Create`, `Update`, and `Delete` invalidate affected cache entries.
Validation delegates to the registered backend before persistence.

An application-owned adapter must follow these rules:

- `Find` matches tenant ID and container key exactly.
- `Get` matches container ID exactly.
- Missing `Find` and `Get` results return the zero container, `false`, and a
  nil error.
- Query, decoding, context, and persistence failures are returned as errors.
- The application owns models, schema migrations, table naming, transactions,
  serialization of `ContainerConfig.Values`, and data-layer error translation.

```go
func createImagesContainer(ctx context.Context, service management.Manager) error {
    container := management.Container{
        ID: "images-acme", TenantID: "tenant-acme", Key: "images", Title: "Images",
        Config: store.NewContainerConfig(minio.Config{
            Bucket: "acme-images", Endpoint: "minio.example:9000",
            AccessKey: "access", SecretKey: "secret", UseSSL: true,
        }),
    }
    return service.Create(ctx, container)
}
```

The repository stores only container configuration; file bytes continue to be
handled by the selected storage backend.

## Tenant Behavior

`Factory.Open(ctx, key)` obtains a scope through `FactoryOptions.Scopes`.
`OpenWithScope` is preferable when the caller already has explicit tenant data.
An application-owned repository must perform an exact `(tenant ID, container
key)` lookup. An empty tenant ID represents a host-level row; it does not fall
back from a tenant row to a host row. Implement that fallback in a custom
`Reader` if it is required by the application.

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

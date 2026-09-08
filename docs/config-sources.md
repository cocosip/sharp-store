# Configuration Sources and Caches

`sharp-store` resolves one `ContainerConfig` from a logical container name and
the caller-provided `TenantContext`. It does not create, update, delete, or list
configuration records.

```go
type ConfigSource interface {
    Load(
        ctx context.Context,
        key ContainerKey,
        tenant TenantContext,
    ) (ContainerConfig, error)
}
```

`ContainerKey` contains only the logical storage name, such as `images` or
`archive`. It never contains a tenant ID. The built-in cache isolates entries
internally by `(tenant.TenantID(), ContainerKey)`.

## Built-in Configuration and Cache

Use `source/static` when configuration is supplied by the program. The source
takes a defensive snapshot and returns the same named configuration for every
tenant. `NewFactory` supplies a TTL in-memory configuration cache when no custom
cache is configured.

```go
configs := static.New(map[store.ContainerKey]store.ContainerConfig{
    "images": store.NewContainerConfig(
        filesystem.NewConfig().WithRoot("D:/data/images"),
    ),
    "archive": store.NewContainerConfig(
        minio.NewConfig().
            WithBucket("archive").
            WithEndpoint("127.0.0.1:9000").
            WithCredentials("access", "secret").
            WithSSL(false),
    ),
})

factory, err := store.NewFactory(
    store.NewConfigOptions(configs),
    store.NewContainerOptions(store.NewBackendRegistry(
        filesystem.New(),
        minio.New(),
    )),
)
if err != nil {
    return err
}
```

The runtime call is always explicit about tenant information:

```go
tenant := store.DefaultTenantContext{
    ID:   "tenant-1",
    Code: "acme",
    Name: "Acme Hospital",
}

container, err := factory.Open(ctx, "images", tenant)
```

For host-level storage, pass `store.NoTenant()`.

## Existing Database, Cache, and Tenant Components

An application that already stores configuration in a database adapts its
existing query. The adapter loads only the requested `(tenant, container)`
record; it does not preload every row during factory initialization.

```go
type databaseConfigSource struct {
    repository ConfigurationRepository
}

func (s databaseConfigSource) Load(
    ctx context.Context,
    name store.ContainerKey,
    tenant store.TenantContext,
) (store.ContainerConfig, error) {
    record, found, err := s.repository.FindByTenantAndName(
        ctx,
        tenant.TenantID(),
        string(name),
    )
    if err != nil {
        return store.ContainerConfig{}, err
    }
    if !found {
        return store.ContainerConfig{}, store.ErrContainerNotFound
    }
    return store.ContainerConfig{
        Backend:    record.Provider,
        TenantMode: store.TenantMode(record.TenantMode),
        Values:     maps.Clone(record.Values),
    }, nil
}
```

The cache adapter receives the same `TenantContext`; it may serialize the
configuration into Redis, another distributed cache, or an application cache.

```go
type configCacheAdapter struct {
    cache ApplicationCache
}

func (a configCacheAdapter) Get(
    ctx context.Context,
    name store.ContainerKey,
    tenant store.TenantContext,
) (store.ContainerConfig, bool, error) {
    var config store.ContainerConfig
    found, err := a.cache.Get(ctx, cacheKey(tenant.TenantID(), name), &config)
    return config, found, err
}

func (a configCacheAdapter) Set(
    ctx context.Context,
    name store.ContainerKey,
    tenant store.TenantContext,
    config store.ContainerConfig,
) error {
    return a.cache.Set(ctx, cacheKey(tenant.TenantID(), name), config)
}

func (a configCacheAdapter) Delete(
    ctx context.Context,
    name store.ContainerKey,
    tenant store.TenantContext,
) error {
    return a.cache.Delete(ctx, cacheKey(tenant.TenantID(), name))
}
```

The application's tenant model is also adapted without exposing its structure
to sharp-store:

```go
type tenantAdapter struct {
    tenant *tenantservice.CurrentTenant
}

func (a tenantAdapter) TenantID() string   { return a.tenant.ID }
func (a tenantAdapter) TenantCode() string { return a.tenant.Code }
func (a tenantAdapter) TenantName() string { return a.tenant.Name }
```

Initialization supplies the adapters:

```go
sharedConfigCache := configCacheAdapter{cache: applicationCache}

factory, err := store.NewFactory(
    store.NewConfigOptions(databaseConfigSource{repository: repository}).
        WithCache(sharedConfigCache),
    store.NewContainerOptions(store.NewBackendRegistry(
        filesystem.New(),
        minio.New(),
        s3.New(),
    )),
)
if err != nil {
    return err
}
```

The runtime call is identical to built-in initialization:

```go
tenant := tenantAdapter{tenant: currentTenant}
container, err := factory.Open(ctx, "images", tenant)
```

The application's configuration-write service owns consistency because it
knows when its database transaction commits. It shares the same cache adapter:

```go
// After a successful create or update:
err := sharedConfigCache.Set(ctx, name, tenant, updatedConfig)

// After a successful delete:
err := sharedConfigCache.Delete(ctx, name, tenant)
```

File-operation callers do not update or invalidate configuration caches.

### Conditional Cache Filling

Automatic cache filling requires the optional `store.VersionedConfigCache`
interface. The built-in memory cache implements it. Custom caches that only
implement `ConfigCache` remain supported: the factory reads application-written
entries and loads misses from `ConfigSource`, but does not write those results
back to the cache.

```go
type VersionedConfigCache interface {
    ConfigCache
    GetWithVersion(ctx context.Context, key ContainerKey, tenant TenantContext) (ContainerConfig, bool, string, error)
    SetIfVersion(ctx context.Context, key ContainerKey, tenant TenantContext, config ContainerConfig, version string) (bool, error)
}
```

`GetWithVersion` reads the configuration, hit flag, and opaque version atomically,
including on a cache miss. `SetIfVersion` atomically compares that version and
writes only if it is still current; a conflict returns `(false, nil)`. Every
successful `Set` and `Delete` must invalidate older versions, even when deleting
an absent entry. Versions must not be reused while an earlier load can still
complete. Remote cache adapters must implement this comparison in the shared
cache service, for example with a Redis script or transaction, rather than a
process-local lock. A separate `Get` followed by `Set` is not sufficient.

The memory cache uses a cache-wide mutation version to avoid retaining per-key
deletion tombstones. An unrelated mutation may therefore skip one cache fill;
the next miss can populate it normally. A custom adapter may instead use
per-key versions with appropriate tombstone lifetime management.

An already-running `Open` can return the configuration snapshot it loaded before
a concurrent update or deletion. That snapshot will not overwrite the updated
cache or resurrect a deleted cache entry. Subsequent opens see the application
update, or reload from the source after deletion.

## Application-Owned Parsing

sharp-store does not parse JSON, YAML, TOML, Viper values, environment
variables, or remote configuration documents. The application owns parsing and
maps the result to `ContainerConfig`.

A Viper-based application can implement `ConfigSource` directly:

```go
type viperConfigSource struct {
    config *viper.Viper
}

type viperContainerConfig struct {
    Backend    string            `mapstructure:"backend"`
    TenantMode store.TenantMode  `mapstructure:"tenant_mode"`
    Values     map[string]string `mapstructure:"values"`
}

func (s viperConfigSource) Load(
    ctx context.Context,
    name store.ContainerKey,
    tenant store.TenantContext,
) (store.ContainerConfig, error) {
    if err := ctx.Err(); err != nil {
        return store.ContainerConfig{}, err
    }

    path := "storage.containers." + string(name)
    if !s.config.IsSet(path) {
        return store.ContainerConfig{}, store.ErrContainerNotFound
    }

    var value viperContainerConfig
    if err := s.config.UnmarshalKey(path, &value); err != nil {
        return store.ContainerConfig{}, err
    }
    return store.ContainerConfig{
        Backend:    value.Backend,
        TenantMode: value.TenantMode,
        Values:     maps.Clone(value.Values),
    }, nil
}
```

The application decides how tenant information affects the Viper path. The
example uses one shared configuration per container and therefore does not use
`tenant`.

For JSON, decode the document in application code and pass the result to the
built-in memory source:

```go
var document struct {
    Containers map[store.ContainerKey]store.ContainerConfig `json:"containers"`
}
if err := json.NewDecoder(reader).Decode(&document); err != nil {
    return err
}

configs := static.New(document.Containers)
factory, err := store.NewFactory(
    store.NewConfigOptions(configs),
    store.NewContainerOptions(backends),
)
```

YAML, TOML, configuration centers, and environment-based configuration follow
the same boundary: parsing stays in the application, and sharp-store receives a
`ConfigSource`. When external configuration changes, the application updates or
deletes the corresponding shared `ConfigCache` entry after publishing its new
configuration.

## Generic and Provider Configuration

Sources and caches always exchange the generic `ContainerConfig`:

```go
type ContainerConfig struct {
    Backend    string
    TenantMode TenantMode
    Values     map[string]string
}
```

Typed provider configuration converts in both directions:

```go
generic := store.NewContainerConfig(
    minio.NewConfig().
        WithBucket("images").
        WithEndpoint("minio.example:9000").
        WithCredentials("access", "secret").
        WithSSL(true),
).WithTenantMode(store.TenantScoped)

typed, err := minio.ParseConfig(generic)
```

Provider values stay strings at the generic boundary. Each provider's
`ParseConfig` validates its backend name and converts booleans or other typed
values.

## Tenant Isolation

`TenantContext` contains only the tenant information sharp-store consumes:

```go
type TenantContext interface {
    TenantID() string
    TenantCode() string
    TenantName() string
}
```

- `TenantID` isolates configuration cache entries and is available to custom sources.
- `TenantCode`, falling back to `TenantID`, is used by the default object key builder.
- `TenantName` is contextual information available to adapters; it is not part of cache identity.
- `TenantScoped` includes the tenant segment in the default object key.
- `TenantShared` omits the tenant segment from the object key.

sharp-store snapshots these three strings during `Open`. It never resolves a
tenant from JWTs, HTTP headers, RPC metadata, or global application state.

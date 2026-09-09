# Storage Backends

`sharp-store` exposes one `store.Container` API for every backend. The
application chooses a backend when it builds a `ContainerConfig`, registers
that backend with the factory, and then uses `Save`, `Get`, `Download`,
`Exists`, `Delete`, and `AccessURL` through the container. File bytes are never
stored by a configuration source.

Register only the providers used by the application. This keeps unused cloud
SDK dependencies and credentials out of the running process.

Each provider's `New()` returns `store.Backend`. Configuration metadata and
validation are catalog operations, so retain the result of
`store.NewBackendRegistry` as `store.BackendCatalog` when the application
needs `List` or `ValidateConfig`.

```go
backends := store.NewBackendRegistry(
    filesystem.New(), s3.New(), minio.New(), azure.New(),
)
```

All providers implement save, delete, exists, download, get/get-or-nil, and
access URL unless noted below. `AccessURL` checks existence only when
`AccessURLOptions.CheckExists` is true.

## Choose a Backend

| Backend name | Use it when | Credentials | `AccessURL` behavior |
| --- | --- | --- | --- |
| `filesystem` | Files are on a local disk or mounted volume. | Filesystem permissions | Requires `base_url`; returns a normal URL. |
| `s3` | The target implements the S3 API, including private endpoints. | Static access and secret keys | Generates a signed S3 GET URL. |
| `aws` | The target is AWS S3 and the deployment should identify it explicitly. | Static keys or the AWS SDK default credential chain | Generates a signed S3 GET URL. |
| `minio` | The target is MinIO. | Access key and secret key | Generates a signed S3-compatible GET URL. |
| `ks3` | The target is Kingsoft Cloud KS3. | Access key and secret key | Generates a signed S3-compatible GET URL. |
| `azure` | The target is Azure Blob Storage. | Azure Storage connection string | Unsupported; serve or sign URLs in the host application. |
| `aliyun` | The target is Alibaba Cloud OSS. | Access key ID and access key secret | Generates an OSS signed GET URL. |
| `obs` | The target is Huawei Cloud OBS. | Access key ID and access key secret | Generates an OBS signed GET URL. |

All signed-URL backends accept `AccessURLOptions.ExpiresAt`. Alibaba OSS and
Huawei OBS use one hour when no expiry is supplied. Set `CheckExists: true`
when generating a URL must fail for a missing object; it otherwise avoids a
separate existence request.

## Common Integration

Use a typed provider configuration for code-owned configuration. Its
`BackendName` and string values are converted by `store.NewContainerConfig`;
do not hand-write different field names for this path.

```go
package main

import (
    "bytes"
    "context"

    store "github.com/cocosip/sharp-store"
    "github.com/cocosip/sharp-store/backend/filesystem"
    "github.com/cocosip/sharp-store/source/static"
)

func saveReport(ctx context.Context) error {
    configs := static.New(map[store.ContainerKey]store.ContainerConfig{
        "reports": store.NewContainerConfig(
            filesystem.NewConfig().WithRoot("D:/store/reports"),
        ),
    })
    backends := store.NewBackendRegistry(filesystem.New())
    factory, err := store.NewFactory(
        store.NewConfigOptions(configs),
        store.NewContainerOptions(backends),
    )
    if err != nil {
        return err
    }

    tenant := store.DefaultTenantContext{ID: "tenant-42", Code: "acme", Name: "Acme"}
    reports, err := factory.Open(ctx, "reports", tenant)
    if err != nil {
        return err
    }
    _, err = reports.Save(ctx, "2026/summary.pdf", bytes.NewReader([]byte("data")), false)
    return err
}
```

With the default key builder, that example stores the object with key
`acme/2026/summary`. Set `TenantMode: store.TenantShared` on the resulting
`ContainerConfig` when objects must not be partitioned by tenant. See
[configuration sources](config-sources.md) for built-in memory configuration
and application-owned Viper, file, or database adapters.

To enable more than one storage type, import and register each one, then select
the provider per container configuration:

```go
backends := store.NewBackendRegistry(
    filesystem.New(), s3.New(), aws.New(), minio.New(), ks3.New(),
    azure.New(), aliyun.New(), obs.New(),
)
```

The `Backend` value returned by a `ConfigSource` must match the name in the
table. All generic `ContainerConfig.Values` entries are strings, including
booleans such as `"true"`.

## Filesystem

```go
filesystem.Config{Root: "D:/store", BaseURL: "https://files.example.test"}
```

`Root` is required. `BaseURL` is optional; without it `AccessURL` returns
`store.ErrUnsupported`. Object keys are validated to prevent root traversal.

Parent-directory segments (`..`) are rejected before path normalization,
including backslash-separated paths. The default key builder also rejects dot
segments in file IDs and separators in tenant identifiers so an object cannot
select a neighboring tenant directory. Custom key builders own their tenant
layout; filesystem keys still cannot contain parent-directory segments.

Saves write a temporary file in the destination directory and publish it only
after writing, syncing, and closing succeed. Failed saves leave the existing
object intact and remove their temporary file. Exclusive saves use hard-link
creation to publish without replacing an existing object; the filesystem must
support hard links. Overwrite saves use the filesystem's rename operation and
preserve existing file permission bits. Atomicity of rename depends on the
operating system and mounted filesystem.

Use `filesystem` for a local disk, Docker volume, or mounted network file
system. `root` is the only directory the backend can write to. `BaseURL` does
not start an HTTP server or expose files by itself: configure the reverse proxy
or application server to map that URL to `Root`. The returned URL is a regular
public URL, not a signed URL.

## Generic S3

```go
s3.Config{
    Bucket: "archive",
    BaseEndpoint: "https://s3.example.test",
    AccessKeyID: "access",
    SecretAccessKey: "secret",
    ForcePathStyle: true,
    UseChunkEncoding: false,
    CreateBucketIfNotExists: true,
}
```

Use `s3` for a service that implements the generic S3 protocol and has no
dedicated backend. `bucket`, `base_endpoint`, `access_key_id`, and
`secret_access_key` are required. `base_endpoint` maps to the AWS Go SDK
`s3.Options.BaseEndpoint` field and must be a complete HTTP(S) URL. This
provider deliberately has no `region`, `authentication_region`, or
`session_token` setting; it signs compatible requests with `us-east-1` and
static credentials.

Set `force_path_style` when the service expects
`https://host/bucket/key`. `use_chunk_encoding` allows an unknown-length
`io.Reader` to stream with chunked transfer encoding. When it is false, the
backend determines the length directly or buffers a non-seekable reader in a
temporary file before calling the SDK. `protocol` is optional and only
overrides the scheme of generated access URLs; it defaults to the scheme in
`base_endpoint`. `create_bucket_if_not_exists` checks and creates the bucket
before saving.

## AWS S3

```go
aws.Config{
    Bucket: "archive",
    Region: "ap-southeast-1",
    CreateContainerIfNotExists: true,
}
```

Use `aws` only for Amazon S3. `bucket` is required and `region` defaults to
`us-east-1`. The backend uses the official AWS SDK endpoint resolver and
default credential chain, so it does not expose a custom endpoint or path-style
switch. For IAM roles, workload identity, shared AWS configuration, or
environment credentials, omit `access_key_id`, `secret_access_key`, and
`session_token`. Static access and secret keys must be supplied together; a
session token is accepted only with those keys. `create_container_if_not_exists`
creates the S3 bucket before saving when necessary.

## MinIO

```go
minio.Config{
    Bucket: "archive", Endpoint: "minio.example:9000",
    AccessKey: "access", SecretKey: "secret", Region: "cn-east-1", SSL: true,
    CreateBucketIfNotExists: true,
}
```

`Bucket`, `Endpoint`, `AccessKey`, and `SecretKey` are required. `Endpoint` is
a domain name or IP address with an optional port and must not include
`http://`, `https://`, a path, or query parameters. `SSL: true` (`with_ssl` in
generic values) uses HTTPS; false uses HTTP. `Region` maps to the MinIO Go SDK
signing region and defaults to `us-east-1`. The backend uses path-style bucket
lookup. `create_bucket_if_not_exists` controls the pre-save bucket check and
creation.

## KS3

```go
ks3.Config{
    Bucket: "archive", Endpoint: "ks3-cn-beijing.ksyuncs.com",
    AccessKey: "access", SecretKey: "secret", Protocol: "https",
    UserAgent: "sharp-store", MaxConnections: 30, Timeout: 100000,
    CreateContainerIfNotExists: true,
}
```

`Bucket`, `Endpoint`, `AccessKey`, and `SecretKey` are required. `Endpoint` is
a domain name or IP address with an optional port and must not include
`http://`, `https://`, a path, or query parameters. `Protocol` must be `http`
or `https` and defaults to `http`.

This backend uses the KS3 SDK with its V2 signer and KS3 overwrite-prevention
header; it does not route through the AWS S3 backend. `user_agent` is optional.
`max_connections` limits connections per endpoint, and `timeout` is the whole
request timeout in milliseconds; zero leaves the Go HTTP defaults in effect.
The SDK requires a region value internally, but it is fixed by the backend and
is not a user setting. `read_write_timeout` is not exposed because the Go SDK
does not provide an equivalent with the same semantics.

## Azure Blob

```go
azure.Config{
    ConnectionString: "DefaultEndpointsProtocol=https;AccountName=...;AccountKey=...;EndpointSuffix=core.windows.net",
    Container: "archive",
    CreateContainerIfNotExists: true,
}
```

`ConnectionString` and `Container` are required. The current implementation
supports object operations through the Azure Blob SDK. It does not generate
access URLs, so `AccessURL` returns `store.ErrUnsupported`.

The backend currently reads the complete `Save` body into memory before
uploading. Applications storing large objects should account for that memory
requirement. Generate Azure SAS URLs or expose downloads through the host
application when clients need direct access. Set
`create_container_if_not_exists` to check and create the blob container before
saving.

## Aliyun OSS

```go
aliyun.Config{
    Endpoint: "https://oss-cn-hangzhou.aliyuncs.com", Bucket: "archive",
    AccessKeyID: "access", AccessKeySecret: "secret",
    CreateContainerIfNotExists: true,
}
```

All four fields are required. `AccessURL` returns an OSS signed GET URL; its
default lifetime is one hour unless `AccessURLOptions.ExpiresAt` is supplied.

Use `aliyun` for Alibaba Cloud OSS. Keep `AccessKeyID` and `AccessKeySecret`
outside committed configuration files. Use `AccessURLOptions.ExpiresAt` when
the default one-hour signed URL lifetime is not suitable. `Endpoint` is named
after the `endpoint` argument accepted by `oss.New` and may include its HTTP(S)
scheme. `create_container_if_not_exists` checks and creates the OSS bucket
before saving.

## Huawei OBS

```go
obs.Config{
    Endpoint: "https://obs.cn-north-4.myhuaweicloud.com", Bucket: "archive",
    AccessKeyID: "access", AccessKeySecret: "secret",
    CreateContainerIfNotExists: true,
}
```

All four fields are required. `AccessURL` returns an OBS signed GET URL; its
default lifetime is one hour unless `AccessURLOptions.ExpiresAt` is supplied.

Use `obs` for Huawei Cloud OBS. Keep `AccessKeyID` and `AccessKeySecret` out
of committed configuration files. Use `AccessURLOptions.ExpiresAt` when the
default one-hour signed URL lifetime is not suitable. `Endpoint` is named after
the `endpoint` argument accepted by `obs.New` and may include its HTTP(S)
scheme. `create_container_if_not_exists` checks and creates the OBS bucket
before saving.

## Validation and Secret Handling

Before saving configuration from a management UI or another dynamic source,
validate against the registered backend:

```go
if err := backends.ValidateConfig(ctx, "minio", values); err != nil {
    return err
}
```

`store.NewBackendRegistry` returns `store.BackendCatalog`, which provides this
validation and `List()` metadata for management UIs. Treat all access keys,
secrets, session tokens, and Azure connection strings as secrets. Supply them
through a secret manager, deployment environment, or protected configuration
store; the examples use placeholders and must not be used with production
credentials.

## FastDFS

FastDFS is intentionally unavailable. Do not configure `Backend: "fastdfs"`;
the registry returns `store.ErrBackendNotFound`.

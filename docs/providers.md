# Storage Providers

Register only the providers used by the application. `BackendRegistry.List()`
returns the same field metadata for management UIs, while typed `Config`
values are the preferred code configuration API.

```go
backends := store.NewBackendRegistry(
    filesystem.New(), s3.New(), minio.New(), azure.New(),
)
```

All providers implement save, delete, exists, download, get/get-or-nil, and
access URL unless noted below. `AccessURL` checks existence only when
`AccessURLOptions.CheckExists` is true.

## Filesystem

```go
filesystem.Config{Root: "D:/store", BaseURL: "https://files.example.test"}
```

`Root` is required. `BaseURL` is optional; without it `AccessURL` returns
`store.ErrUnsupported`. Object keys are validated to prevent root traversal.

## Generic S3

```go
s3.Config{
    Bucket: "archive", Region: "us-east-1", Endpoint: "https://s3.example.test",
    AccessKeyID: "access", SecretAccessKey: "secret", SessionToken: "",
    PathStyle: true,
}
```

`Bucket` is required; region defaults to `us-east-1`. Omit static credentials
to use the AWS SDK default credential chain. `Endpoint` is optional. For a
custom HTTP endpoint the backend uses unsigned payload signing so streaming
`io.Reader` bodies work; normal AWS HTTPS signing remains unchanged.

## AWS S3

```go
aws.Config{Bucket: "archive", Region: "ap-southeast-1"}
```

`aws.Config` has the same fields as `s3.Config`: `Bucket`, `Region`,
`Endpoint`, `AccessKeyID`, `SecretAccessKey`, `SessionToken`, and `PathStyle`.
Use `Endpoint` for LocalStack or a private partition; use the SDK default
credential chain when access keys are omitted.

## MinIO

```go
minio.Config{
    Bucket: "archive", Endpoint: "minio.example:9000",
    AccessKey: "access", SecretKey: "secret", UseSSL: true, Region: "us-east-1",
}
```

`Bucket`, `Endpoint`, `AccessKey`, and `SecretKey` are required. Endpoint may
include a scheme; otherwise `UseSSL` selects `https` or `http`. Path-style
addressing is always used.

## KS3

```go
ks3.Config{
    Bucket: "archive", Endpoint: "ks3-cn-beijing.ksyuncs.com",
    AccessKey: "access", SecretKey: "secret", Protocol: "https", Region: "us-east-1",
}
```

`Bucket`, `Endpoint`, `AccessKey`, and `SecretKey` are required. `Protocol`
is used only for endpoints without a scheme and must be `http` or `https`.

## Azure Blob

```go
azure.Config{
    ConnectionString: "DefaultEndpointsProtocol=https;AccountName=...;AccountKey=...;EndpointSuffix=core.windows.net",
    Container: "archive",
}
```

Both fields are required. The current implementation supports object operations
through the Azure Blob SDK. It does not generate access URLs, so `AccessURL`
returns `store.ErrUnsupported`.

## Aliyun OSS

```go
aliyun.Config{
    Endpoint: "https://oss-cn-hangzhou.aliyuncs.com", Bucket: "archive",
    AccessKeyID: "access", AccessKeySecret: "secret",
}
```

All four fields are required. `AccessURL` returns an OSS signed GET URL; its
default lifetime is one hour unless `AccessURLOptions.ExpiresAt` is supplied.

## Huawei OBS

```go
obs.Config{
    Endpoint: "https://obs.cn-north-4.myhuaweicloud.com", Bucket: "archive",
    AccessKeyID: "access", AccessKeySecret: "secret",
}
```

All four fields are required. `AccessURL` returns an OBS signed GET URL; its
default lifetime is one hour unless `AccessURLOptions.ExpiresAt` is supplied.

## FastDFS

FastDFS is intentionally unavailable. Do not configure `Backend: "fastdfs"`;
the registry returns `store.ErrBackendNotFound`.

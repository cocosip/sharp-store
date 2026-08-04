package minio

import "strconv"

type Config struct {
	Bucket, Endpoint, AccessKey, SecretKey, Region string
	UseSSL                                         bool
}

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{BucketKey: c.Bucket, EndpointKey: c.Endpoint, AccessKeyKey: c.AccessKey, SecretKeyKey: c.SecretKey, RegionKey: c.Region, UseSSLKey: strconv.FormatBool(c.UseSSL)}
}

package ks3

type Config struct{ Bucket, Endpoint, AccessKey, SecretKey, Protocol, Region string }

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{BucketKey: c.Bucket, EndpointKey: c.Endpoint, AccessKeyKey: c.AccessKey, SecretKeyKey: c.SecretKey, ProtocolKey: c.Protocol, RegionKey: c.Region}
}

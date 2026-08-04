package aws

import "strconv"

type Config struct {
	Bucket, Region, Endpoint, AccessKeyID, SecretAccessKey, SessionToken string
	PathStyle                                                            bool
}

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{BucketKey: c.Bucket, RegionKey: c.Region, EndpointKey: c.Endpoint, AccessKeyIDKey: c.AccessKeyID, SecretAccessKeyKey: c.SecretAccessKey, SessionTokenKey: c.SessionToken, PathStyleKey: strconv.FormatBool(c.PathStyle)}
}

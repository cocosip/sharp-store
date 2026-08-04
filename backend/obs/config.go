package obs

type Config struct{ Endpoint, Bucket, AccessKeyID, AccessKeySecret string }

func (Config) BackendName() string { return Name }
func (c Config) Values() map[string]string {
	return map[string]string{EndpointKey: c.Endpoint, BucketKey: c.Bucket, AccessKeyIDKey: c.AccessKeyID, AccessKeySecretKey: c.AccessKeySecret}
}

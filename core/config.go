package core

// Config is a bucket configuration. v1 wraps a single Bandwidth; the wrapper is
// the seam that lets multi-bandwidth be added later without breaking the public
// API (NewLocalBucket / ProxyManager.Bucket take a Config).
type Config struct {
	bandwidth Bandwidth
}

// NewConfig builds a configuration from one bandwidth.
func NewConfig(b Bandwidth) Config {
	return Config{bandwidth: b}
}

// Bandwidth returns the configured bandwidth.
func (c Config) Bandwidth() Bandwidth { return c.bandwidth }

// valid reports whether the configuration is usable.
func (c Config) valid() bool { return c.bandwidth.valid() }

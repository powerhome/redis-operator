package v1

const (
	defaultRedisNumber             = 3
	defaultSentinelNumber          = 3
	defaultHaproxyNumber           = 2
	defaultRedisPort               = 6379
	DefaultHaproxyExporterPort     = 8405
	DefaultHaproxyExporterPortName = "metrics"
)

// The images a failover runs when it names none of its own. Each one names a tag
// and the digest that tag resolves to, so the operator version decides the bytes
// that run. A registry that repoints a tag cannot change what this release runs.
const (
	defaultSentinelExporterImage = "leominov/redis_sentinel_exporter:1.7.1@sha256:812e617d2201d0ee8ba51d60b9fe9590a46bed84abc95746f554e1ca04e5478a"
	defaultExporterImage         = "quay.io/oliver006/redis_exporter:v1.57.0@sha256:9571c54505365f7323bf33d9cb3ddbaa9f480903a024449ade73538c48b130d9"
	defaultImage                 = "redis:7.2.4-alpine@sha256:c8bb255c3559b3e458766db810aa7b3c7af1235b204cfdb304e79ff388fe1a5a"
	defaultHAProxyImage          = "haproxy:3.1.0@sha256:d33ab89215eff68d3fe238e4fbed00a7c33cc3aec20b1079a3c1250394a9c8da"
)

var (
	defaultSentinelCustomConfig = []string{
		"down-after-milliseconds 5000",
		"failover-timeout 10000",
		"parallel-syncs 1",
	}
	defaultRedisCustomConfig = []string{
		"replica-priority 100",
	}
	bootstrappingRedisCustomConfig = []string{
		"replica-priority 0",
	}
)

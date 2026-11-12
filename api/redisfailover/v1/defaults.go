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
	defaultExporterImage         = "quay.io/oliver006/redis_exporter:v1.97.0@sha256:f5f903fbe77571d455ede0453b084d6840bc62fc105efac001034421cab83b99"
	defaultImage                 = "redis:7.2.4-alpine@sha256:c8bb255c3559b3e458766db810aa7b3c7af1235b204cfdb304e79ff388fe1a5a"
	defaultHAProxyImage          = "haproxy:3.4.7@sha256:f6d2ee87b47192d9cc8c22e88fef405122ff339859644883edc2a7d1827f87b3"
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

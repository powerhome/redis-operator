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
	defaultExporterImage         = "quay.io/oliver006/redis_exporter:v1.99.0@sha256:2c0cdc2452096b5af460d870e72fa6ee2ebb971780f443a616885685d2c0ebad"
	defaultImage                 = "redis:7.2.17-alpine@sha256:2f66de68111e64c3927859f158f8e666066dae2b97d9f82770a6f37a5aa1c335"
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

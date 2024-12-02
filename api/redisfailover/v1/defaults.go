package v1

const (
	defaultRedisNumber           = 3
	defaultSentinelNumber        = 3
	defaultSentinelExporterImage = "leominov/redis_sentinel_exporter:1.7.1"
	defaultExporterImage         = "quay.io/oliver006/redis_exporter:v1.57.0"
	defaultImage                 = "redis:7.2.4-alpine"
	defaultHAProxyImage          = "haproxy:3.1.0"
	defaultRedisPort             = 6379
)

var (
	defaultSentinelCustomConfig = []string{
		"down-after-milliseconds 5000",
		"failover-timeout 10000",
	}
	defaultRedisCustomConfig = []string{
		"replica-priority 100",
	}
	bootstrappingRedisCustomConfig = []string{
		"replica-priority 0",
	}
)

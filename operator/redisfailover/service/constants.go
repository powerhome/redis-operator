package service

// variables refering to the redis exporter port
const (
	exporterPort                  = 9121
	sentinelExporterPort          = 9355
	exporterPortName              = "http-metrics"
	exporterContainerName         = "redis-exporter"
	sentinelExporterContainerName = "sentinel-exporter"
	exporterDefaultRequestCPU     = "10m"
	exporterDefaultLimitCPU       = "1000m"
	exporterDefaultRequestMemory  = "50Mi"
	exporterDefaultLimitMemory    = "100Mi"
)

const (
	baseName                    = "rf"
	sentinelName                = "s"
	sentinelRoleName            = "sentinel"
	sentinelConfigFileName      = "sentinel.conf"
	sentinelNetworkPolicyName   = "s-np"
	redisConfigFileName         = "redis.conf"
	redisName                   = "r"
	redisNetworkPolicyName      = "r-np"
	redisMasterName             = "rm"
	redisSlaveName              = "rs"
	redisShutdownName           = "r-s"
	redisReadinessName          = "r-readiness"
	redisRoleName               = "redis"
	appLabel                    = "redis-failover"
	hostnameTopologyKey         = "kubernetes.io/hostname"
	redisHAProxySlaveRedisName  = "rs-haproxy"
	redisHAProxyMasterRedisName = "rm-haproxy"
)

const (
	redisRoleLabelKey    = "redisfailovers-role"
	redisRoleLabelMaster = "master"
	redisRoleLabelSlave  = "slave"
	redisHARoleLabelKey  = "redishaproxy-role"
)

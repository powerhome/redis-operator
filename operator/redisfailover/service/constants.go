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
)

const (
	haproxyConfigChecksumAnnotationKey = "checksum/haproxy-cfg"
	haproxyDeploymentSpecChecksumKey   = "checksum/haproxy-deployment-spec"
	sentinelDeploymentSpecChecksumKey  = "checksum/sentinel-deployment-spec"
	redisStatefulSetSpecChecksumKey    = "checksum/redis-statefulset-spec"
	// Changes when the failover's password does, which the pod spec
	// otherwise never reflects: the secret is mounted by reference, so
	// rotating its value leaves the template identical and gives the
	// rolling update nothing to act on.
	redisPasswordChecksumKey = "checksum/redis-password"
)

package service

import "github.com/spotahome/redis-operator/service/redis"

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
	// noMasterYet is defined by the package that reads it off an instance.
	noMasterYet = redis.NoMasterYet

	// ownPod is the address a sidecar reaches its sibling container on. The same
	// address as noMasterYet, meaning something else entirely.
	ownPod = "127.0.0.1"
)

const (
	baseName                  = "rf"
	sentinelName              = "s"
	sentinelRoleName          = "sentinel"
	sentinelConfigFileName    = "sentinel.conf"
	sentinelNetworkPolicyName = "s-np"
	redisConfigFileName       = "redis.conf"
	redisName                 = "r"
	redisNetworkPolicyName    = "r-np"
	redisMasterName           = "rm"
	redisSlaveName            = "rs"
	redisShutdownName         = "r-s"
	redisReadinessName        = "r-readiness"
	redisRoleName             = "redis"
	// redisPodNameEnvVar carries a pod's own name into its Redis command, where
	// Kubernetes substitutes it before Redis reads it. Declared on the container
	// and referenced by announceOwnName, which is the only reason it exists.
	redisPodNameEnvVar          = "REDIS_POD_NAME"
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

package service

import (
	"fmt"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
)

// GetRedisShutdownConfigMapName returns the name for redis configmap
func GetRedisShutdownConfigMapName(rf *redisfailoverv1.RedisFailover) string {
	if rf.Spec.Redis.ShutdownConfigMap != "" {
		return rf.Spec.Redis.ShutdownConfigMap
	}
	return GetRedisShutdownName(rf)
}

// GetRedisName returns the name for redis resources
func GetRedisName(rf *redisfailoverv1.RedisFailover) string {
	return generateName(redisName, rf.Name)
}

// GetRedisShutdownName returns the name for redis resources
func GetRedisShutdownName(rf *redisfailoverv1.RedisFailover) string {
	return generateName(redisShutdownName, rf.Name)
}

// GetRedisReadinessName returns the name for redis resources
func GetRedisReadinessName(rf *redisfailoverv1.RedisFailover) string {
	return generateName(redisReadinessName, rf.Name)
}

// GetSentinelName returns the name for sentinel resources
func GetSentinelName(rf *redisfailoverv1.RedisFailover) string {
	return generateName(sentinelName, rf.Name)
}

// GetRedisNetworkPolicyName returns the name for the redis network policy
func GetRedisNetworkPolicyName(rf *redisfailoverv1.RedisFailover) string {
	return generateName(redisNetworkPolicyName, rf.Name)
}

// GetSentinelNetworkPolicyName returns the name for the sentinel network policy
func GetSentinelNetworkPolicyName(rf *redisfailoverv1.RedisFailover) string {
	return generateName(sentinelNetworkPolicyName, rf.Name)
}

func GetRedisMasterName(rf *redisfailoverv1.RedisFailover) string {
	return generateName(redisMasterName, rf.Name)
}

func GetRedisSlaveName(rf *redisfailoverv1.RedisFailover) string {
	return generateName(redisSlaveName, rf.Name)
}

func GetHaproxySlaveName(rf *redisfailoverv1.RedisFailover) string {
	return generateName(redisHAProxySlaveRedisName, rf.Name)
}

func GetHaproxyMasterName(rf *redisfailoverv1.RedisFailover) string {
	return generateName(redisHAProxyMasterRedisName, rf.Name)
}

func generateName(typeName, metaName string) string {
	return fmt.Sprintf("%s%s-%s", baseName, typeName, metaName)
}

package v1

import "fmt"

const (
	// https://redis.io/docs/latest/operate/oss_and_stack/management/sentinel/
	minSentinelsToElectAFailover  = 3
	minRedisToPromoteAReplica     = 2
	minProxiesToSurviveAnEviction = 2
)

// UnsupportedReplicaCount is one component whose replica count is too small,
// named so a reader can act on it and so a metric can be labelled by it.
//
// +k8s:deepcopy-gen=false
type UnsupportedReplicaCount struct {
	Component string
	Reason    string
}

// UnsupportedReplicaCounts names each replica count too small for the failover
// to survive the loss of a pod, and the count it would take.
func (r *RedisFailover) UnsupportedReplicaCounts() []UnsupportedReplicaCount {
	var unsupported []UnsupportedReplicaCount

	if r.Spec.Redis.Replicas < minRedisToPromoteAReplica {
		unsupported = append(unsupported, UnsupportedReplicaCount{
			Component: "redis",
			Reason: fmt.Sprintf(
				"redis.replicas is %d. A failover needs %d Redis, so that one can be promoted when the master is lost.",
				r.Spec.Redis.Replicas, minRedisToPromoteAReplica),
		})
	}

	if r.Spec.Sentinel.Replicas < minSentinelsToElectAFailover {
		unsupported = append(unsupported, UnsupportedReplicaCount{
			Component: "sentinel",
			Reason: fmt.Sprintf(
				"sentinel.replicas is %d. A failover needs %d Sentinels to elect the one that carries it out.",
				r.Spec.Sentinel.Replicas, minSentinelsToElectAFailover),
		})
	}

	if r.Spec.Haproxy != nil && r.Spec.Haproxy.Replicas < minProxiesToSurviveAnEviction {
		unsupported = append(unsupported, UnsupportedReplicaCount{
			Component: "haproxy",
			Reason: fmt.Sprintf(
				"haproxy.replicas is %d. A failover needs %d proxies, so that a drain or a lost node leaves one serving clients.",
				r.Spec.Haproxy.Replicas, minProxiesToSurviveAnEviction),
		})
	}

	return unsupported
}

// ReplicaCountComponents names every component of this failover that has a
// minimum, so a reader of the metric sees a healthy count rather than the
// absence of an unhealthy one.
func (r *RedisFailover) ReplicaCountComponents() []string {
	components := []string{"redis", "sentinel"}
	if r.Spec.Haproxy != nil {
		components = append(components, "haproxy")
	}
	return components
}

package v1

import "fmt"

const (
	// https://redis.io/docs/latest/operate/oss_and_stack/management/sentinel/
	sentinelsToElectAFailover = 3
	redisToPromoteOne         = 2
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

	if r.Spec.Redis.Replicas < redisToPromoteOne {
		unsupported = append(unsupported, UnsupportedReplicaCount{
			Component: "redis",
			Reason: fmt.Sprintf(
				"redis.replicas is %d. A failover needs %d Redis, so that one can be promoted when the master is lost.",
				r.Spec.Redis.Replicas, redisToPromoteOne),
		})
	}

	if r.Spec.Sentinel.Replicas < sentinelsToElectAFailover {
		unsupported = append(unsupported, UnsupportedReplicaCount{
			Component: "sentinel",
			Reason: fmt.Sprintf(
				"sentinel.replicas is %d. A failover needs %d Sentinels to elect the one that carries it out.",
				r.Spec.Sentinel.Replicas, sentinelsToElectAFailover),
		})
	}

	return unsupported
}

// ReplicaCountComponents names every component that has a minimum, whether or
// not this failover meets it, so a reader of the metric sees a healthy count
// rather than the absence of an unhealthy one.
func ReplicaCountComponents() []string {
	return []string{"redis", "sentinel"}
}

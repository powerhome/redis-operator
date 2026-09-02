package service

import (
	"crypto/sha256"
	"encoding/hex"

	corev1 "k8s.io/api/core/v1"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/service/k8s"
)

// redisPasswordChecksum identifies a password without disclosing it, so that
// what a pod was built for can be compared with what the failover is now
// configured with. Pod templates are readable by anything that can get pods,
// which is a wider audience than those trusted with the secret itself.
//
// The failover's namespace and name are mixed in. A digest of the password
// alone is the same value in every cluster, so one table of precomputed hashes
// would read passwords back out of any annotation anywhere. Tying it to a
// single failover means such a table buys nothing beyond that failover, which
// is the amortisation that makes building one worthwhile. It does not defend
// against someone attacking this failover in particular: the prefix is public
// and SHA-256 is fast, so a weak password still falls to a targeted search.
//
// No password digests to no annotation, which is what a pod built without one
// carries. Hashing the empty string instead would give every such pod a digest
// it can never match, and a failover with no auth.secretPath would read as
// permanently out of date.
func redisPasswordChecksum(rf *redisfailoverv1.RedisFailover, password string) string {
	if password == "" {
		return ""
	}

	// Neither a namespace nor a name can contain "/" or a NUL, so no two
	// failovers can produce the same input from different parts.
	sum := sha256.Sum256([]byte(rf.Namespace + "/" + rf.Name + "\x00" + password))
	return hex.EncodeToString(sum[:])
}

// servingRedisPods returns the Redis pods whose password is worth comparing:
// those that are up and staying up.
//
// A pod that has not reached Running is not serving a password yet, and one
// carrying a deletion timestamp is on its way out. Restarting either changes
// nothing, and waiting on either holds back a change it will never take: a pod
// stuck terminating on a lost node would otherwise pin HAProxy on a password
// its backends have already given up.
func servingRedisPods(s k8s.Services, rf *redisfailoverv1.RedisFailover) ([]corev1.Pod, error) {
	pods, err := s.GetStatefulSetPods(rf.Namespace, GetRedisName(rf))
	if err != nil {
		return nil, err
	}
	if pods == nil {
		return nil, nil
	}

	serving := []corev1.Pod{}
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
			continue
		}
		serving = append(serving, pod)
	}

	return serving, nil
}

// redisPodServesPassword reports whether a pod was built for the password the
// digest identifies. An absent annotation means no password, which is what the
// empty digest stands for.
func redisPodServesPassword(pod corev1.Pod, digest string) bool {
	return pod.Annotations[redisPasswordChecksumKey] == digest
}

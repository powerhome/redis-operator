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

// redisPodsInService returns the Redis pods that are up and staying up: those
// that have reached Running and are not being deleted.
//
// These are the pods whose password is worth comparing. One that has not
// started is not serving anything yet, and one carrying a deletion timestamp is
// on its way out; restarting either changes nothing, and waiting on either
// holds back a change it will never take. A pod stuck terminating on a lost
// node would otherwise pin HAProxy on a password its backends have replaced.
func redisPodsInService(s k8s.Services, rf *redisfailoverv1.RedisFailover) ([]corev1.Pod, error) {
	pods, err := s.GetStatefulSetPods(rf.Namespace, GetRedisName(rf))
	if err != nil {
		return nil, err
	}
	if pods == nil {
		return nil, nil
	}

	inService := []corev1.Pod{}
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
			continue
		}
		inService = append(inService, pod)
	}

	return inService, nil
}

// redisPodBuiltForPassword reports whether a pod carries the digest of the
// password it was generated with. An absent annotation means no password, which
// is what the empty digest stands for.
//
// This reads the annotation stamped when the pod was created, not what Redis
// loaded. The two agree because the annotation and the config come from the
// same pod template, and the whole scheme rests on that.
func redisPodBuiltForPassword(pod corev1.Pod, digest string) bool {
	return pod.Annotations[redisPasswordChecksumKey] == digest
}

// anyRedisPodOnAnotherPassword reports whether some Redis pod in service was
// built for a password other than the one the digest names.
//
// This is what decides when the HAProxy Deployment may be written. HAProxy
// authenticates its health check and reads the password when its pod starts, so
// a proxy whose configuration disagrees with a Redis that is up and answering
// fails that backend for as long as the two differ. Deferring the write leaves
// the running proxy on the password its backends still have.
//
// Pods out of service are not waited for. They come back from the current pod
// template and carry the current password when they do, so one that is still
// starting, or one stuck terminating on a lost node, must not hold the proxy
// back indefinitely. With no pods in service at all there is nothing to
// disagree with and nothing to protect by waiting.
//
// Being unable to tell counts as a disagreement, which costs a pass rather than
// risking a restart onto the wrong password.
func anyRedisPodOnAnotherPassword(s k8s.Services, rf *redisfailoverv1.RedisFailover, digest string) bool {
	pods, err := redisPodsInService(s, rf)
	if err != nil {
		return true
	}

	for _, pod := range pods {
		if !redisPodBuiltForPassword(pod, digest) {
			return true
		}
	}

	return false
}

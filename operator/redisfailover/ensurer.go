package redisfailover

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/metrics"
)

// Ensure is called to ensure all of the resources associated with a RedisFailover are created
func (w *RedisFailoverHandler) Ensure(rf *redisfailoverv1.RedisFailover, labels map[string]string, or []metav1.OwnerReference, metricsClient metrics.Recorder) error {
	w.warnPodNetworkingIsGoingAway(rf)

	// This service is what names the Redis pods in DNS. The exporter only adds a
	// port to it. See docs/adr/ADR-002.
	if err := w.rfService.EnsureRedisService(rf, labels, or); err != nil {
		return err
	}

	w.warnNetworkPolicyNsListIsVestigial(rf)

	if err := w.rfService.DestroydOrphanedRedisNetworkPolicy(rf); err != nil {
		return err
	}

	if err := w.rfService.DestroyOrphanedSentinelNetworkPolicy(rf); err != nil {
		return err
	}

	// Whenever HAProxy should not be running, its resources are removed. That
	// covers the failover being bootstrapped, and it covers the haproxy block
	// being taken out of the spec: previously the outer condition let the second
	// case reach neither branch, so a proxy kept serving traffic from a
	// configuration nothing would update again.
	if rf.Spec.Haproxy != nil && rf.HaproxyAllowed() {
		if err := w.rfService.EnsureHAProxyRedisMasterService(rf, labels, or); err != nil {
			return err
		}

		if err := w.rfService.EnsureRedisHeadlessService(rf, labels, or); err != nil {
			return err
		}

		if err := w.rfService.EnsureHAProxyRedisMasterConfigmap(rf, labels, or); err != nil {
			return err
		}

		if err := w.rfService.EnsureHAProxyRedisMasterDeployment(rf, labels, or); err != nil {
			return err
		}

		if err := w.rfService.DestroyOrphanedRedisSlaveHaProxy(rf); err != nil {
			return err
		}
	} else {
		if err := w.rfService.DestroyHaproxyMasterResources(rf); err != nil {
			return err
		}
	}

	if err := w.rfService.EnsureRedisMasterService(rf, labels, or); err != nil {
		return err
	}

	if err := w.rfService.EnsureRedisSlaveService(rf, labels, or); err != nil {
		return err
	}

	if err := w.rfService.EnsureRedisShutdownConfigMap(rf, labels, or); err != nil {
		return err
	}
	if err := w.rfService.EnsureRedisReadinessConfigMap(rf, labels, or); err != nil {
		return err
	}
	if err := w.rfService.EnsureRedisConfigMap(rf, labels, or); err != nil {
		return err
	}
	if err := w.rfService.EnsureRedisStatefulset(rf, labels, or); err != nil {
		return err
	}

	sentinelsAllowed := rf.SentinelsAllowed()
	if sentinelsAllowed {

		if err := w.rfService.EnsureSentinelService(rf, labels, or); err != nil {
			return err
		}
		if err := w.rfService.EnsureSentinelConfigMap(rf, labels, or); err != nil {
			return err
		}

		// A failover that has given its Sentinels storage runs them as a set,
		// so what each one learns survives it. Without that they stay a
		// Deployment and are told the topology on every start.
		if rf.Spec.Sentinel.Storage.PersistentVolumeClaim != nil {
			if err := w.rfService.EnsureSentinelHeadlessService(rf, labels, or); err != nil {
				return err
			}
			if err := w.rfService.EnsureSentinelStatefulSet(rf, labels, or); err != nil {
				return err
			}
		} else {
			if err := w.rfService.EnsureSentinelDeployment(rf, labels, or); err != nil {
				return err
			}
		}
	} else {
		if err := w.rfService.DestroySentinelResources(rf); err != nil {
			return err
		}
	}

	return nil
}

// warnPodNetworkingIsGoingAway names the deprecated pod networking fields a
// RedisFailover sets, which nothing the reader can query would tell them.
//
// Once per failover, per operator process: a reconcile happens every few seconds
// and the fields still do what they say, so there is nothing to act on urgently.
// See docs/cir/CIR-010.
func (w *RedisFailoverHandler) warnPodNetworkingIsGoingAway(rf *redisfailoverv1.RedisFailover) {
	var set []string
	if rf.Spec.Redis.HostNetwork {
		set = append(set, "redis.hostNetwork")
	}
	if rf.Spec.Redis.DNSPolicy != "" {
		set = append(set, "redis.dnsPolicy")
	}
	if rf.Spec.Sentinel.HostNetwork {
		set = append(set, "sentinel.hostNetwork")
	}
	if rf.Spec.Sentinel.DNSPolicy != "" {
		set = append(set, "sentinel.dnsPolicy")
	}
	if len(set) == 0 {
		return
	}

	key := fmt.Sprintf("%s/%s", rf.Namespace, rf.Name)
	if _, said := w.warnedPodNetworking.LoadOrStore(key, true); said {
		return
	}

	w.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).
		Warningf("deprecated fields set on this RedisFailover: %s. How a Redis or Sentinel instance can be addressed is the operator's to decide, and pod networking fields change it. They still apply, and a later release removes them from the API with no replacement. Take them out while they still work.", strings.Join(set, ", "))
}

// warnNetworkPolicyNsListIsVestigial tells the reader of a RedisFailover that the
// field decides nothing, which nothing they can query would tell them.
//
// Once per failover, per operator process; see docs/cir/CIR-007 for why not every
// reconcile and why not a status condition.
func (w *RedisFailoverHandler) warnNetworkPolicyNsListIsVestigial(rf *redisfailoverv1.RedisFailover) {
	if len(rf.Spec.NetworkPolicyNsList) == 0 {
		return
	}

	key := fmt.Sprintf("%s/%s", rf.Namespace, rf.Name)
	if _, said := w.warnedNetworkPolicyNsList.LoadOrStore(key, true); said {
		return
	}

	w.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).
		Warningf("networkPolicyNsList is set and does nothing: the operator no longer writes a NetworkPolicy for the sentinels, and removes the one it used to write. Take the field out of this RedisFailover, and write the policy yourself if you want one. The field will be removed from the API in a later release")
}

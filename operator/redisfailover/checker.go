package redisfailover

import (
	"errors"
	"fmt"
	"time"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/metrics"
	"github.com/spotahome/redis-operator/service/redis"
)

// UpdateRedisesPods if the running version of pods are equal to the statefulset one
func (r *RedisFailoverHandler) UpdateRedisesPods(rf *redisfailoverv1.RedisFailover) error {
	redises, err := r.rfChecker.GetRedisesIPs(rf)
	if err != nil {
		return err
	}

	masterIP := ""
	if !rf.Bootstrapping() {
		masterIP, _ = r.rfChecker.GetMasterIP(rf)
	}
	// No perform updates when nodes are syncing, still not connected, etc.
	for _, rip := range redises {
		if rip != masterIP {
			ready, err := r.rfChecker.CheckRedisSlavesReady(rip, rf)
			if err != nil {
				return err
			}
			if !ready {
				return nil
			}
		}
	}

	ssUR, err := r.rfChecker.GetStatefulSetUpdateRevision(rf)
	if err != nil {
		return err
	}

	redisesPods, err := r.rfChecker.GetRedisesSlavesPods(rf)
	if err != nil {
		return err
	}

	// Update stale pods with slave role
	for _, pod := range redisesPods {
		revision, err := r.rfChecker.GetRedisRevisionHash(pod, rf)
		if err != nil {
			return err
		}
		if revision != ssUR {
			//Delete pod and wait next round to check if the new one is synced
			err = r.rfHealer.DeletePod(pod, rf)
			if err != nil {
				return err
			}
			return nil
		}
	}

	if !rf.Bootstrapping() {
		// Update stale pod with role master
		master, err := r.rfChecker.GetRedisesMasterPod(rf)
		if err != nil {
			return err
		}

		masterRevision, err := r.rfChecker.GetRedisRevisionHash(master, rf)
		if err != nil {
			return err
		}
		if masterRevision != ssUR {
			err = r.rfHealer.DeletePod(master, rf)
			if err != nil {
				return err
			}
			return nil
		}
	}

	return nil
}

// applyCredentialChange restarts the Redis pods that are not yet running the
// current pod template, so they pick up the password the failover is configured
// with.
//
// They go together, not one at a time. A pod that has restarted has the new
// requirepass and masterauth while one that has not has the old, so replication
// between them fails for as long as they disagree. Rolling them singly leaves
// the failover in that split for a whole reconcile at least: the pod that
// already restarted cannot sync, so it never reports ready, and the wait for it
// to become ready is what would drive the next restart. Nothing arrives to end
// it. Restarting together costs a short window with no Redis at all, which
// sentinel and the clients already handle, instead of a split that persists.
//
// While bootstrapping the pods replicate from the external node rather than from
// each other, so that split cannot arise there. They still go together: the
// authoritative data is on the bootstrap node, which the restart does not touch,
// so there is nothing to gain from drawing it out.
//
// Only pods on a stale template are restarted. If they all carry the current one
// and Redis still refuses the credential, restarting cannot help -- the secret
// itself is wrong, or its value never reached the config -- and repeating it
// would be an endless restart loop, so the refusal is returned instead.
func (r *RedisFailoverHandler) applyCredentialChange(rf *redisfailoverv1.RedisFailover, authErr error) error {
	logger := r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace)

	stale, err := r.rfChecker.GetRedisesPodsWithStalePassword(rf)
	if err != nil {
		return err
	}

	if len(stale) == 0 {
		logger.Errorf("Redis refuses the configured password on pods that already carry the current configuration, so restarting them will not help: %s", authErr.Error())
		return authErr
	}

	logger.Warningf("Redis is not using the password the failover is configured with, restarting %d pod(s) together to apply it: %s", len(stale), authErr.Error())

	// Every stale pod is attempted before anything is reported. Stopping at the
	// first failure would leave behind exactly the half-restarted failover this
	// goes to such lengths to avoid: some pods on the new password, some on the
	// old, and replication between them refused.
	var errs []error
	for _, pod := range stale {
		if err := r.rfHealer.DeletePod(pod, rf); err != nil {
			errs = append(errs, fmt.Errorf("restarting %s: %w", pod, err))
		}
	}

	return errors.Join(errs...)
}

// CheckAndHeal runs verifcation checks to ensure the RedisFailover is in an expected and healthy state.
// If the checks do not match up to expectations, an attempt will be made to "heal" the RedisFailover into a healthy state.
func (r *RedisFailoverHandler) CheckAndHeal(rf *redisfailoverv1.RedisFailover) error {
	if rf.Bootstrapping() {
		return r.checkAndHealBootstrapMode(rf)
	}

	// Number of redis is equal as the set on the RF spec
	// Number of sentinel is equal as the set on the RF spec
	// Check only one master
	// Number of redis master is 1
	// All redis slaves have the same master
	// All sentinels points to the same redis master
	// Sentinel has not death nodes
	// Sentinel knows the correct slave number

	if !r.rfChecker.IsRedisRunning(rf) {
		setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.REDIS_REPLICA_MISMATCH, metrics.NOT_APPLICABLE, errors.New("not all replicas running"))
		r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Debugf("Number of redis mismatch, waiting for redis statefulset reconcile")
		return nil
	}

	if !r.rfChecker.IsSentinelRunning(rf) {
		setRedisCheckerMetrics(r.mClient, "sentinel", rf.Namespace, rf.Name, metrics.SENTINEL_REPLICA_MISMATCH, metrics.NOT_APPLICABLE, errors.New("not all replicas running"))
		r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Debugf("Number of sentinel mismatch, waiting for sentinel deployment reconcile")
		return nil
	}

	nMasters, err := r.rfChecker.GetNumberMasters(rf)
	if redis.IsAuthError(err) {
		// The running Redis is not using the password the failover is
		// configured with. Two ordinary actions leave it that way: adding
		// auth.secretPath to a running failover, and rotating the value in the
		// secret it names. In both the generated config carries the new
		// password and the running Redis does not, because it reads that config
		// only at startup.
		//
		// Nothing below can see past this. Every check authenticates, and the
		// rolling update that applies the new password sits behind all of them,
		// so the failover would wedge here and never reach its own repair.
		// Waiting does not help either, since the pods keep whatever they
		// started with.
		return r.applyCredentialChange(rf, err)
	}
	if err != nil {
		return err
	}

	switch nMasters {
	case 0:
		setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.NO_MASTER, metrics.NOT_APPLICABLE, errors.New("no masters detected"))
		//when number of redis replicas is 1 , the redis is configured for standalone master mode
		//Configure to master
		if rf.Spec.Redis.Replicas == 1 {
			r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Infof("Resource spec with standalone master - operator will set the master")
			err = r.rfHealer.SetOldestAsMaster(rf)
			setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.NO_MASTER, metrics.NOT_APPLICABLE, err)
			if err != nil {
				r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Errorf("Error in Setting oldest Pod as master")
				return err
			}
			return nil
		}
		//During the First boot(New deployment or all pods of the statefulsets have restarted),
		//Sentinesl will not be able to choose the master , so operator should select a master
		//Also in scenarios where Sentinels is not in a position to choose a master like , No quorum reached
		//Operator can choose a master , These scenarios can be checked by asking the all the sentinels
		//if its in a postion to choose a master also check if the redis is configured with local host IP as master.
		r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Warningf("Number of Masters running is 0")
		maxUptime, err := r.rfChecker.GetMaxRedisPodTime(rf)
		if err != nil {
			return err
		}

		r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Infof("No master avaiable but max pod up time is : %f", maxUptime.Round(time.Second).Seconds())
		//Check If Sentinel has quorum to take a failover decision
		noqrm_cnt, err := r.rfChecker.CheckSentinelQuorum(rf)
		if err != nil {
			// Sentinels are not in a situation to choose a master we pick one
			r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Warningf("Quorum not available for sentinel to choose master,estimated unhealthy sentinels :%d , Operator to step-in", noqrm_cnt)
			err2 := r.rfHealer.SetOldestAsMaster(rf)
			setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.NO_MASTER, metrics.NOT_APPLICABLE, err2)
			if err2 != nil {
				r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Errorf("Error in Setting oldest Pod as master")
				return err2
			}
		} else {
			//sentinels are having a quorum to make a failover , but check if redis are not having local hostip (first boot) as master
			status, err2 := r.rfChecker.CheckIfMasterLocalhost(rf)
			if err2 != nil {
				r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Errorf("CheckIfMasterLocalhost failed retry later")
				return err2
			} else if status {
				// all avaialable redis pods have local host ip as master
				r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Errorf("all available redis is having local loop back as master , operator initiates master selection")
				err3 := r.rfHealer.SetOldestAsMaster(rf)
				setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.NO_MASTER, metrics.NOT_APPLICABLE, err3)
				if err3 != nil {
					r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Errorf("Error in Setting oldest Pod as master")
					return err3
				}

			} else {

				// We'll wait until failover is done
				r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Infof("no master found, wait until failover or fix manually")
				setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.NO_MASTER, metrics.NOT_APPLICABLE, errors.New("no master not fixed, wait until failover or fix manually"))
				return nil
			}

		}

	case 1:
		setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.NUMBER_OF_MASTERS, metrics.NOT_APPLICABLE, nil)
	default:
		setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.NUMBER_OF_MASTERS, metrics.NOT_APPLICABLE, errors.New("multiple masters detected"))
		return errors.New("more than one master, fix manually")
	}

	master, err := r.rfChecker.GetMasterIP(rf)
	if err != nil {
		return err
	}

	err = r.rfChecker.CheckAllSlavesFromMaster(master, rf)
	setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.SLAVE_WRONG_MASTER, metrics.NOT_APPLICABLE, err)
	if err != nil {
		r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Warningf("Slave not associated to master: %s", err.Error())
		if err = r.rfHealer.SetMasterOnAll(master, rf); err != nil {
			return err
		}
	}

	err = r.rfChecker.CheckNumberRedisConnectedSlaves(master, rf)
	setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.REDIS_SLAVES_NUMBER_CONNECTED_MISMATCH, metrics.NOT_APPLICABLE, err)
	if err != nil {
		r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Warningf("Master has wrong number of slaves: %s", err.Error())
		if err = r.rfHealer.ResetReplicaConnections(master, rf); err != nil {
			return err
		}
	}

	err = r.applyRedisCustomConfig(rf)
	setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.APPLY_REDIS_CONFIG, metrics.NOT_APPLICABLE, err)
	if err != nil {
		return err
	}

	err = r.UpdateRedisesPods(rf)
	if err != nil {
		return err
	}

	sentinels, err := r.rfChecker.GetSentinelsIPs(rf)
	if err != nil {
		return err
	}

	port := rf.Spec.Redis.Port.ToString()
	sentinelPort := rf.Spec.Sentinel.Port.ToString()
	for _, sip := range sentinels {
		err = r.rfChecker.CheckSentinelMonitor(sip, sentinelPort, master, port)
		setRedisCheckerMetrics(r.mClient, "sentinel", rf.Namespace, rf.Name, metrics.SENTINEL_WRONG_MASTER, sip, err)
		if err != nil {
			r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Warningf("Fixing sentinel not monitoring expected master: %s", err.Error())
			if err := r.rfHealer.NewSentinelMonitor(sip, master, rf); err != nil {
				return err
			}
		}
	}
	return r.checkAndHealSentinels(rf, sentinels)
}

func (r *RedisFailoverHandler) checkAndHealBootstrapMode(rf *redisfailoverv1.RedisFailover) error {

	if !r.rfChecker.IsRedisRunning(rf) {
		setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.REDIS_REPLICA_MISMATCH, metrics.NOT_APPLICABLE, errors.New("not all replicas running"))
		r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Debugf("Number of redis mismatch, waiting for redis statefulset reconcile")
		return nil
	}

	// Bootstrapping never reaches the credential handling in CheckAndHeal, which
	// returns here on its first line, and UpdateRedisesPods below authenticates.
	// Without this a password change wedges the failover at that call exactly as
	// it used to everywhere else.
	//
	// GetNumberMasters serves as the probe: it asks every pod and reports a
	// refusal only when none of them answered, so a pod that is merely
	// unreachable does not trigger a restart. Its count is meaningless while
	// bootstrapping, since every pod is a replica of the external node by
	// design, and is deliberately ignored.
	if _, err := r.rfChecker.GetNumberMasters(rf); err != nil {
		if redis.IsAuthError(err) {
			return r.applyCredentialChange(rf, err)
		}
		// Any other failure belongs to the calls below, which report what they
		// cannot do. Recording it keeps a probe that failed from reading like
		// one that found nothing wrong.
		r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Debugf("Could not probe for a refused credential, carrying on: %v", err)
	}

	err := r.UpdateRedisesPods(rf)
	if err != nil {
		return err
	}
	err = r.applyRedisCustomConfig(rf)
	setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.APPLY_REDIS_CONFIG, metrics.NOT_APPLICABLE, err)
	if err != nil {
		return err
	}

	bootstrapSettings := rf.Spec.BootstrapNode
	err = r.rfHealer.SetExternalMasterOnAll(bootstrapSettings.Host, bootstrapSettings.Port, rf)
	setRedisCheckerMetrics(r.mClient, "redis", rf.Namespace, rf.Name, metrics.APPLY_EXTERNAL_MASTER, metrics.NOT_APPLICABLE, err)
	if err != nil {
		return err
	}

	if rf.SentinelsAllowed() {
		if !r.rfChecker.IsSentinelRunning(rf) {
			setRedisCheckerMetrics(r.mClient, "sentinel", rf.Namespace, rf.Name, metrics.SENTINEL_REPLICA_MISMATCH, metrics.NOT_APPLICABLE, errors.New("not all replicas running"))
			r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Debugf("Number of sentinel mismatch, waiting for sentinel deployment reconcile")
			return nil
		}

		sentinels, err := r.rfChecker.GetSentinelsIPs(rf)
		if err != nil {
			return err
		}
		for _, sip := range sentinels {
			err = r.rfChecker.CheckSentinelMonitor(sip, bootstrapSettings.Host, bootstrapSettings.Port)
			setRedisCheckerMetrics(r.mClient, "sentinel", rf.Namespace, rf.Name, metrics.SENTINEL_WRONG_MASTER, sip, err)
			if err != nil {
				r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Warningf("Fixing sentinel not monitoring expected master: %s", err.Error())
				if err := r.rfHealer.NewSentinelMonitorWithPort(sip, bootstrapSettings.Host, bootstrapSettings.Port, rf); err != nil {
					return err
				}
			}
		}
		return r.checkAndHealSentinels(rf, sentinels)
	}
	return nil
}

func (r *RedisFailoverHandler) applyRedisCustomConfig(rf *redisfailoverv1.RedisFailover) error {
	redises, err := r.rfChecker.GetRedisesIPs(rf)
	if err != nil {
		return err
	}
	for _, rip := range redises {
		if err := r.rfHealer.SetRedisCustomConfig(rip, rf); err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisFailoverHandler) checkAndHealSentinels(rf *redisfailoverv1.RedisFailover, sentinels []string) error {
	sentinelPort := rf.Spec.Sentinel.Port.ToString()
	for _, sip := range sentinels {
		err := r.rfChecker.CheckSentinelNumberInMemory(sip, rf)
		setRedisCheckerMetrics(r.mClient, "sentinel", rf.Namespace, rf.Name, metrics.SENTINEL_NUMBER_IN_MEMORY_MISMATCH, sip, err)
		if err != nil {
			r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Warningf("Sentinel %s mismatch number of sentinels in memory. resetting", sip)
			if err := r.rfHealer.RestoreSentinel(sip, sentinelPort); err != nil {
				return err
			}
		}

	}
	for _, sip := range sentinels {
		err := r.rfChecker.CheckSentinelSlavesNumberInMemory(sip, rf)
		setRedisCheckerMetrics(r.mClient, "sentinel", rf.Namespace, rf.Name, metrics.REDIS_SLAVES_NUMBER_IN_MEMORY_MISMATCH, sip, err)
		if err != nil {
			r.logger.WithField("redisfailover", rf.ObjectMeta.Name).WithField("namespace", rf.ObjectMeta.Namespace).Warningf("Sentinel %s mismatch number of expected slaves in memory. resetting", sip)
			if err := r.rfHealer.RestoreSentinel(sip, sentinelPort); err != nil {
				return err
			}
		}
	}
	for _, sip := range sentinels {
		err := r.rfHealer.SetSentinelCustomConfig(sip, rf)
		setRedisCheckerMetrics(r.mClient, "sentinel", rf.Namespace, rf.Name, metrics.APPLY_SENTINEL_CONFIG, sip, err)
		if err != nil {
			return err
		}
	}
	return nil
}

func setRedisCheckerMetrics(metricsClient metrics.Recorder, mode /* redis or sentinel? */ string, rfNamespace string, rfName string, property string, IP string, err error) {
	if mode == "sentinel" {
		if err != nil {
			metricsClient.RecordSentinelCheck(rfNamespace, rfName, property, IP, metrics.STATUS_UNHEALTHY)
		} else {
			metricsClient.RecordSentinelCheck(rfNamespace, rfName, property, IP, metrics.STATUS_HEALTHY)
		}

	} else if mode == "redis" {
		if err != nil {
			metricsClient.RecordRedisCheck(rfNamespace, rfName, property, IP, metrics.STATUS_UNHEALTHY)
		} else {
			metricsClient.RecordRedisCheck(rfNamespace, rfName, property, IP, metrics.STATUS_HEALTHY)
		}
	}
}

package service

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	"github.com/spotahome/redis-operator/operator/redisfailover/util"
	"github.com/spotahome/redis-operator/service/k8s"
	"github.com/spotahome/redis-operator/service/redis"
)

// RedisFailoverCheck defines the interface able to check the correct status of a redis failover
type RedisFailoverCheck interface {
	CheckRedisNumber(rFailover *redisfailoverv1.RedisFailover) error
	CheckSentinelNumber(rFailover *redisfailoverv1.RedisFailover) error
	CheckAllSlavesFromMaster(masterHostname string, rFailover *redisfailoverv1.RedisFailover) error
	CheckSentinelNumberInMemory(sentinel string, rFailover *redisfailoverv1.RedisFailover) error
	CheckNumberRedisConnectedSlaves(masterIP string, rFailover *redisfailoverv1.RedisFailover) error
	CheckSentinelSlavesNumberInMemory(sentinel string, rFailover *redisfailoverv1.RedisFailover) error
	CheckSentinelQuorum(rFailover *redisfailoverv1.RedisFailover) (int, error)
	CheckSentinelsCanFailover(rFailover *redisfailoverv1.RedisFailover, replacing string) error
	CheckIfMasterLocalhost(rFailover *redisfailoverv1.RedisFailover) (bool, error)
	CheckSentinelMonitor(sentinel string, sentinelPort string, monitor ...string) error
	GetMasterIP(rFailover *redisfailoverv1.RedisFailover) (string, error)
	GetRedisHostnameAt(rFailover *redisfailoverv1.RedisFailover, address string) (string, error)
	GetNumberMasters(rFailover *redisfailoverv1.RedisFailover) (int, error)
	GetRedisesIPs(rFailover *redisfailoverv1.RedisFailover) ([]string, error)
	GetSentinelsIPs(rFailover *redisfailoverv1.RedisFailover) ([]string, error)
	GetSentinelRememberedMaster(rFailover *redisfailoverv1.RedisFailover) (string, error)
	GetMaxRedisPodTime(rFailover *redisfailoverv1.RedisFailover) (time.Duration, error)
	GetRedisesPodsWithStalePassword(rFailover *redisfailoverv1.RedisFailover) ([]string, error)
	GetRedisesSlavesPods(rFailover *redisfailoverv1.RedisFailover) ([]string, error)
	GetRedisesMasterPod(rFailover *redisfailoverv1.RedisFailover) (string, error)
	GetStatefulSetUpdateRevision(rFailover *redisfailoverv1.RedisFailover) (string, error)
	GetRedisRevisionHash(podName string, rFailover *redisfailoverv1.RedisFailover) (string, error)
	GetRedisesPodsWaitingOnFilesystemResize(rFailover *redisfailoverv1.RedisFailover) (map[string]bool, error)
	CheckRedisSlavesReady(slaveIP string, rFailover *redisfailoverv1.RedisFailover) (bool, error)
	IsRedisRunning(rFailover *redisfailoverv1.RedisFailover) bool
	IsSentinelRunning(rFailover *redisfailoverv1.RedisFailover) bool
	IsClusterRunning(rFailover *redisfailoverv1.RedisFailover) bool
	IsHAProxyRunning(rFailover *redisfailoverv1.RedisFailover) bool
}

// RedisFailoverChecker is our implementation of RedisFailoverCheck interface
type RedisFailoverChecker struct {
	k8sService    k8s.Services
	redisClient   redis.Client
	logger        log.Logger
	metricsClient metrics.Recorder
}

// NewRedisFailoverChecker creates an object of the RedisFailoverChecker struct
func NewRedisFailoverChecker(k8sService k8s.Services, redisClient redis.Client, logger log.Logger, metricsClient metrics.Recorder) *RedisFailoverChecker {
	return &RedisFailoverChecker{
		k8sService:    k8sService,
		redisClient:   redisClient,
		logger:        logger,
		metricsClient: metricsClient,
	}
}

// CheckRedisNumber controlls that the number of deployed redis is the same than the requested on the spec
func (r *RedisFailoverChecker) CheckRedisNumber(rf *redisfailoverv1.RedisFailover) error {
	ss, err := r.k8sService.GetStatefulSet(rf.Namespace, GetRedisName(rf))
	if err != nil {
		return err
	}
	if rf.Spec.Redis.Replicas != *ss.Spec.Replicas {
		return errors.New("number of redis pods differ from specification")
	}
	return nil
}

// CheckSentinelNumber controlls that the number of deployed sentinel is the same than the requested on the spec
func (r *RedisFailoverChecker) CheckSentinelNumber(rf *redisfailoverv1.RedisFailover) error {
	d, err := r.k8sService.GetDeployment(rf.Namespace, GetSentinelName(rf))
	if err != nil {
		return err
	}
	if rf.Spec.Sentinel.Replicas != *d.Spec.Replicas {
		return errors.New("number of sentinel pods differ from specification")
	}
	return nil
}

func (r *RedisFailoverChecker) setMasterLabelIfNecessary(namespace string, pod corev1.Pod) error {
	for labelKey, labelValue := range pod.ObjectMeta.Labels {
		if labelKey == redisRoleLabelKey && labelValue == redisRoleLabelMaster {
			return nil
		}
	}
	return r.k8sService.UpdatePodLabels(namespace, pod.ObjectMeta.Name, generateRedisMasterRoleLabel())
}

func (r *RedisFailoverChecker) setSlaveLabelIfNecessary(namespace string, pod corev1.Pod) error {
	for labelKey, labelValue := range pod.ObjectMeta.Labels {
		if labelKey == redisRoleLabelKey && labelValue == redisRoleLabelSlave {
			return nil
		}
	}
	return r.k8sService.UpdatePodLabels(namespace, pod.ObjectMeta.Name, generateRedisSlaveRoleLabel())
}

// CheckAllSlavesFromMaster fails when a Redis is following anything other than
// the named master, and labels each pod with the role it is found in.
//
// The master is named rather than addressed, because a name is what everything
// writes into a replica: Sentinel after a failover, and the operator when it
// repoints one. See docs/adr/ADR-002.
func (r *RedisFailoverChecker) CheckAllSlavesFromMaster(masterHostname string, rf *redisfailoverv1.RedisFailover) error {
	rps, err := r.k8sService.GetStatefulSetPods(rf.Namespace, GetRedisName(rf))
	if err != nil {
		return err
	}

	password, err := k8s.GetRedisPassword(r.k8sService, rf)
	if err != nil {
		return err
	}

	rport := rf.Spec.Redis.Port.ToString()
	for _, rp := range rps.Items {
		if RedisPodHostname(rf, rp.Name) == masterHostname {
			err = r.setMasterLabelIfNecessary(rf.Namespace, rp)
			if err != nil {
				return err
			}
		} else {
			err = r.setSlaveLabelIfNecessary(rf.Namespace, rp)
			if err != nil {
				return err
			}
		}

		slave, err := r.redisClient.GetSlaveOf(rp.Status.PodIP, rport, password)
		if err != nil {
			r.logger.Errorf("Get slave of master failed, maybe this node is not ready, pod ip: %s", rp.Status.PodIP)
			return err
		}
		if slave != "" && slave != masterHostname {
			return fmt.Errorf("slave %s is not following %s, but %s", rp.Name, masterHostname, slave)
		}
	}
	return nil
}

// CheckSentinelNumberInMemory controls that the provided sentinel has only the living sentinels on its memory.
func (r *RedisFailoverChecker) CheckSentinelNumberInMemory(sentinel string, rf *redisfailoverv1.RedisFailover) error {
	portString := rf.Spec.Sentinel.Port.ToString()
	nSentinels, err := r.redisClient.GetNumberSentinelsInMemory(sentinel, portString)
	if err != nil {
		return err
	} else if nSentinels != rf.Spec.Sentinel.Replicas {
		return errors.New("sentinels in memory mismatch")
	}
	return nil
}

// This function will check if the local host ip is set as the master for all currently available pods
// This  can be used to detect the fresh boot of all the redis pods
// This function returns true if it all available pods have local host ip as master,
// false if atleast one of the ip is not local hostip
// false and error if any function fails
func (r *RedisFailoverChecker) CheckIfMasterLocalhost(rFailover *redisfailoverv1.RedisFailover) (bool, error) {

	var lhmaster int = 0
	redisIps, err := r.GetRedisesIPs(rFailover)
	if len(redisIps) == 0 || err != nil {
		r.logger.Warningf("CheckIfMasterLocalhost GetRedisesIPs Failed- unable to fetch any redis Ips Currently")
		return false, errors.New("unable to fetch any redis Ips Currently")
	}
	password, err := k8s.GetRedisPassword(r.k8sService, rFailover)
	if err != nil {
		r.logger.Errorf("CheckIfMasterLocalhost -- GetRedisPassword Failed")
		return false, err
	}
	rport := rFailover.Spec.Redis.Port.ToString()
	for _, sip := range redisIps {
		master, err := r.redisClient.GetSlaveOf(sip, rport, password)
		if err != nil {
			r.logger.Warningf("CheckIfMasterLocalhost -- GetSlaveOf Failed")
			return false, err
		} else if master == "" {
			r.logger.Warningf("CheckIfMasterLocalhost -- Master already available ?? check manually")
			return false, errors.New("unexpected master state, fix manually")
		} else {
			if master == noMasterYet {
				lhmaster++
			}
		}
	}
	if lhmaster == len(redisIps) {
		r.logger.Infof("all available redis configured localhost as master , operator must heal")
		return true, nil
	}
	r.logger.Infof("atleast one pod does not have localhost as master , operator should not heal")
	return false, nil
}

// This function will call the sentinel client apis to check with sentinel if the sentinel is in a state
// to heal the redis system
func (r *RedisFailoverChecker) CheckSentinelQuorum(rFailover *redisfailoverv1.RedisFailover) (int, error) {

	var unhealthyCnt int = -1

	sentinels, err := r.GetSentinelsIPs(rFailover)
	if err != nil {
		r.logger.Warningf("CheckSentinelQuorum Error in getting sentinel Ip's")
		return unhealthyCnt, err
	}
	if len(sentinels) < int(getQuorum(rFailover)) {
		unhealthyCnt = int(getQuorum(rFailover)) - len(sentinels)
		r.logger.Warningf("insufficnet sentinel to reach Quorum - Unhealthy count: %d", unhealthyCnt)
		return unhealthyCnt, errors.New("insufficnet sentinel to reach Quorum")
	}

	portString := strconv.Itoa(int(rFailover.Spec.Sentinel.Port))

	unhealthyCnt = 0
	for _, sip := range sentinels {
		err = r.redisClient.SentinelCheckQuorum(sip, portString)
		if err != nil {
			unhealthyCnt += 1
		} else {
			continue
		}
	}
	if unhealthyCnt < int(getQuorum(rFailover)) {
		return unhealthyCnt, nil
	} else {
		r.logger.Errorf("insufficnet sentinel to reach Quorum - Unhealthy count: %d", unhealthyCnt)
		return unhealthyCnt, errors.New("insufficnet sentinel to reach Quorum")
	}
}

// CheckSentinelsCanFailover reports whether every Sentinel holds a replica it
// could promote, which is what taking the master away asks them to do.
//
// A Sentinel that has discarded its replica list answers a missing master with
// -failover-abort-no-good-slave until it reads the list again.
//
// Every Sentinel rather than one, because any of them may be the leader that
// has to carry out the promotion.
//
// replacing names the Redis pod the caller is about to delete, and it does not
// count towards what a Sentinel could promote. See docs/cir/CIR-008.
func (r *RedisFailoverChecker) CheckSentinelsCanFailover(rf *redisfailoverv1.RedisFailover, replacing string) error {
	sentinels, err := r.GetSentinelsIPs(rf)
	if err != nil {
		return err
	}
	if len(sentinels) == 0 {
		return errors.New("no sentinel is running to fail over")
	}

	excluding, err := r.redisNameAndAddressOf(rf, replacing)
	if err != nil {
		return err
	}

	port := rf.Spec.Sentinel.Port.ToString()
	for _, sip := range sentinels {
		promotable, err := r.redisClient.PromotableReplicas(sip, port, excluding)
		if err != nil {
			return fmt.Errorf("asking sentinel %s what it could promote: %w", sip, err)
		}
		if promotable == 0 {
			if replacing == "" {
				return fmt.Errorf("sentinel %s holds no replica it could promote", sip)
			}
			return fmt.Errorf("sentinel %s holds no replica it could promote once %s is gone", sip, replacing)
		}
	}
	return nil
}

func (r *RedisFailoverChecker) redisNameAndAddressOf(rf *redisfailoverv1.RedisFailover, podName string) ([]string, error) {
	if podName == "" {
		return nil, nil
	}

	addresses := []string{RedisPodHostname(rf, podName)}
	rps, err := r.k8sService.GetStatefulSetPods(rf.Namespace, GetRedisName(rf))
	if err != nil {
		return nil, err
	}
	for _, rp := range rps.Items {
		if rp.ObjectMeta.Name == podName && rp.Status.PodIP != "" {
			addresses = append(addresses, rp.Status.PodIP)
		}
	}
	return addresses, nil
}

// CheckSentinelSlavesNumberInMemory controls that the provided sentinel has only the expected slaves number.
func (r *RedisFailoverChecker) CheckSentinelSlavesNumberInMemory(sentinel string, rf *redisfailoverv1.RedisFailover) error {
	portString := rf.Spec.Sentinel.Port.ToString()
	nSlaves, err := r.redisClient.GetNumberSentinelSlavesInMemory(sentinel, portString)
	if err != nil {
		return err
	} else {
		if rf.Bootstrapping() {
			if nSlaves != rf.Spec.Redis.Replicas {
				return errors.New("redis slaves in sentinel memory mismatch")
			}
		} else {
			if nSlaves != rf.Spec.Redis.Replicas-1 {
				return errors.New("redis slaves in sentinel memory mismatch")
			}
		}
	}
	return nil
}

func (r *RedisFailoverChecker) CheckNumberRedisConnectedSlaves(masterIP string, rf *redisfailoverv1.RedisFailover) error {
	password, err := k8s.GetRedisPassword(r.k8sService, rf)
	if err != nil {
		return fmt.Errorf("resolving redis password: %w", err)
	}

	portString := rf.Spec.Redis.Port.ToString()
	nSlaves, err := r.redisClient.GetNumberRedisConnectedSlaves(masterIP, portString, password)
	if err != nil {
		return err
	} else {
		if nSlaves != rf.Spec.Redis.Replicas-1 {
			return errors.New("redis number of slaves mismatch")
		}
	}
	return nil
}

// CheckSentinelMonitor controls if the sentinels are monitoring the expected master
func (r *RedisFailoverChecker) CheckSentinelMonitor(sentinel string, sentinelPort string, monitor ...string) error {
	monitorIP := monitor[0]
	monitorPort := ""
	if len(monitor) > 1 {
		monitorPort = monitor[1]
	}

	actualMonitorIP, actualMonitorPort, err := r.redisClient.GetSentinelMonitor(sentinel, sentinelPort)
	if err != nil {
		return err
	}
	if actualMonitorIP != monitorIP || (monitorPort != "" && monitorPort != actualMonitorPort) {
		return fmt.Errorf("sentinel monitoring %s:%s instead %s:%s", actualMonitorIP, actualMonitorPort, monitorIP, monitorPort)
	}
	return nil
}

// RedisPodHostname is the name a Redis pod answers to in DNS. The service named
// in it is the one the StatefulSet is created with as its serviceName, which
// Kubernetes holds immutable afterwards, so this record cannot be pointed
// somewhere else for a failover that already exists.
//
// Nothing outside that namespace and set can answer to it. See docs/adr/ADR-002.
func RedisPodHostname(rf *redisfailoverv1.RedisFailover, podName string) string {
	return fmt.Sprintf("%s.%s.%s.svc", podName, GetRedisName(rf), rf.Namespace)
}

// GetRedisHostnameAt names the Redis pod holding the given address. It decides
// nothing about roles, so a caller that has established which pod is the master
// can name it without inviting a second answer to that question.
func (r *RedisFailoverChecker) GetRedisHostnameAt(rf *redisfailoverv1.RedisFailover, address string) (string, error) {
	rps, err := r.k8sService.GetStatefulSetPods(rf.Namespace, GetRedisName(rf))
	if err != nil {
		return "", err
	}

	for _, rp := range rps.Items {
		if rp.Status.PodIP == address {
			return RedisPodHostname(rf, rp.ObjectMeta.Name), nil
		}
	}

	return "", fmt.Errorf("no redis pod holds the address %s", address)
}

// GetMasterIP connects to all redis and returns the master of the redis failover
func (r *RedisFailoverChecker) GetMasterIP(rf *redisfailoverv1.RedisFailover) (string, error) {
	rips, err := r.GetRedisesIPs(rf)
	if err != nil {
		return "", err
	}

	password, err := k8s.GetRedisPassword(r.k8sService, rf)
	if err != nil {
		return "", err
	}

	masters := []string{}
	rport := rf.Spec.Redis.Port.ToString()
	for _, rip := range rips {
		master, err := r.redisClient.IsMaster(rip, rport, password)
		if err != nil {
			r.logger.Errorf("Get redis info failed, maybe this node is not ready, pod ip: %s", rip)
			continue
		}
		if master {
			masters = append(masters, rip)
		}
	}

	if len(masters) != 1 {
		return "", errors.New("number of redis nodes known as master is different than 1")
	}
	return masters[0], nil
}

// GetNumberMasters returns the number of redis nodes that are working as a master
func (r *RedisFailoverChecker) GetNumberMasters(rf *redisfailoverv1.RedisFailover) (int, error) {
	nMasters := 0
	rips, err := r.GetRedisesIPs(rf)
	if err != nil {
		r.logger.Errorf(err.Error())
		return nMasters, err
	}

	password, err := k8s.GetRedisPassword(r.k8sService, rf)
	if err != nil {
		r.logger.Errorf("Error getting password: %s", err.Error())
		return nMasters, err
	}

	rport := rf.Spec.Redis.Port.ToString()
	// A node that could not be asked is unknown, not answered. It may be the
	// master, and the caller promotes a node on the strength of this count
	// reaching zero, so reporting a node we never reached as simply "not a
	// master" is how a running master gets replaced by an arbitrary replica.
	//
	// A refused credential is kept apart from every other fault because it is
	// the one the caller can repair, by restarting the pods onto the password
	// the failover is configured with.
	var authErr, unreachableErr error
	for _, rip := range rips {
		master, err := r.redisClient.IsMaster(rip, rport, password)
		if err != nil {
			if redis.IsAuthError(err) {
				authErr = err
			} else if unreachableErr == nil {
				unreachableErr = err
			}
			r.logger.Errorf("Get redis info failed, maybe this node is not ready, pod ip: %s", rip)
			continue
		}
		if master {
			nMasters++
		}
	}

	// Both only matter when nothing answered as a master. A reachable master
	// makes the rest moot: there is no recovery to hold back, and while a
	// password is being applied some pods hold the new one and some the old.
	//
	// The refusal goes first when both happened, since it is the actionable one.
	if nMasters == 0 {
		if authErr != nil {
			return nMasters, authErr
		}
		if unreachableErr != nil {
			return nMasters, unreachableErr
		}
	}

	return nMasters, nil
}

// GetRedisesIPs returns the IPs of the Redis nodes
func (r *RedisFailoverChecker) GetRedisesIPs(rf *redisfailoverv1.RedisFailover) ([]string, error) {
	redises := []string{}
	rps, err := r.k8sService.GetStatefulSetPods(rf.Namespace, GetRedisName(rf))
	if err != nil {
		return nil, err
	}
	for _, rp := range rps.Items {
		if rp.Status.Phase == corev1.PodRunning && rp.DeletionTimestamp == nil { // Only work with running pods
			redises = append(redises, rp.Status.PodIP)
		}
	}
	return redises, nil
}

// GetSentinelRememberedMaster returns the master the Sentinels still hold,
// which is the last one they elected.
//
// After every Redis restarts there is no master to find by asking the Redis
// themselves: each comes back replicating from localhost, because that is what
// its generated configuration says. Sentinel decided which node was master
// while the failover was running and wrote it down, so where that record
// survives it is the answer, and it is the answer from the component this
// operator defers to rather than a guess made in its absence.
//
// Empty when the Sentinels have nothing to say: none reachable, none agreeing,
// or all of them back to watching localhost because they kept nothing. The
// caller falls back to seeding without a preference.
func (r *RedisFailoverChecker) GetSentinelRememberedMaster(rf *redisfailoverv1.RedisFailover) (string, error) {
	sentinels, err := r.GetSentinelsIPs(rf)
	if err != nil {
		return "", err
	}

	port := rf.Spec.Sentinel.Port.ToString()
	remembered := ""
	for _, sip := range sentinels {
		host, _, err := r.redisClient.GetSentinelMonitor(sip, port)
		if err != nil {
			r.logger.Debugf("sentinel %s could not be asked what it monitors: %v", sip, err)
			continue
		}
		if toldNoMaster(host) {
			continue
		}
		if remembered == "" {
			remembered = host
			continue
		}
		if remembered != host {
			// Two Sentinels naming different masters is not a record to act
			// on. Saying nothing leaves the caller where it was.
			r.logger.Infof("sentinels disagree on the last master, %s and %s, so neither is used", remembered, host)
			return "", nil
		}
	}
	return remembered, nil
}

func (r *RedisFailoverChecker) getSentinelPods(rf *redisfailoverv1.RedisFailover) (*corev1.PodList, error) {
	return r.k8sService.GetStatefulSetPods(rf.Namespace, GetSentinelName(rf))
}

// GetSentinelsIPs returns the IPs of the Sentinel nodes
func (r *RedisFailoverChecker) GetSentinelsIPs(rf *redisfailoverv1.RedisFailover) ([]string, error) {
	sentinels := []string{}
	rps, err := r.getSentinelPods(rf)
	if err != nil {
		return nil, err
	}
	for _, sp := range rps.Items {
		if sp.Status.Phase == corev1.PodRunning && sp.DeletionTimestamp == nil { // Only work with running pods
			sentinels = append(sentinels, sp.Status.PodIP)
		}
	}
	return sentinels, nil
}

// GetMaxRedisPodTime returns the MAX uptime among the active Pods
func (r *RedisFailoverChecker) GetMaxRedisPodTime(rf *redisfailoverv1.RedisFailover) (time.Duration, error) {
	maxTime := 0 * time.Hour
	rps, err := r.k8sService.GetStatefulSetPods(rf.Namespace, GetRedisName(rf))
	if err != nil {
		return maxTime, err
	}
	for _, redisNode := range rps.Items {
		if redisNode.Status.StartTime == nil {
			continue
		}
		start := redisNode.Status.StartTime.Round(time.Second)
		alive := time.Since(start)
		r.logger.Debugf("Pod %s has been alive for %.f seconds", redisNode.Status.PodIP, alive.Seconds())
		if alive > maxTime {
			maxTime = alive
		}
	}
	return maxTime, nil
}

// GetRedisesPodsWithStalePassword returns the names of the running Redis pods
// that were built for a different password than the failover is now configured
// with, and so are still serving the old one.
//
// The pods carry a digest of their password, inherited from the StatefulSet's
// pod template. Asking them directly is deliberate: every other pod lookup here
// decides what it needs by querying Redis, which is unavailable precisely when
// Redis is refusing the operator's credential, and comparing pod revisions
// instead would depend on the StatefulSet having already been updated this pass.
func (r *RedisFailoverChecker) GetRedisesPodsWithStalePassword(rf *redisfailoverv1.RedisFailover) ([]string, error) {
	password, err := k8s.GetRedisPassword(r.k8sService, rf)
	if err != nil {
		return nil, err
	}

	pods, err := redisPodsInService(r.k8sService, rf)
	if err != nil {
		return nil, err
	}

	current := redisPasswordChecksum(rf, password)
	stale := []string{}
	for _, pod := range pods {
		if !redisPodBuiltForPassword(pod, current) {
			stale = append(stale, pod.Name)
		}
	}

	return stale, nil
}

// GetRedisesSlavesPods returns pods names of the Redis slave nodes
func (r *RedisFailoverChecker) GetRedisesSlavesPods(rf *redisfailoverv1.RedisFailover) ([]string, error) {
	redises := []string{}
	rps, err := r.k8sService.GetStatefulSetPods(rf.Namespace, GetRedisName(rf))
	if err != nil {
		return nil, err
	}

	password, err := k8s.GetRedisPassword(r.k8sService, rf)
	if err != nil {
		return redises, err
	}

	rport := rf.Spec.Redis.Port.ToString()
	for _, rp := range rps.Items {
		if rp.Status.Phase == corev1.PodRunning && rp.DeletionTimestamp == nil { // Only work with running
			master, err := r.redisClient.IsMaster(rp.Status.PodIP, rport, password)
			if err != nil {
				return []string{}, err
			}
			if !master {
				redises = append(redises, rp.ObjectMeta.Name)
			}
		}
	}
	return redises, nil
}

// GetRedisesMasterPod returns pods names of the Redis slave nodes
func (r *RedisFailoverChecker) GetRedisesMasterPod(rFailover *redisfailoverv1.RedisFailover) (string, error) {
	rps, err := r.k8sService.GetStatefulSetPods(rFailover.Namespace, GetRedisName(rFailover))
	if err != nil {
		return "", err
	}

	password, err := k8s.GetRedisPassword(r.k8sService, rFailover)
	if err != nil {
		return "", err
	}

	rport := rFailover.Spec.Redis.Port.ToString()
	for _, rp := range rps.Items {
		if rp.Status.Phase == corev1.PodRunning && rp.DeletionTimestamp == nil { // Only work with running
			master, err := r.redisClient.IsMaster(rp.Status.PodIP, rport, password)
			if err != nil {
				return "", err
			}
			if master {
				return rp.ObjectMeta.Name, nil
			}
		}
	}
	return "", errors.New("redis nodes known as master not found")
}

// GetRedisesPodsWaitingOnFilesystemResize names the Redis pods that must
// restart before their filesystem grows to match their claim.
func (r *RedisFailoverChecker) GetRedisesPodsWaitingOnFilesystemResize(rFailover *redisfailoverv1.RedisFailover) (map[string]bool, error) {
	return r.k8sService.PodsWaitingOnFilesystemResize(rFailover.Namespace, GetRedisName(rFailover))
}

// GetStatefulSetUpdateRevision returns current version for the statefulSet
// If the label don't exists, we return an empty value and no error, so previous versions don't break
func (r *RedisFailoverChecker) GetStatefulSetUpdateRevision(rFailover *redisfailoverv1.RedisFailover) (string, error) {
	ss, err := r.k8sService.GetStatefulSet(rFailover.Namespace, GetRedisName(rFailover))
	if err != nil {
		return "", err
	}

	if ss == nil {
		return "", errors.New("statefulSet not found")
	}

	return ss.Status.UpdateRevision, nil
}

// GetRedisRevisionHash returns the statefulset uid for the pod
func (r *RedisFailoverChecker) GetRedisRevisionHash(podName string, rFailover *redisfailoverv1.RedisFailover) (string, error) {
	pod, err := r.k8sService.GetPod(rFailover.Namespace, podName)
	if err != nil {
		return "", err
	}

	if pod == nil {
		return "", errors.New("pod not found")
	}

	if pod.ObjectMeta.Labels == nil {
		return "", errors.New("labels not found")
	}

	val := pod.ObjectMeta.Labels[appsv1.ControllerRevisionHashLabelKey]

	return val, nil
}

// CheckRedisSlavesReady returns true if the slave is ready (sync, connected, etc)
func (r *RedisFailoverChecker) CheckRedisSlavesReady(ip string, rFailover *redisfailoverv1.RedisFailover) (bool, error) {
	password, err := k8s.GetRedisPassword(r.k8sService, rFailover)
	if err != nil {
		return false, err
	}

	port := rFailover.Spec.Redis.Port.ToString()
	return r.redisClient.SlaveIsReady(ip, port, password)
}

// IsRedisRunning returns true if all the pods are Running
func (r *RedisFailoverChecker) IsRedisRunning(rFailover *redisfailoverv1.RedisFailover) bool {
	dp, err := r.k8sService.GetStatefulSetPods(rFailover.Namespace, GetRedisName(rFailover))
	return err == nil && len(dp.Items) > int(rFailover.Spec.Redis.Replicas-1) && AreAllRunning(dp, int(rFailover.Spec.Redis.Replicas))
}

// IsSentinelRunning returns true if all the pods are Running
func (r *RedisFailoverChecker) IsSentinelRunning(rFailover *redisfailoverv1.RedisFailover) bool {
	dp, err := r.getSentinelPods(rFailover)
	return err == nil && len(dp.Items) > int(rFailover.Spec.Sentinel.Replicas-1) && AreAllRunning(dp, int(rFailover.Spec.Sentinel.Replicas))
}

// IsHAProxyRunning returns true if all the pods are Running
func (r *RedisFailoverChecker) IsHAProxyRunning(rFailover *redisfailoverv1.RedisFailover) bool {
	if rFailover.Spec.Haproxy == nil {
		return true
	}
	haproxyName := GetHaproxyMasterName(rFailover)
	dp, err := r.k8sService.GetDeploymentPods(rFailover.Namespace, haproxyName)
	return err == nil && len(dp.Items) > int(rFailover.Spec.Haproxy.Replicas-1) && AreAllRunning(dp, int(rFailover.Spec.Haproxy.Replicas))
}

// IsClusterRunning returns true if all the pods in the given RedisFailover are running.
func (r *RedisFailoverChecker) IsClusterRunning(rf *redisfailoverv1.RedisFailover) bool {
	if rf.Bootstrapping() {
		if !rf.SentinelsAllowed() {
			return r.IsRedisRunning(rf)
		}
		return r.IsRedisRunning(rf) && r.IsSentinelRunning(rf)
	}

	// In normal mode, Redis, Sentinel, and HAProxy must all be running
	return r.IsRedisRunning(rf) &&
		r.IsSentinelRunning(rf) &&
		r.IsHAProxyRunning(rf)
}

func AreAllRunning(pods *corev1.PodList, expectedRunningPods int) bool {
	var runningPods int
	for _, pod := range pods.Items {
		if util.PodIsScheduling(&pod) {
			return false
		}
		if util.PodIsTerminal(&pod) {
			continue
		}
		runningPods++
	}
	return runningPods >= expectedRunningPods
}

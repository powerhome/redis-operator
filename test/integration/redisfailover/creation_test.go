//go:build integration
// +build integration

package redisfailover_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	rediscli "github.com/go-redis/redis/v8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/util/homedir"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	redisfailoverclientset "github.com/spotahome/redis-operator/client/k8s/clientset/versioned"
	"github.com/spotahome/redis-operator/cmd/utils"
	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	"github.com/spotahome/redis-operator/operator/redisfailover"
	"github.com/spotahome/redis-operator/service/k8s"
	"github.com/spotahome/redis-operator/service/redis"
)

const (
	name           = "testing"
	namespace      = "rf-integration-tests"
	redisSize      = int32(3)
	sentinelSize   = int32(3)
	haproxySize    = int32(1)
	authSecretPath = "redis-auth"
	testPass       = "test-pass"
	rotatedPass    = "rotated-pass"
	redisAddr      = "redis://127.0.0.1:6379"

	// Every wait in this file samples a condition on this interval. The operator
	// reconciles on its own timer and Kubernetes reports readiness
	// asynchronously, so there is no moment at which the cluster can be assumed
	// settled; the only way to know is to look.
	pollInterval = 2 * time.Second

	// How long a wait gives the cluster before reporting what it was still
	// doing. Generous, because it costs nothing when the condition is met
	// sooner and a runner under load is slower than a workstation.
	readyTimeout = 5 * time.Minute
)

type clients struct {
	k8sClient   kubernetes.Interface
	rfClient    redisfailoverclientset.Interface
	aeClient    apiextensionsclientset.Interface
	redisClient redis.Client
}

func (c *clients) prepareNS() error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	_, err := c.k8sClient.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	return err
}

func (c *clients) cleanup(stopC chan struct{}) {
	c.k8sClient.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	close(stopC)
}

// waitFor samples condition until it holds, and returns the reason it did not
// hold when the timeout runs out.
//
// The returned error is what makes this worth writing rather than reaching for
// assert.Eventually: a wait that fails in CI has to say what the cluster was
// still doing, because nobody can inspect the cluster afterwards. "redis
// statefulset has 2 of 3 replicas ready" and "no HAProxy pod has been assigned
// an address yet" send a reader to different places.
func waitFor(timeout time.Duration, condition func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		ok, err := condition()
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			if err == nil {
				err = errors.New("condition still did not hold when the deadline passed")
			}
			return err
		}
		time.Sleep(pollInterval)
	}
}

// waitForNamespace waits for the namespace the test works in to be usable.
func (c *clients) waitForNamespace(t *testing.T) {
	t.Helper()

	err := waitFor(readyTimeout, func() (bool, error) {
		ns, err := c.k8sClient.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if ns.Status.Phase != corev1.NamespaceActive {
			return false, fmt.Errorf("namespace is %s", ns.Status.Phase)
		}
		return true, nil
	})
	require.New(t).NoError(err, "the test namespace should become active")
}

// waitForWorkloadsReady waits until the operator has built every workload a
// RedisFailover owns and each one reports all of its replicas ready.
//
// This is the gate the assertions below depend on, and it deliberately stops at
// what Kubernetes reports. Whether Sentinel elected a single master, whether the
// sentinels agree on it, and whether HAProxy routes to it are the questions those
// assertions exist to answer, so each of them waits for its own condition rather
// than having it guaranteed here.
func (c *clients) waitForWorkloadsReady(t *testing.T) {
	t.Helper()

	err := waitFor(readyTimeout, func() (bool, error) {
		redisSS, err := c.k8sClient.AppsV1().StatefulSets(namespace).Get(context.Background(), fmt.Sprintf("rfr-%s", name), metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("redis statefulset: %w", err)
		}
		if redisSS.Status.ReadyReplicas != redisSize {
			return false, fmt.Errorf("redis statefulset has %d of %d replicas ready", redisSS.Status.ReadyReplicas, redisSize)
		}

		sentinelSS, err := c.k8sClient.AppsV1().StatefulSets(namespace).Get(context.Background(), fmt.Sprintf("rfs-%s", name), metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("sentinel statefulset: %w", err)
		}
		if sentinelSS.Status.ReadyReplicas != sentinelSize {
			return false, fmt.Errorf("sentinel statefulset has %d of %d replicas ready", sentinelSS.Status.ReadyReplicas, sentinelSize)
		}

		haproxyD, err := c.k8sClient.AppsV1().Deployments(namespace).Get(context.Background(), fmt.Sprintf("rfrm-haproxy-%s", name), metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("haproxy deployment: %w", err)
		}
		if haproxyD.Status.ReadyReplicas != haproxySize {
			return false, fmt.Errorf("haproxy deployment has %d of %d replicas ready", haproxyD.Status.ReadyReplicas, haproxySize)
		}

		return true, nil
	})
	require.New(t).NoError(err, "the operator should bring up every workload the RedisFailover owns")
}

func TestRedisFailover(t *testing.T) {
	require := require.New(t)

	// Create signal channels.
	stopC := make(chan struct{})
	errC := make(chan error)

	flags := &utils.CMDFlags{
		KubeConfig:  filepath.Join(homedir.HomeDir(), ".kube", "config"),
		Development: true,
	}

	// Kubernetes clients.
	k8sClient, customClient, aeClientset, err := utils.CreateKubernetesClients(flags)
	require.NoError(err)

	// Create the redis clients
	redisClient := redis.New(metrics.Dummy)

	clients := clients{
		k8sClient:   k8sClient,
		rfClient:    customClient,
		aeClient:    aeClientset,
		redisClient: redisClient,
	}

	// Create kubernetes service.
	k8sservice := k8s.New(k8sClient, customClient, aeClientset, log.Dummy, metrics.Dummy)

	// Prepare namespace
	prepErr := clients.prepareNS()
	require.NoError(prepErr)

	clients.waitForNamespace(t)

	// Create operator and run.
	redisfailoverOperator, err := redisfailover.New(redisfailover.Config{}, k8sservice, k8sClient, namespace, redisClient, metrics.Dummy, log.Dummy)
	require.NoError(err)

	go func() {
		errC <- redisfailoverOperator.Run(context.Background())
	}()

	// Prepare cleanup for when the test ends
	defer clients.cleanup(stopC)

	// Create secret
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      authSecretPath,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"password": []byte(testPass),
		},
	}
	_, err = k8sClient.CoreV1().Secrets(namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	require.NoError(err)

	// Check that if we create a RedisFailover, it is certainly created and we can get it
	ok := t.Run("Check Custom Resource Creation", clients.testCRCreation)
	require.True(ok, "the custom resource has to be created to continue")

	// Wait for the operator to build the failover. Nothing below this point
	// depends on how long that takes, only on it having happened, and the
	// operator has no reason to take the same time twice: it starts when it
	// starts, images may or may not be cached, and a loaded runner schedules
	// pods when it gets to them.
	clients.waitForWorkloadsReady(t)

	// Verify that auth is set and actually working
	t.Run("Check that auth is set in sentinel and redis configs", clients.testAuth)

	// Check custom config is set
	t.Run("Check that custom config is behave expected", clients.testCustomConfig)

	// Check that a Redis Statefulset is created and the size of it is the one defined by the
	// Redis Failover definition created before.
	t.Run("Check Redis Statefulset existing and size", clients.testRedisStatefulSet)

	t.Run("Check Sentinel Statefulset existing and size", clients.testSentinelStatefulSet)

	// Connect to all the Redis pods and, asking to the Redis running inside them, check
	// that only one of them is the master of the failover.
	t.Run("Check Only One Redis Master", clients.testRedisMaster)

	// Connect to all the Sentinel pods and, asking to the Sentinel running inside them,
	// check that all of them are connected to the same Redis node, and also that that node
	// is the master.
	t.Run("Check Sentinels Checking the Redis Master", clients.testSentinelMonitoring)

	// Check that an HAProxy Deployment is created and the size of it is the one
	// defined by the Redis Failover definition created before.
	t.Run("Check HAProxy Deployment existing and size", clients.testHaproxyDeployment)

	// Reach the master through HAProxy, which only routes once its health check
	// has authenticated against a password-protected Redis.
	t.Run("Check HAProxy Routing To The Redis Master", clients.testHaproxyMaster)

	// Change the password and check the operator applies it without help. These
	// run last because each one restarts every Redis pod, and they run in
	// sequence because each starts from where the previous one left the
	// failover.
	//
	// Each one finishes by reaching a master through HAProxy on the password then
	// in force, not only through the Redis pods. The proxy keeps the password its
	// own pod started with, so the operator holds the Deployment write back until
	// every Redis agrees with the configured password. A proxy restarted ahead of
	// its backends can authenticate against none of them and routes nowhere,
	// while the resources it is generated from read as correct the whole time.
	t.Run("Check Rotating The Password Is Applied", clients.testPasswordRotation)
	t.Run("Check Removing The Password Is Applied", clients.testPasswordRemoval)
	t.Run("Check Adding The Password Back Is Applied", clients.testPasswordAddition)
}

const sentinelPort = 26379

func (c *clients) testCRCreation(t *testing.T) {
	assert := assert.New(t)
	toCreate := &redisfailoverv1.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: redisfailoverv1.RedisFailoverSpec{
			Redis: redisfailoverv1.RedisSettings{
				Replicas: redisSize,
				Exporter: redisfailoverv1.Exporter{
					Enabled: true,
				},
				CustomConfig: []string{`save ""`},
			},
			Sentinel: redisfailoverv1.SentinelSettings{
				Replicas: sentinelSize,
				Port:     redisfailoverv1.Port(sentinelPort),
			},
			Haproxy: &redisfailoverv1.HaproxySettings{
				Replicas: haproxySize,
			},
			Auth: redisfailoverv1.AuthSettings{
				SecretPath: authSecretPath,
			},
		},
	}

	c.rfClient.DatabasesV1().RedisFailovers(namespace).Create(context.Background(), toCreate, metav1.CreateOptions{})
	gotRF, err := c.rfClient.DatabasesV1().RedisFailovers(namespace).Get(context.Background(), name, metav1.GetOptions{})

	assert.NoError(err)
	assert.Equal(toCreate.Spec, gotRF.Spec)
}

func (c *clients) testRedisStatefulSet(t *testing.T) {
	assert := assert.New(t)
	redisSS, err := c.k8sClient.AppsV1().StatefulSets(namespace).Get(context.Background(), fmt.Sprintf("rfr-%s", name), metav1.GetOptions{})
	assert.NoError(err)
	assert.Equal(redisSize, int32(redisSS.Status.Replicas))
}

func (c *clients) testSentinelStatefulSet(t *testing.T) {
	assert := assert.New(t)
	sentinelSS, err := c.k8sClient.AppsV1().StatefulSets(namespace).Get(context.Background(), fmt.Sprintf("rfs-%s", name), metav1.GetOptions{})
	assert.NoError(err)
	assert.Equal(3, int(sentinelSS.Status.Replicas))
}

// testRedisMaster asks every Redis pod who it is and expects exactly one to
// answer that it is the master.
//
// Sentinel elects the master, and it does so once the pods are up rather than as
// they come up, so this waits for the election instead of reading the roles the
// instant every pod is ready. Two masters is a split brain and no master is a
// failover nothing can write to; both are real failures, and both look like "not
// yet" for a short window after the pods arrive.
func (c *clients) testRedisMaster(t *testing.T) {
	assert := assert.New(t)

	var masters []string
	err := waitFor(readyTimeout, func() (bool, error) {
		redisSS, err := c.k8sClient.AppsV1().StatefulSets(namespace).Get(context.Background(), fmt.Sprintf("rfr-%s", name), metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		listOptions := metav1.ListOptions{
			LabelSelector: labels.FormatLabels(redisSS.Spec.Selector.MatchLabels),
		}
		redisPodList, err := c.k8sClient.CoreV1().Pods(namespace).List(context.Background(), listOptions)
		if err != nil {
			return false, err
		}

		masters = nil
		for _, pod := range redisPodList.Items {
			ip := pod.Status.PodIP
			if ok, _ := c.redisClient.IsMaster(ip, "6379", testPass); ok {
				masters = append(masters, ip)
			}
		}
		if len(masters) != 1 {
			return false, fmt.Errorf("%d of %d Redis pods report being the master", len(masters), len(redisPodList.Items))
		}
		return true, nil
	})

	assert.NoError(err, "the failover should elect exactly one master")
	assert.Len(masters, 1, "only one master expected")
}

// testSentinelMonitoring asks every Sentinel which Redis it is monitoring and
// expects them all to name the same one, and that one to be the master.
//
// Sentinels discover each other and agree among themselves, so a disagreement
// here is either a transient mid-election reading or a real split in the quorum.
// Waiting tells the two apart: the transient one resolves, and the real one is
// still there when the deadline passes, reported as the disagreement it is.
func (c *clients) testSentinelMonitoring(t *testing.T) {
	assert := assert.New(t)

	var monitored string
	err := waitFor(readyTimeout, func() (bool, error) {
		sentinelSS, err := c.k8sClient.AppsV1().StatefulSets(namespace).Get(context.Background(), fmt.Sprintf("rfs-%s", name), metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		listOptions := metav1.ListOptions{
			LabelSelector: labels.FormatLabels(sentinelSS.Spec.Selector.MatchLabels),
		}
		sentinelPodList, err := c.k8sClient.CoreV1().Pods(namespace).List(context.Background(), listOptions)
		if err != nil {
			return false, err
		}
		if len(sentinelPodList.Items) == 0 {
			return false, errors.New("no Sentinel pods to ask")
		}

		port := strconv.FormatInt(int64(sentinelPort), 10)
		monitored = ""
		for _, pod := range sentinelPodList.Items {
			master, _, err := c.redisClient.GetSentinelMonitor(pod.Status.PodIP, port)
			if err != nil {
				return false, fmt.Errorf("asking Sentinel %s what it monitors: %w", pod.Status.PodIP, err)
			}
			if monitored == "" {
				monitored = master
				continue
			}
			if master != monitored {
				return false, fmt.Errorf("Sentinels disagree on the master: %s and %s", monitored, master)
			}
		}
		if monitored == "" {
			return false, errors.New("the Sentinels are not monitoring anything yet")
		}

		// Sentinel answers with a name, which is the point: it is what it was
		// told, and what it reports to a client. An address here would mean
		// either that it was given one or that it is not announcing hostnames.
		if net.ParseIP(monitored) != nil {
			return false, fmt.Errorf("the Sentinels monitor the address %s rather than a name", monitored)
		}
		// That name answers inside the cluster and not out here, so the pod it
		// names is found through the Kubernetes API and asked at its address.
		podName, _, isPodName := strings.Cut(monitored, ".")
		if !isPodName {
			return false, fmt.Errorf("the Sentinels monitor %q, which is not a Redis pod's name in DNS", monitored)
		}
		masterPod, err := c.k8sClient.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("finding the pod the Sentinels monitor, %s: %w", podName, err)
		}

		isMaster, err := c.redisClient.IsMaster(masterPod.Status.PodIP, "6379", testPass)
		if err != nil {
			return false, fmt.Errorf("asking %s whether it is the master: %w", monitored, err)
		}
		if !isMaster {
			return false, fmt.Errorf("the Sentinels monitor %s, which does not report being the master", monitored)
		}
		return true, nil
	})

	assert.NoError(err, "every Sentinel should monitor the same Redis, and that Redis should be the master")
	assert.NotEmpty(monitored, "Sentinel should monitor the Redis master")
}

func (c *clients) testHaproxyDeployment(t *testing.T) {
	assert := assert.New(t)
	haproxyD, err := c.k8sClient.AppsV1().Deployments(namespace).Get(context.Background(), fmt.Sprintf("rfrm-haproxy-%s", name), metav1.GetOptions{})
	assert.NoError(err)
	assert.Equal(haproxySize, int32(haproxyD.Status.Replicas))
}

func (c *clients) testHaproxyMaster(t *testing.T) {
	c.waitForHaproxyMaster(t, testPass)
}

// waitForHaproxyMaster reaches Redis through the HAProxy master proxy instead of
// through a Redis pod directly. Backends start DOWN and only join the pool once
// the health check gets the reply it expects, so where the failover sets
// requirepass the check has to authenticate before Redis will answer it. An
// unauthenticated check leaves HAProxy with an empty pool and nothing to route
// to, which is why this asserts on reaching a master rather than on the text of
// the generated config.
//
// An empty password means the failover is expected to be running without
// authentication, and the proxy is expected to have given its own password up in
// step with Redis.
func (c *clients) waitForHaproxyMaster(t *testing.T, password string) {
	t.Helper()
	assert := assert.New(t)

	haproxyD, err := c.k8sClient.AppsV1().Deployments(namespace).Get(context.Background(), fmt.Sprintf("rfrm-haproxy-%s", name), metav1.GetOptions{})
	if !assert.NoError(err) {
		return
	}
	listOptions := metav1.ListOptions{
		LabelSelector: labels.FormatLabels(haproxyD.Spec.Selector.MatchLabels),
	}

	// Backends are discovered through SRV records and checked once a second, so
	// wait for the pool to come up rather than reading it the instant the pod is
	// scheduled. The pod is looked up each time round: one that has not been
	// assigned an address yet, or one replaced while this waits, would otherwise
	// leave every attempt dialling an address that can never answer, and report
	// it as an authentication failure.
	var isMaster bool
	waitErr := waitFor(readyTimeout, func() (bool, error) {
		haproxyPods, err := c.k8sClient.CoreV1().Pods(namespace).List(context.Background(), listOptions)
		if err != nil {
			return false, err
		}

		address := ""
		for _, pod := range haproxyPods.Items {
			if pod.Status.PodIP != "" {
				address = pod.Status.PodIP
				break
			}
		}
		if address == "" {
			return false, errors.New("no HAProxy pod has been assigned an address yet")
		}

		isMaster, err = c.redisClient.IsMaster(address, "6379", password)
		if err != nil {
			return false, err
		}
		if !isMaster {
			return false, fmt.Errorf("HAProxy at %s routes to a Redis that does not report being the master", address)
		}
		return true, nil
	})

	assert.NoError(waitErr, "HAProxy should have a Redis backend to route to; without an authenticated health check every backend stays DOWN")
	assert.True(isMaster, "HAProxy should route to the Redis master")
}

// waitForFailoverPassword polls until every Redis pod answers on the given
// password and one of them is the master. An empty password means the failover
// is expected to be running without authentication.
//
// It samples the whole transition rather than the end state, which matters: the
// operator restarts the pods, sentinel elects a master, and the operator
// restores the role labels. An assertion over the generated resources passes
// just as well while the pods are still serving the previous password.
func (c *clients) waitForFailoverPassword(t *testing.T, password string) {
	t.Helper()
	assert := assert.New(t)

	redisSS, err := c.k8sClient.AppsV1().StatefulSets(namespace).Get(context.Background(), fmt.Sprintf("rfr-%s", name), metav1.GetOptions{})
	if !assert.NoError(err) {
		return
	}
	listOptions := metav1.ListOptions{LabelSelector: labels.FormatLabels(redisSS.Spec.Selector.MatchLabels)}

	var master string
	waitErr := waitFor(readyTimeout, func() (bool, error) {
		pods, err := c.k8sClient.CoreV1().Pods(namespace).List(context.Background(), listOptions)
		if err != nil {
			return false, err
		}

		master = ""
		accepted := 0
		for _, pod := range pods.Items {
			if pod.Status.PodIP == "" {
				continue
			}
			isMaster, err := c.redisClient.IsMaster(pod.Status.PodIP, "6379", password)
			if err != nil {
				return false, fmt.Errorf("Redis %s does not accept the configured password: %w", pod.Status.PodIP, err)
			}
			accepted++
			if isMaster {
				master = pod.Status.PodIP
			}
		}

		if accepted != int(redisSize) {
			return false, fmt.Errorf("%d of %d Redis pods accept the configured password", accepted, redisSize)
		}
		if master == "" {
			return false, errors.New("no Redis reports being the master")
		}
		return true, nil
	})

	assert.NoError(waitErr, "every Redis pod should accept the configured password once the operator has applied it")
	assert.NotEmpty(master, "the failover should have a master again after the change")
}

// setSecretPassword writes a new value into the secret the failover names.
func (c *clients) setSecretPassword(t *testing.T, password string) bool {
	t.Helper()
	assert := assert.New(t)

	secret, err := c.k8sClient.CoreV1().Secrets(namespace).Get(context.Background(), authSecretPath, metav1.GetOptions{})
	if !assert.NoError(err) {
		return false
	}
	secret.Data = map[string][]byte{"password": []byte(password)}
	_, err = c.k8sClient.CoreV1().Secrets(namespace).Update(context.Background(), secret, metav1.UpdateOptions{})
	return assert.NoError(err)
}

// setAuthSecretPath points the failover at a secret, or at none.
func (c *clients) setAuthSecretPath(t *testing.T, secretPath string) bool {
	t.Helper()
	assert := assert.New(t)

	rf, err := c.rfClient.DatabasesV1().RedisFailovers(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if !assert.NoError(err) {
		return false
	}
	rf.Spec.Auth.SecretPath = secretPath
	_, err = c.rfClient.DatabasesV1().RedisFailovers(namespace).Update(context.Background(), rf, metav1.UpdateOptions{})
	return assert.NoError(err)
}

// testPasswordRotation changes the value in the secret the failover names.
//
// Redis reads requirepass only at startup and the StatefulSet uses the OnDelete
// update strategy, so nothing restarts the pods on their own; the operator has
// to notice that Redis is refusing the configured password and restart them.
func (c *clients) testPasswordRotation(t *testing.T) {
	if !c.setSecretPassword(t, rotatedPass) {
		return
	}
	c.waitForFailoverPassword(t, rotatedPass)
	c.waitForHaproxyMaster(t, rotatedPass)
}

// testPasswordRemoval takes auth.secretPath away from a running failover.
//
// The pods keep requirepass until they restart, so the operator has to apply
// this exactly as it applies a rotation. HAProxy has to follow Redis rather
// than lead it here too: a proxy that has given up the password while its
// backends still demand one fails every health check.
func (c *clients) testPasswordRemoval(t *testing.T) {
	if !c.setAuthSecretPath(t, "") {
		return
	}
	c.waitForFailoverPassword(t, "")
	c.waitForHaproxyMaster(t, "")
}

// testPasswordAddition points a running failover at a secret again.
//
// This is the case an operator hits first, and the one Redis answers with a
// complaint that no password is configured rather than with WRONGPASS.
func (c *clients) testPasswordAddition(t *testing.T) {
	if !c.setAuthSecretPath(t, authSecretPath) {
		return
	}
	c.waitForFailoverPassword(t, rotatedPass)
	c.waitForHaproxyMaster(t, rotatedPass)
}

func (c *clients) testAuth(t *testing.T) {
	assert := assert.New(t)

	redisCfg, err := c.k8sClient.CoreV1().ConfigMaps(namespace).Get(context.Background(), fmt.Sprintf("rfr-%s", name), metav1.GetOptions{})
	assert.NoError(err)
	assert.Contains(redisCfg.Data["redis.conf"], "requirepass "+testPass)
	assert.Contains(redisCfg.Data["redis.conf"], "masterauth "+testPass)

	redisSS, err := c.k8sClient.AppsV1().StatefulSets(namespace).Get(context.Background(), fmt.Sprintf("rfr-%s", name), metav1.GetOptions{})
	assert.NoError(err)

	assert.Len(redisSS.Spec.Template.Spec.Containers, 2)
	assert.Equal(redisSS.Spec.Template.Spec.Containers[1].Env[1].Name, "REDIS_ADDR")
	assert.Equal(redisSS.Spec.Template.Spec.Containers[1].Env[1].Value, redisAddr)
	assert.Equal(redisSS.Spec.Template.Spec.Containers[1].Env[2].Name, "REDIS_PORT")
	assert.Equal(redisSS.Spec.Template.Spec.Containers[1].Env[2].Value, "6379")
	assert.Equal(redisSS.Spec.Template.Spec.Containers[1].Env[3].Name, "REDIS_USER")
	assert.Equal(redisSS.Spec.Template.Spec.Containers[1].Env[3].Value, "default")
	assert.Equal(redisSS.Spec.Template.Spec.Containers[1].Env[4].Name, "REDIS_PASSWORD")
	assert.Equal(redisSS.Spec.Template.Spec.Containers[1].Env[4].ValueFrom.SecretKeyRef.Key, "password")
	assert.Equal(redisSS.Spec.Template.Spec.Containers[1].Env[4].ValueFrom.SecretKeyRef.LocalObjectReference.Name, authSecretPath)
}

func (c *clients) testCustomConfig(t *testing.T) {
	assert := assert.New(t)

	redisSS, err := c.k8sClient.AppsV1().StatefulSets(namespace).Get(context.Background(), fmt.Sprintf("rfr-%s", name), metav1.GetOptions{})
	assert.NoError(err)

	listOptions := metav1.ListOptions{
		LabelSelector: labels.FormatLabels(redisSS.Spec.Selector.MatchLabels),
	}
	redisPodList, err := c.k8sClient.CoreV1().Pods(namespace).List(context.Background(), listOptions)
	assert.NoError(err)

	rClient := rediscli.NewClient(&rediscli.Options{
		Addr:     net.JoinHostPort(redisPodList.Items[0].Status.PodIP, "6379"),
		Password: testPass,
		DB:       0,
	})
	defer rClient.Close()

	result := rClient.ConfigGet(context.TODO(), "save")
	assert.NoError(result.Err())

	values, err := result.Result()
	assert.NoError(err)

	assert.Len(values, 2)
	assert.Empty(values[1])
}

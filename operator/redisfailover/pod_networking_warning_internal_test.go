package redisfailover

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
)

// countingLogger records the warnings it is given, so a test can say how many
// times something was said rather than only that it was said.
type countingLogger struct {
	log.DummyLogger
	mu       sync.Mutex
	warnings []string
}

func (c *countingLogger) Warningf(format string, args ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.warnings = append(c.warnings, fmt.Sprintf(format, args...))
}

func (c *countingLogger) WithField(string, interface{}) log.Logger { return c }

func (c *countingLogger) mentioning(word string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, w := range c.warnings {
		if strings.Contains(w, word) {
			n++
		}
	}
	return n
}

func failoverWithPodNetworking(name string, mutate func(*redisfailoverv1.RedisFailover)) *redisfailoverv1.RedisFailover {
	rf := &redisfailoverv1.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "testns"},
	}
	mutate(rf)
	return rf
}

func TestPodNetworkingDeprecationIsAnnouncedOncePerFailover(t *testing.T) {
	assert := assert.New(t)

	logger := &countingLogger{}
	handler := NewRedisFailoverHandler(Config{}, nil, nil, nil, nil, metrics.Dummy, logger)

	rf := failoverWithPodNetworking("one", func(rf *redisfailoverv1.RedisFailover) {
		rf.Spec.Redis.HostNetwork = true
	})
	for i := 0; i < 5; i++ {
		handler.warnPodNetworkingIsGoingAway(rf)
	}

	assert.Equal(1, logger.mentioning("redis.hostNetwork"),
		"five passes over one failover should say it once")

	// A second failover is a second reader who has not been told.
	handler.warnPodNetworkingIsGoingAway(failoverWithPodNetworking("two", func(rf *redisfailoverv1.RedisFailover) {
		rf.Spec.Redis.HostNetwork = true
	}))
	assert.Equal(2, logger.mentioning("redis.hostNetwork"))
}

func TestPodNetworkingDeprecationNamesEveryFieldThatIsSet(t *testing.T) {
	assert := assert.New(t)

	logger := &countingLogger{}
	handler := NewRedisFailoverHandler(Config{}, nil, nil, nil, nil, metrics.Dummy, logger)

	handler.warnPodNetworkingIsGoingAway(failoverWithPodNetworking("all", func(rf *redisfailoverv1.RedisFailover) {
		rf.Spec.Redis.HostNetwork = true
		rf.Spec.Redis.DNSPolicy = corev1.DNSDefault
		rf.Spec.Sentinel.HostNetwork = true
		rf.Spec.Sentinel.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	}))

	for _, field := range []string{"redis.hostNetwork", "redis.dnsPolicy", "sentinel.hostNetwork", "sentinel.dnsPolicy"} {
		assert.Equal(1, logger.mentioning(field), "the reader has to be told which field to take out: %s", field)
	}
	assert.Equal(1, len(logger.warnings), "one warning naming all of them, not one per field")
}

func TestPodNetworkingDeprecationSaysNothingWhenNoFieldIsSet(t *testing.T) {
	assert := assert.New(t)

	logger := &countingLogger{}
	handler := NewRedisFailoverHandler(Config{}, nil, nil, nil, nil, metrics.Dummy, logger)

	handler.warnPodNetworkingIsGoingAway(failoverWithPodNetworking("plain", func(*redisfailoverv1.RedisFailover) {}))

	assert.Equal(0, logger.mentioning("deprecated"))
}

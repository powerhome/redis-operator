package v1_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
)

func TestUnsupportedReplicaCounts(t *testing.T) {
	tests := []struct {
		name             string
		redisReplicas    int32
		sentinelReplicas int32
		haproxyReplicas  int32
		bootstrapping    bool
		allowSentinels   bool
		expMentions      []string
		expAbsent        []string
		expSilent        bool
	}{
		{
			name:             "the default shape says nothing",
			redisReplicas:    3,
			sentinelReplicas: 3,
			expSilent:        true,
		},
		{
			name:             "two redis and three sentinels says nothing",
			redisReplicas:    2,
			sentinelReplicas: 3,
			expSilent:        true,
		},
		{
			name:             "one redis has nothing to promote",
			redisReplicas:    1,
			sentinelReplicas: 3,
			expMentions:      []string{"redis: ", "redis.replicas is 1", "needs 2 Redis"},
		},
		{
			name:             "two sentinels cannot elect",
			redisReplicas:    3,
			sentinelReplicas: 2,
			expMentions:      []string{"sentinel: ", "sentinel.replicas is 2", "needs 3 Sentinels"},
		},
		{
			name:             "one proxy cannot be evicted without taking the client path",
			redisReplicas:    3,
			sentinelReplicas: 3,
			haproxyReplicas:  1,
			expMentions:      []string{"haproxy: ", "haproxy.replicas is 1", "needs 2 proxies"},
		},
		{
			name:             "two proxies say nothing",
			redisReplicas:    3,
			sentinelReplicas: 3,
			haproxyReplicas:  2,
			expSilent:        true,
		},
		{
			name:             "a failover with no haproxy block is not asked about proxies",
			redisReplicas:    3,
			sentinelReplicas: 3,
			expSilent:        true,
		},
		{
			name:             "both counts are named, each in its own entry",
			redisReplicas:    1,
			sentinelReplicas: 1,
			expMentions:      []string{"redis.replicas is 1", "sentinel.replicas is 1"},
		},
		{
			name:             "a bootstrapping failover runs no proxy and is not asked about one",
			redisReplicas:    3,
			sentinelReplicas: 3,
			haproxyReplicas:  1,
			bootstrapping:    true,
			allowSentinels:   true,
			expSilent:        true,
		},
		{
			name:             "a bootstrapping failover that runs no Sentinels is not asked about them",
			redisReplicas:    3,
			sentinelReplicas: 1,
			bootstrapping:    true,
			expSilent:        true,
		},
		{
			name:             "a bootstrapping failover that runs Sentinels is asked about them",
			redisReplicas:    3,
			sentinelReplicas: 2,
			bootstrapping:    true,
			allowSentinels:   true,
			expMentions:      []string{"sentinel.replicas is 2", "needs 3 Sentinels"},
		},
		{
			name:             "a bootstrapping failover follows a master rather than promoting one",
			redisReplicas:    1,
			sentinelReplicas: 3,
			bootstrapping:    true,
			allowSentinels:   true,
			expMentions:      []string{"redis.replicas is 1", "needs 2 Redis", "replicating the node it bootstraps from"},
			expAbsent:        []string{"promoted"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			rf := &redisfailoverv1.RedisFailover{}
			rf.Spec.Redis.Replicas = test.redisReplicas
			rf.Spec.Sentinel.Replicas = test.sentinelReplicas
			if test.haproxyReplicas > 0 {
				rf.Spec.Haproxy = &redisfailoverv1.HaproxySettings{Replicas: test.haproxyReplicas}
			}
			if test.bootstrapping {
				rf.Spec.BootstrapNode = &redisfailoverv1.BootstrapSettings{
					Host:           "10.0.0.1",
					Enabled:        true,
					AllowSentinels: test.allowSentinels,
				}
			}

			unsupported := rf.UnsupportedReplicaCounts()

			if test.expSilent {
				assert.Empty(unsupported, "a shape that can survive a failure is not worth a word")
				return
			}

			var reasons []string
			for _, count := range unsupported {
				reasons = append(reasons, count.Component+": "+count.Reason)
			}
			joined := strings.Join(reasons, "\n")
			for _, mention := range test.expMentions {
				assert.Contains(joined, mention,
					"the reader is told which count is wrong and what it takes: %s", mention)
			}
			for _, absent := range test.expAbsent {
				assert.NotContains(joined, absent,
					"a reason that describes the wrong mechanism sends the reader after the wrong fix: %s", absent)
			}
		})
	}
}

func TestUnsupportedReplicaCountsReportsEachComponentOnce(t *testing.T) {
	assert := assert.New(t)

	rf := &redisfailoverv1.RedisFailover{}
	rf.Spec.Redis.Replicas = 1
	rf.Spec.Sentinel.Replicas = 1

	assert.Len(rf.UnsupportedReplicaCounts(), 2,
		"one entry per component, so a reader can act on them one at a time")
}

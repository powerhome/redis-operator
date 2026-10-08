package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	mK8SService "github.com/spotahome/redis-operator/mocks/service/k8s"
	mRedisService "github.com/spotahome/redis-operator/mocks/service/redis"
	rfservice "github.com/spotahome/redis-operator/operator/redisfailover/service"
)

// Taking a Sentinel away costs its vote until the operator points the
// replacement at a master, so the question is whether the rest could still
// agree a failover.
//
// At or below the quorum there is no answer that permits it, and a strict
// question would hold those failovers on an old pod template for good, so they
// are asked only that none is already missing.
func TestCheckSentinelsCanSpareOne(t *testing.T) {
	tests := []struct {
		name      string
		replicas  int32
		reporting int32
		losing    int32
		allowed   bool
	}{
		{name: "three, all reporting", replicas: 3, reporting: 3, losing: 1, allowed: true},
		{name: "three, one already missing", replicas: 3, reporting: 2, losing: 1, allowed: false},
		{name: "three, two already missing", replicas: 3, reporting: 1, losing: 1, allowed: false},
		{name: "five, one already missing", replicas: 5, reporting: 4, losing: 1, allowed: true},
		{name: "five, two already missing", replicas: 5, reporting: 3, losing: 1, allowed: false},

		// Quorum is 2 for two Sentinels and 1 for one, so losing either breaks
		// it. Asked the strict question these could never roll.
		{name: "two, both reporting", replicas: 2, reporting: 2, losing: 1, allowed: true},
		{name: "two, one already missing", replicas: 2, reporting: 1, losing: 1, allowed: false},
		{name: "one, reporting", replicas: 1, reporting: 1, losing: 1, allowed: true},
		{name: "one, not reporting", replicas: 1, reporting: 0, losing: 1, allowed: false},

		{name: "three, the stale one already down", replicas: 3, reporting: 2, losing: 0, allowed: true},
		{name: "two, the stale one already down", replicas: 2, reporting: 1, losing: 0, allowed: true},
		{name: "one, the only one already down", replicas: 1, reporting: 0, losing: 0, allowed: true},
		{name: "five, the stale one and another down", replicas: 5, reporting: 3, losing: 0, allowed: true},

		{name: "three, none reporting", replicas: 3, reporting: 0, losing: 0, allowed: false},
		{name: "five, none reporting", replicas: 5, reporting: 0, losing: 0, allowed: false},
		{name: "two, none reporting", replicas: 2, reporting: 0, losing: 0, allowed: false},

		// One Sentinel is the exception: there is no second answer to tell an
		// unreachable Sentinel from a broken one, and holding it would leave the
		// failover on an old pod template for good.
		{name: "one, none reporting", replicas: 1, reporting: 0, losing: 0, allowed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			rf := generateRF()
			rf.Spec.Sentinel.Replicas = test.replicas

			checker := rfservice.NewRedisFailoverChecker(&mK8SService.Services{}, &mRedisService.Client{}, log.DummyLogger{}, metrics.Dummy)

			err := checker.CheckSentinelsCanSpareOne(rf, test.reporting, test.losing)
			if test.allowed {
				assert.NoError(err)
				return
			}
			require.Error(t, err)
			assert.Contains(err.Error(), "report the master",
				"the failure names what it was waiting for")
		})
	}
}

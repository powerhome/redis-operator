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
		allowed   bool
	}{
		{name: "three, all reporting", replicas: 3, reporting: 3, allowed: true},
		{name: "three, one already missing", replicas: 3, reporting: 2, allowed: false},
		{name: "three, two already missing", replicas: 3, reporting: 1, allowed: false},
		{name: "five, one already missing", replicas: 5, reporting: 4, allowed: true},
		{name: "five, two already missing", replicas: 5, reporting: 3, allowed: false},

		// Quorum is 2 for two Sentinels and 1 for one, so losing either breaks
		// it. Asked the strict question these could never roll.
		{name: "two, both reporting", replicas: 2, reporting: 2, allowed: true},
		{name: "two, one already missing", replicas: 2, reporting: 1, allowed: false},
		{name: "one, reporting", replicas: 1, reporting: 1, allowed: true},
		{name: "one, not reporting", replicas: 1, reporting: 0, allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			rf := generateRF()
			rf.Spec.Sentinel.Replicas = test.replicas

			checker := rfservice.NewRedisFailoverChecker(&mK8SService.Services{}, &mRedisService.Client{}, log.DummyLogger{}, metrics.Dummy)

			err := checker.CheckSentinelsCanSpareOne(rf, test.reporting)
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

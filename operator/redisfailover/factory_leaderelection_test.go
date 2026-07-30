package redisfailover

import (
	"testing"
	"time"

	"github.com/spotahome/kooper/v2/controller/leaderelection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/spotahome/redis-operator/log"
)

// countLeaseRenewals runs a real leader election loop against a fake API
// server for the given window and returns how many lease UPDATE requests
// (renewals) it produced.
func countLeaseRenewals(t *testing.T, lockCfg *leaderelection.LockConfig, window time.Duration) int {
	t.Helper()

	cli := fake.NewSimpleClientset()
	runner, err := leaderelection.New(lockKey, "default", lockCfg, cli, kooperlogger{Logger: log.Dummy})
	require.NoError(t, err)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runner.Run(func() error {
			<-stop
			return nil
		})
	}()

	time.Sleep(window)
	close(stop)
	<-done

	renewals := 0
	for _, action := range cli.Actions() {
		if action.GetVerb() == "update" && action.GetResource().Resource == "leases" {
			if _, ok := action.(k8stesting.UpdateAction); ok {
				renewals++
			}
		}
	}
	return renewals
}

func TestLeaderElectionRetryPeriodDrivesRenewalRate(t *testing.T) {
	window := 2 * time.Second

	fast := countLeaseRenewals(t, &leaderelection.LockConfig{
		LeaseDuration: 600 * time.Millisecond,
		RenewDeadline: 400 * time.Millisecond,
		RetryPeriod:   100 * time.Millisecond,
	}, window)

	slow := countLeaseRenewals(t, &leaderelection.LockConfig{
		LeaseDuration: 2400 * time.Millisecond,
		RenewDeadline: 1600 * time.Millisecond,
		RetryPeriod:   400 * time.Millisecond,
	}, window)

	assert := assert.New(t)
	assert.Greater(fast, 0, "leader should renew the lease at least once")
	assert.Greater(slow, 0, "leader should renew the lease at least once")
	// 100ms vs 400ms retry period is a 4x cadence difference; require at
	// least 2x to stay robust against scheduler jitter in CI.
	assert.GreaterOrEqual(fast, 2*slow,
		"a 4x shorter retry period should renew the lease materially more often (fast=%d slow=%d)", fast, slow)
}

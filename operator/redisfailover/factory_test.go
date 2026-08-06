package redisfailover

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/spotahome/kooper/v2/controller/leaderelection"
)

func TestLeaderElectionLockConfig(t *testing.T) {
	tests := []struct {
		name   string
		cfg    Config
		expCfg *leaderelection.LockConfig
		expErr bool
	}{
		{
			name: "Zero or negative values fall back to defaults",
			cfg: Config{
				LeaderElectionLeaseDuration: -1 * time.Second,
			},
			expCfg: &leaderelection.LockConfig{
				LeaseDuration: DefaultLeaderElectionLeaseDuration,
				RenewDeadline: DefaultLeaderElectionRenewDeadline,
				RetryPeriod:   DefaultLeaderElectionRetryPeriod,
			},
		},
		{
			name: "Valid values pass through unchanged",
			cfg: Config{
				LeaderElectionLeaseDuration: 30 * time.Second,
				LeaderElectionRenewDeadline: 20 * time.Second,
				LeaderElectionRetryPeriod:   5 * time.Second,
			},
			expCfg: &leaderelection.LockConfig{
				LeaseDuration: 30 * time.Second,
				RenewDeadline: 20 * time.Second,
				RetryPeriod:   5 * time.Second,
			},
		},
		{
			name: "Lease duration not greater than renew deadline errors",
			cfg: Config{
				LeaderElectionLeaseDuration: 40 * time.Second,
				LeaderElectionRenewDeadline: 40 * time.Second,
				LeaderElectionRetryPeriod:   10 * time.Second,
			},
			expErr: true,
		},
		{
			name: "Renew deadline not greater than retry period errors",
			cfg: Config{
				LeaderElectionLeaseDuration: 60 * time.Second,
				LeaderElectionRenewDeadline: 10 * time.Second,
				LeaderElectionRetryPeriod:   10 * time.Second,
			},
			expErr: true,
		},
		{
			name: "Renew deadline within the retry period jitter factor errors",
			cfg: Config{
				LeaderElectionLeaseDuration: 60 * time.Second,
				LeaderElectionRenewDeadline: 11 * time.Second,
				LeaderElectionRetryPeriod:   10 * time.Second,
			},
			expErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			lockCfg, err := leaderElectionLockConfig(test.cfg)

			if test.expErr {
				assert.Error(err)
			} else {
				assert.NoError(err)
				assert.Equal(test.expCfg, lockCfg)
			}
		})
	}
}

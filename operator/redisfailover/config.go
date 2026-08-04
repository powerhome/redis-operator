package redisfailover

import "time"

// Default leader election lock timings, used when the corresponding Config
// values are unset. The flag defaults in cmd/utils derive from these.
const (
	DefaultLeaderElectionLeaseDuration = 60 * time.Second
	DefaultLeaderElectionRenewDeadline = 40 * time.Second
	DefaultLeaderElectionRetryPeriod   = 10 * time.Second
)

// Config is the configuration for the redis operator.
type Config struct {
	ListenAddress               string
	MetricsPath                 string
	Concurrency                 int
	SupportedNamespacesRegex    string
	LeaderElectionLeaseDuration time.Duration
	LeaderElectionRenewDeadline time.Duration
	LeaderElectionRetryPeriod   time.Duration
}

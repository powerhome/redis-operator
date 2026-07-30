package redisfailover

import "time"

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

package v1

// Bootstrapping returns true when a BootstrapNode is provided to the RedisFailover spec. Otherwise, it returns false.
func (r *RedisFailover) Bootstrapping() bool {
	return r.Spec.BootstrapNode != nil && r.Spec.BootstrapNode.Enabled
}

// SentinelsAllowed returns true if not Bootstrapping orif BootstrapNode settings allow sentinels to exist
func (r *RedisFailover) SentinelsAllowed() bool {
	bootstrapping := r.Bootstrapping()
	return !bootstrapping || (bootstrapping && r.Spec.BootstrapNode.AllowSentinels)
}

// HaproxyAllowed returns true if haproxy is allowed to run
func (r *RedisFailover) HaproxyAllowed() bool {
	bootstrapping := r.Bootstrapping()
	return !bootstrapping
}

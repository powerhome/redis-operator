package redis

// InstanceAddress holds the forms one instance can appear under in what
// Sentinel reports. Sentinel repeats whatever an instance announced for itself,
// so the same Redis is its pod's address until it announces a name and the name
// afterwards. A caller holding both recognizes the instance either way, and
// across the window where some instances have announced and others have not.
//
// Both fields are optional. An InstanceAddress with neither one names no
// instance, which is how ReplicasUp is asked for every replica rather than all
// but one.
type InstanceAddress struct {
	// IP is the address of the pod running the instance.
	IP string
	// Hostname is the name the pod answers to in DNS.
	Hostname string
}

// Names reports whether the address Sentinel reported belongs to this instance.
// An empty reported address names nothing, whatever this holds.
func (a InstanceAddress) Names(reported string) bool {
	if reported == "" {
		return false
	}
	return reported == a.IP || reported == a.Hostname
}

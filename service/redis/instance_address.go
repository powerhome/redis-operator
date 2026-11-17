package redis

// InstanceAddress holds the forms one instance can appear under in what
// Sentinel reports. Sentinel repeats whatever an instance announced for itself,
// so the same Redis is its pod's address until it announces a name and the name
// afterwards. A caller holding both recognizes the instance either way, and
// across the window where some instances have announced and others have not.
//
// Both fields are optional. The zero value names no instance, which is what a
// caller asking about every replica holds.
type InstanceAddress struct {
	// IP is the address of the pod running the instance.
	IP string
	// Hostname is the name the pod answers to in DNS.
	Hostname string
}

// Names reports whether the address Sentinel reported belongs to this instance.
// An empty reported address belongs to nothing, as does any address when this
// names no instance.
func (a InstanceAddress) Names(reported string) bool {
	if reported == "" || a.IsZero() {
		return false
	}
	return reported == a.IP || reported == a.Hostname
}

// IsZero reports whether this names no instance at all.
func (a InstanceAddress) IsZero() bool {
	return a.IP == "" && a.Hostname == ""
}

package redis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// An instance is whichever of its two forms Sentinel happens to report, so both
// have to name it. The address alone is what a caller holds today, and a caller
// that keeps holding only the address stops recognizing the instance the moment
// it announces a name.
func TestAnInstanceIsNamedByEitherForm(t *testing.T) {
	pod := InstanceAddress{IP: "10.0.0.2", Hostname: "rfr-test-1.rfr-test.testns.svc"}

	tests := []struct {
		name     string
		reported string
		expected bool
	}{
		{name: "its address", reported: "10.0.0.2", expected: true},
		{name: "its name", reported: "rfr-test-1.rfr-test.testns.svc", expected: true},
		{name: "another pod's address", reported: "10.0.0.1", expected: false},
		{name: "another pod's name", reported: "rfr-test-0.rfr-test.testns.svc", expected: false},
		{name: "the same name in another namespace", reported: "rfr-test-1.rfr-test.other.svc", expected: false},
		{name: "nothing at all", reported: "", expected: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, pod.Names(test.reported))
		})
	}
}

// Holding one form and not the other is ordinary: a caller reading a pod that
// has no address yet has only its name, and the reverse holds before anything
// announces names at all.
func TestAnInstanceKnownByOneFormNamesThatForm(t *testing.T) {
	addressOnly := InstanceAddress{IP: "10.0.0.2"}
	assert.True(t, addressOnly.Names("10.0.0.2"))
	assert.False(t, addressOnly.Names("rfr-test-1.rfr-test.testns.svc"))

	nameOnly := InstanceAddress{Hostname: "rfr-test-1.rfr-test.testns.svc"}
	assert.True(t, nameOnly.Names("rfr-test-1.rfr-test.testns.svc"))
	assert.False(t, nameOnly.Names("10.0.0.2"))
}

// Naming no instance is how a caller asks about every replica, and it must not
// come to mean "the replica reporting nothing", which would drop a replica
// Sentinel described without an address.
func TestNoInstanceNamesNothing(t *testing.T) {
	var none InstanceAddress

	assert.False(t, none.Names(""))
	assert.False(t, none.Names("10.0.0.2"))
	assert.False(t, none.Names("rfr-test-1.rfr-test.testns.svc"))
}

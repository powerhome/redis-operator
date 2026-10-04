package redis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spotahome/redis-operator/metrics"
)

func TestIsReachable(t *testing.T) {
	tests := []struct {
		name      string
		flags     string
		reachable bool
	}{
		{name: "a replica", flags: "slave", reachable: true},
		{name: "one sentinel described without flags", flags: "", reachable: false},
		{name: "one this sentinel cannot reach", flags: "s_down,slave", reachable: false},
		{name: "one the sentinels agree is gone", flags: "s_down,o_down,slave", reachable: false},
		{name: "one with no connection", flags: "slave,disconnected", reachable: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.reachable, isReachable(test.flags))
		})
	}
}

func TestPromotableReplicasAsksSentinelForTheMastersReplicas(t *testing.T) {
	sentinel := startFakeSentinel(t, replies{
		"SENTINEL replicas": "*1\r\n" + respArray("name", "10.0.0.1:6379", "ip", "10.0.0.1", "flags", "slave"),
	})

	promotable, err := New(metrics.Dummy).PromotableReplicas(sentinel.host, sentinel.port, nil)

	require.NoError(t, err)
	assert.Equal(t, int32(1), promotable)

	asked := sentinel.asked()
	require.Len(t, asked, 1)
	assert.True(t, isCommand(asked[0], "SENTINEL", "replicas", masterName),
		"it asks for the replicas of the master it monitors, and nothing else")
}

func TestCountPromotableCountsOnlyReachableReplicas(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected int32
	}{
		{name: "none at all", flags: nil, expected: 0},
		{name: "one reachable", flags: []string{"slave"}, expected: 1},
		{name: "two reachable", flags: []string{"slave", "slave"}, expected: 2},
		{name: "one of each", flags: []string{"s_down,slave", "slave"}, expected: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var replicas []interface{}
			for i, flags := range test.flags {
				replicas = append(replicas, describedReplica(addressOfReplica(i), flags))
			}

			promotable, err := countPromotable(replicas, nil)

			require.NoError(t, err)
			assert.Equal(t, test.expected, promotable)
		})
	}
}

// Redis Sentinel reports whichever address a replica announced under "ip", and
// that address with its port under "name", so a bare name or address matches
// only "ip".
func TestCountPromotableLeavesOutTheExcludedAddresses(t *testing.T) {
	replicas := []interface{}{
		describedReplica("rfr-test-1.rfr-test.testns.svc", "slave"),
		describedReplica("rfr-test-2.rfr-test.testns.svc", "slave"),
		describedReplica("10.0.0.9", "slave"),
	}

	tests := []struct {
		name      string
		excluding []string
		expected  int32
	}{
		{name: "nothing excluded", excluding: nil, expected: 3},
		{name: "excluded by name", excluding: []string{"rfr-test-1.rfr-test.testns.svc"}, expected: 2},
		{name: "excluded by address", excluding: []string{"10.0.0.9"}, expected: 2},
		{
			name:      "a pod named both ways is counted out once",
			excluding: []string{"rfr-test-1.rfr-test.testns.svc", "10.0.0.9"},
			expected:  1,
		},
		{name: "an empty address excludes nothing", excluding: []string{""}, expected: 3},
		{name: "an address no replica holds excludes nothing", excluding: []string{"10.0.0.99"}, expected: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			promotable, err := countPromotable(replicas, test.excluding)

			require.NoError(t, err)
			assert.Equal(t, test.expected, promotable)
		})
	}
}

func TestCountPromotableRejectsAReplicaItCannotRead(t *testing.T) {
	_, err := countPromotable([]interface{}{"mymaster"}, nil)

	assert.ErrorContains(t, err, "expected a list of fields")
}

func describedReplica(address, flags string) []interface{} {
	return []interface{}{
		"name", address + ":6379",
		"ip", address,
		"port", "6379",
		"flags", flags,
	}
}

func addressOfReplica(i int) string {
	return "10.0.0." + string(rune('1'+i))
}

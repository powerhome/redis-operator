package redis

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spotahome/redis-operator/metrics"
)

// Sentinel promotes a replica it can still reach. One it has given up on, alone
// or by agreement with the other Sentinels, stays where it is, so counting every
// replica Sentinel remembers would report a failover that cannot happen.
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

func TestReplicasUpCountsOnlyReachableReplicas(t *testing.T) {
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
			sentinel := startStatefulSentinel(t, func(args []string) string {
				return describeReplicas(test.flags)
			})

			up, err := New(metrics.Dummy).ReplicasUp(sentinel.host, sentinel.port)

			require.NoError(t, err)
			assert.Equal(t, test.expected, up)
		})
	}
}

func TestReplicasUpAsksSentinelForTheMastersReplicas(t *testing.T) {
	sentinel := startStatefulSentinel(t, func(args []string) string {
		return describeReplicas([]string{"slave"})
	})

	_, err := New(metrics.Dummy).ReplicasUp(sentinel.host, sentinel.port)

	require.NoError(t, err)
	asked := sentinel.asked()
	require.Len(t, asked, 1)
	assert.True(t, isCommand(asked[0], "SENTINEL", "replicas", masterName),
		"it asks for the replicas of the master it monitors, and nothing else")
}

func TestReplicasUpRejectsAReplicaItCannotRead(t *testing.T) {
	sentinel := startStatefulSentinel(t, func(args []string) string {
		return "*1\r\n$8\r\nmymaster\r\n"
	})

	_, err := New(metrics.Dummy).ReplicasUp(sentinel.host, sentinel.port)

	assert.ErrorContains(t, err, "expected a list of fields")
}

// SENTINEL replicas answers with one list of fields per replica.
func describeReplicas(flags []string) string {
	reply := fmt.Sprintf("*%d\r\n", len(flags))
	for i, flag := range flags {
		reply += respArray("name", fmt.Sprintf("replica-%d", i), "flags", flag)
	}
	return reply
}

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

			up, err := New(metrics.Dummy).ReplicasUp(sentinel.host, sentinel.port, "")

			require.NoError(t, err)
			assert.Equal(t, test.expected, up)
		})
	}
}

func TestReplicasUpAsksSentinelForTheMastersReplicas(t *testing.T) {
	sentinel := startStatefulSentinel(t, func(args []string) string {
		return describeReplicas([]string{"slave"})
	})

	_, err := New(metrics.Dummy).ReplicasUp(sentinel.host, sentinel.port, "")

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

	_, err := New(metrics.Dummy).ReplicasUp(sentinel.host, sentinel.port, "")

	assert.ErrorContains(t, err, "expected a list of fields")
}

// SENTINEL replicas answers with one list of fields per replica.
func describeReplicasWithPriority(flags, priorities []string) string {
	reply := fmt.Sprintf("*%d\r\n", len(flags))
	for i, flag := range flags {
		reply += respArray("name", fmt.Sprintf("replica-%d", i),
			"flags", flag, "slave-priority", priorities[i])
	}
	return reply
}

func describeReplicas(flags []string) string {
	reply := fmt.Sprintf("*%d\r\n", len(flags))
	for i, flag := range flags {
		reply += respArray("name", fmt.Sprintf("replica-%d", i), "flags", flag)
	}
	return reply
}

// The values Sentinel reports, against what the operator may promote.
func TestIsPromotable(t *testing.T) {
	tests := []struct {
		name       string
		priority   string
		promotable bool
	}{
		{name: "the default sentinel reports", priority: "100", promotable: true},
		{name: "lowered but still eligible", priority: "1", promotable: true},
		{name: "never promote this one", priority: "0", promotable: false},
		// Sentinel's own default is 100, and holding a master over a field this
		// did not understand would be a wait nobody could account for.
		{name: "a sentinel that did not report it", priority: "", promotable: true},
		{name: "something unparseable", priority: "banana", promotable: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.promotable, isPromotable(test.priority))
		})
	}
}

func TestReplicasUpSkipsAReplicaSentinelWouldNotPromote(t *testing.T) {
	tests := []struct {
		name       string
		flags      []string
		priorities []string
		expected   int32
	}{
		{name: "reachable and eligible", flags: []string{"slave"}, priorities: []string{"100"}, expected: 1},
		{name: "reachable but never promoted", flags: []string{"slave"}, priorities: []string{"0"}, expected: 0},
		{name: "one of each", flags: []string{"slave", "slave"}, priorities: []string{"0", "100"}, expected: 1},
		// What a failover carrying `replica-priority 0` looks like: every replica
		// answers, and none of them can be promoted.
		{name: "every replica barred", flags: []string{"slave", "slave"}, priorities: []string{"0", "0"}, expected: 0},
		{name: "barred and unreachable", flags: []string{"s_down,slave"}, priorities: []string{"0"}, expected: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sentinel := startStatefulSentinel(t, func(args []string) string {
				return describeReplicasWithPriority(test.flags, test.priorities)
			})

			up, err := New(metrics.Dummy).ReplicasUp(sentinel.host, sentinel.port, "")

			require.NoError(t, err)
			assert.Equal(t, test.expected, up)
		})
	}
}

func describeReplicasAt(ips []string) string {
	reply := fmt.Sprintf("*%d\r\n", len(ips))
	for _, ip := range ips {
		reply += respArray("name", fmt.Sprintf("%s:6379", ip),
			"ip", ip, "flags", "slave", "slave-priority", "100")
	}
	return reply
}

// Asking whether a failover could happen once a pod is gone means asking about
// the replicas that will still be there. A replica on its way out cannot take
// over for a master, so it does not count towards what a Sentinel could promote.
func TestReplicasUpSkipsTheReplicaThatIsLeaving(t *testing.T) {
	tests := []struct {
		name     string
		ips      []string
		leaving  string
		expected int32
	}{
		{name: "nothing is leaving", ips: []string{"10.0.0.1", "10.0.0.2"}, leaving: "", expected: 2},
		{name: "one of two is leaving", ips: []string{"10.0.0.1", "10.0.0.2"}, leaving: "10.0.0.1", expected: 1},
		{name: "the only replica is leaving", ips: []string{"10.0.0.1"}, leaving: "10.0.0.1", expected: 0},
		{name: "an address no replica holds", ips: []string{"10.0.0.1"}, leaving: "10.0.0.9", expected: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sentinel := startStatefulSentinel(t, func(args []string) string {
				return describeReplicasAt(test.ips)
			})

			up, err := New(metrics.Dummy).ReplicasUp(sentinel.host, sentinel.port, test.leaving)

			require.NoError(t, err)
			assert.Equal(t, test.expected, up)
		})
	}
}

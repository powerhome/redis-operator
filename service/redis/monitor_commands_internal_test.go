package redis

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spotahome/redis-operator/metrics"
)

const (
	respOK              = "+OK\r\n"
	refusedUnresolvable = "-ERR Invalid IP address or hostname specified\r\n"
	refusedDuplicate    = "-ERR Duplicate master name.\r\n"
	noSuchMaster        = "-ERR No such master with that name\r\n"

	unresolvableName = "rfr-test-0.rfr-test.testns.svc"
	masterPassword   = "the-password"
)

func TestARefusedMonitorSendsBackTheMasterAndItsPassword(t *testing.T) {
	const held, heldPort = "10.244.3.7", "6379"

	sentinel := startFakeSentinel(t, replies{
		"SENTINEL master": describeMaster(held, heldPort),
		"SENTINEL MONITOR " + masterName + " " + unresolvableName: refusedUnresolvable,
	})

	err := New(metrics.Dummy).MonitorRedisWithPort(sentinel.host, unresolvableName, "6379", "2", masterPassword, sentinel.port)

	assert.Error(t, err)
	assert.Contains(t, sentinel.asked(), []string{"SENTINEL", "MONITOR", masterName, held, heldPort, "2"},
		"the master it was holding goes back")
	assert.Contains(t, sentinel.asked(), []string{"SENTINEL", "SET", masterName, "auth-pass", masterPassword},
		"SENTINEL REMOVE took the password with the master, so it goes back too")
}

func TestARefusedMonitorSendsNothingBackWhenNothingWasWatched(t *testing.T) {
	sentinel := startFakeSentinel(t, replies{
		"SENTINEL master":  noSuchMaster,
		"SENTINEL MONITOR": refusedUnresolvable,
	})

	err := New(metrics.Dummy).MonitorRedisWithPort(sentinel.host, unresolvableName, "6379", "2", masterPassword, sentinel.port)

	assert.Error(t, err)
	assert.Len(t, monitorCommands(sentinel.asked()), 1, "there was no master to put back")
	for _, command := range sentinel.asked() {
		assert.NotContains(t, command, "auth-pass")
	}
}

func TestAnAcceptedMonitorIsGivenThePassword(t *testing.T) {
	sentinel := startFakeSentinel(t, replies{
		"SENTINEL master": describeMaster("10.244.3.7", "6379"),
	})

	err := New(metrics.Dummy).MonitorRedisWithPort(sentinel.host, unresolvableName, "6379", "2", masterPassword, sentinel.port)

	require.NoError(t, err)
	assert.Equal(t, [][]string{{"SENTINEL", "MONITOR", masterName, unresolvableName, "6379", "2"}}, monitorCommands(sentinel.asked()),
		"the address was accepted, so nothing is put back")
	assert.Contains(t, sentinel.asked(), []string{"SENTINEL", "SET", masterName, "auth-pass", masterPassword})
}

func TestMonitoringSendsRemoveBeforeMonitor(t *testing.T) {
	var held struct {
		sync.Mutex
		master bool
	}
	held.master = true

	sentinel := startStatefulSentinel(t, func(args []string) string {
		held.Lock()
		defer held.Unlock()
		switch {
		case isCommand(args, "SENTINEL", "MASTER"):
			return describeMaster("10.244.3.7", "6379")
		case isCommand(args, "SENTINEL", "REMOVE"):
			held.master = false
			return respOK
		case isCommand(args, "SENTINEL", "MONITOR") && held.master:
			return refusedDuplicate
		default:
			return respOK
		}
	})

	err := New(metrics.Dummy).MonitorRedisWithPort(sentinel.host, unresolvableName, "6379", "2", masterPassword, sentinel.port)

	require.NoError(t, err)
	assert.Equal(t, [][]string{{"SENTINEL", "MONITOR", masterName, unresolvableName, "6379", "2"}}, monitorCommands(sentinel.asked()))
}

func TestResettingSendsSentinelReset(t *testing.T) {
	sentinel := startFakeSentinel(t, replies{"SENTINEL reset": ":1\r\n"})

	require.NoError(t, New(metrics.Dummy).ResetSentinel(sentinel.host, sentinel.port))

	assert.Equal(t, [][]string{{"SENTINEL", "reset", "*"}}, sentinel.asked(),
		"every master this sentinel holds, which is only ever mymaster")
}

func monitorCommands(asked [][]string) [][]string {
	var monitors [][]string
	for _, command := range asked {
		if isCommand(command, "SENTINEL", "MONITOR") {
			monitors = append(monitors, command)
		}
	}
	return monitors
}

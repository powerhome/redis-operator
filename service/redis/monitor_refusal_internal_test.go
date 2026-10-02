package redis

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spotahome/redis-operator/metrics"
)

const (
	unresolvableName = "rfr-test-0.rfr-test.testns.svc"
	watchedAddress   = "10.244.3.7"
	watchedPort      = "6379"
	masterPassword   = "the-password"
)

// Sentinel refuses an address it cannot resolve, and by the time it answers,
// SENTINEL REMOVE has already taken away the master it was watching along with
// the password it reached that master by. A Sentinel holding a master it cannot
// authenticate to reports that master down, so both go back.
func TestARefusedMonitorPutsBackTheMasterAndItsPassword(t *testing.T) {
	sentinel := startSentinelThatRefuses(t, unresolvableName)

	err := New(metrics.Dummy).MonitorRedisWithPort(sentinel.host, unresolvableName, "6379", "2", masterPassword, sentinel.port)

	assert.Error(t, err)
	assert.Contains(t, sentinel.asked(), []string{"SENTINEL", "MONITOR", masterName, watchedAddress, watchedPort, "2"})
	assert.Contains(t, sentinel.asked(), []string{"SENTINEL", "SET", masterName, "auth-pass", masterPassword})
}

// A Sentinel watching nothing yet answers SENTINEL master with an error, and
// there is then nothing to put back. Monitoring whatever the last reply
// happened to mention would point it at an arbitrary node.
func TestARefusedMonitorPutsNothingBackWhenNothingWasWatched(t *testing.T) {
	sentinel := startFakeSentinel(t, func(args []string) string {
		switch {
		case isCommand(args, "SENTINEL", "MASTER"):
			return "-ERR No such master with that name\r\n"
		case isCommand(args, "SENTINEL", "MONITOR"):
			return "-ERR Invalid IP address or hostname specified\r\n"
		default:
			return "+OK\r\n"
		}
	})

	err := New(metrics.Dummy).MonitorRedisWithPort(sentinel.host, unresolvableName, "6379", "2", masterPassword, sentinel.port)

	assert.Error(t, err)
	for _, command := range sentinel.asked() {
		assert.NotContains(t, command, "auth-pass")
	}
	assert.Len(t, monitorCommands(sentinel.asked()), 1)
}

// An accepted address is monitored once and given the password, with nothing
// put back.
func TestAnAcceptedMonitorIsGivenThePassword(t *testing.T) {
	sentinel := startSentinelThatRefuses(t, "nothing")

	err := New(metrics.Dummy).MonitorRedisWithPort(sentinel.host, unresolvableName, "6379", "2", masterPassword, sentinel.port)

	require.NoError(t, err)
	assert.Equal(t, [][]string{{"SENTINEL", "MONITOR", masterName, unresolvableName, "6379", "2"}}, monitorCommands(sentinel.asked()))
	assert.Contains(t, sentinel.asked(), []string{"SENTINEL", "SET", masterName, "auth-pass", masterPassword})
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

// A Sentinel already watching watchedAddress, which accepts every address but
// the one named.
func startSentinelThatRefuses(t *testing.T, refused string) *fakeSentinel {
	return startFakeSentinel(t, func(args []string) string {
		switch {
		case isCommand(args, "SENTINEL", "MASTER"):
			return describeMaster(watchedAddress, watchedPort)
		case isCommand(args, "SENTINEL", "MONITOR") && args[3] == refused:
			return "-ERR Invalid IP address or hostname specified\r\n"
		default:
			return "+OK\r\n"
		}
	})
}

// The fields monitored reads, at the positions it reads them from.
func describeMaster(address, port string) string {
	fields := []string{"name", masterName, "ip", address, "port", port}
	reply := fmt.Sprintf("*%d\r\n", len(fields))
	for _, field := range fields {
		reply += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
	}
	return reply
}

func isCommand(args []string, name ...string) bool {
	if len(args) < len(name) {
		return false
	}
	for i, word := range name {
		if !strings.EqualFold(args[i], word) {
			return false
		}
	}
	return true
}

// A Sentinel that answers whatever the test tells it to, and remembers what it
// was asked. Enough of the protocol for the commands MonitorRedisWithPort
// sends, and no more.
type fakeSentinel struct {
	host, port string

	mutex    sync.Mutex
	commands [][]string
}

func (s *fakeSentinel) asked() [][]string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([][]string(nil), s.commands...)
}

func (s *fakeSentinel) record(args []string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.commands = append(s.commands, args)
}

func startFakeSentinel(t *testing.T, answer func(args []string) string) *fakeSentinel {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	host, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	sentinel := &fakeSentinel{host: host, port: port}

	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				reader := bufio.NewReader(connection)
				for {
					args, err := readCommand(reader)
					if err != nil {
						return
					}
					sentinel.record(args)
					if _, err := io.WriteString(connection, answer(args)); err != nil {
						return
					}
				}
			}()
		}
	}()

	return sentinel
}

func readCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "*")))
	if err != nil {
		return nil, fmt.Errorf("reading the argument count from %q: %w", header, err)
	}

	args := make([]string, 0, count)
	for range count {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		length, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "$")))
		if err != nil {
			return nil, fmt.Errorf("reading an argument length from %q: %w", header, err)
		}
		argument := make([]byte, length+len("\r\n"))
		if _, err := io.ReadFull(reader, argument); err != nil {
			return nil, err
		}
		args = append(args, string(argument[:length]))
	}
	return args, nil
}

// Sentinel promotes a replica it can still reach. One it has given up on, alone
// or by agreement with the other Sentinels, stays where it is, so counting
// every replica Sentinel remembers would report a failover that cannot happen.
func TestOnlyReachableReplicasAreCounted(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected int32
	}{
		{name: "none at all", flags: nil, expected: 0},
		{name: "one reachable", flags: []string{"slave"}, expected: 1},
		{name: "two reachable", flags: []string{"slave", "slave"}, expected: 2},
		{name: "one it cannot reach", flags: []string{"s_down,slave"}, expected: 0},
		{name: "one the others agree is gone", flags: []string{"s_down,o_down,slave"}, expected: 0},
		{name: "one with no connection", flags: []string{"slave,disconnected"}, expected: 0},
		{name: "one of each", flags: []string{"s_down,slave", "slave"}, expected: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sentinel := startFakeSentinel(t, func(args []string) string {
				return describeReplicas(test.flags)
			})

			up, err := New(metrics.Dummy).ReplicasUp(sentinel.host, sentinel.port, nil)

			require.NoError(t, err)
			assert.Equal(t, test.expected, up)
		})
	}
}

// SENTINEL replicas answers with one list of fields per replica.
func describeReplicas(flags []string) string {
	reply := fmt.Sprintf("*%d\r\n", len(flags))
	for i, flag := range flags {
		fields := []string{"name", fmt.Sprintf("replica-%d", i), "flags", flag}
		reply += fmt.Sprintf("*%d\r\n", len(fields))
		for _, field := range fields {
			reply += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
		}
	}
	return reply
}

func describeNamedReplicas(replicas map[string]string) string {
	reply := fmt.Sprintf("*%d\r\n", len(replicas))
	for _, address := range slices.Sorted(maps.Keys(replicas)) {
		fields := []string{"name", address + ":6379", "ip", address, "flags", replicas[address]}
		reply += fmt.Sprintf("*%d\r\n", len(fields))
		for _, field := range fields {
			reply += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
		}
	}
	return reply
}

func TestReplicasUpLeavesOutTheExcludedAddresses(t *testing.T) {
	replicas := map[string]string{
		"rfr-test-1.rfr-test.testns.svc": "slave",
		"rfr-test-2.rfr-test.testns.svc": "slave",
		"10.0.0.9":                       "slave",
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
			sentinel := startFakeSentinel(t, func(args []string) string {
				return describeNamedReplicas(replicas)
			})

			up, err := New(metrics.Dummy).ReplicasUp(sentinel.host, sentinel.port, test.excluding)

			require.NoError(t, err)
			assert.Equal(t, test.expected, up)
		})
	}
}

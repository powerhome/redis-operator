package redis

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

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

type replies map[string]string

func (r replies) answer(args []string) string {
	reply, matched := respOK, -1
	for command, candidate := range r {
		words := strings.Fields(command)
		if len(words) > matched && isCommand(args, words...) {
			reply, matched = candidate, len(words)
		}
	}
	return reply
}

func startFakeSentinel(t *testing.T, answers replies) *fakeSentinel {
	t.Helper()
	return startStatefulSentinel(t, answers.answer)
}

func startStatefulSentinel(t *testing.T, answer func(args []string) string) *fakeSentinel {
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

func respArray(fields ...string) string {
	reply := fmt.Sprintf("*%d\r\n", len(fields))
	for _, field := range fields {
		reply += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
	}
	return reply
}

// The fields monitored reads, at the positions it reads them from.
func describeMaster(address, port string) string {
	return respArray("name", masterName, "ip", address, "port", port)
}

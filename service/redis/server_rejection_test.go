package redis_test

import (
	"errors"
	"fmt"
	"net"
	"testing"

	rediscli "github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"

	"github.com/spotahome/redis-operator/service/redis"
)

// serverReply is what go-redis hands back when Redis answers with an error. Its
// own type for these is internal to that module, and the interface is the whole
// of what distinguishes a reply from a failure to get one.
type serverReply string

func (e serverReply) Error() string { return string(e) }

func (serverReply) RedisError() {}

// The caller that asks this is deciding whether to go on to the next instance.
// A reply makes the next answer predictable, so there is nothing to learn from
// asking. Everything else leaves the command untried, and the next instance may
// well take it.
func TestIsServerRejection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "an address sentinel will not accept",
			err:  serverReply("ERR Invalid IP address or hostname specified"),
			want: true,
		},
		{
			name: "a reply wrapped by the caller",
			err:  fmt.Errorf("pointing sentinel 1.1.1.1 at 0.0.0.0: %w", serverReply("ERR Invalid IP address or hostname specified")),
			want: true,
		},
		{
			name: "nothing listening on the sentinel port",
			err:  &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			want: false,
		},
		{
			name: "a sentinel that never answers",
			err:  errors.New("dial tcp 1.1.1.1:26379: i/o timeout"),
			want: false,
		},
		{
			name: "the client closed under us",
			err:  rediscli.ErrClosed,
			want: false,
		},
		{
			name: "no error at all",
			err:  nil,
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, redis.IsServerRejection(test.err))
		})
	}
}

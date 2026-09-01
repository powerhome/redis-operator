package redis_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/spotahome/redis-operator/service/redis"
)

// The three answers below all mean the running Redis is not using the password
// the failover is configured with, which is a state no amount of waiting
// resolves. Anything else is a fault that may clear on its own, so the operator
// must not mistake it for one.
func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "no error", err: nil, want: false},
		{
			name: "Redis wants a password and got none",
			err:  errors.New("NOAUTH Authentication required."),
			want: true,
		},
		{
			name: "Redis got the wrong password",
			err:  errors.New("WRONGPASS invalid username-password pair or user is disabled."),
			want: true,
		},
		{
			// What Redis answers when auth.secretPath is added to a failover
			// whose pods are already running without a password.
			name: "Redis wants no password and got one",
			err:  errors.New("ERR AUTH <password> called without any password configured for the default user. Are you sure your configuration is correct?"),
			want: true,
		},
		{
			name: "the same, as older Redis phrases it",
			err:  errors.New("ERR Client sent AUTH, but no password is set"),
			want: true,
		},
		{
			name: "a pod that is not up yet",
			err:  errors.New("dial tcp 10.0.0.1:6379: connect: connection refused"),
			want: false,
		},
		{
			name: "a slow or unreachable pod",
			err:  errors.New("dial tcp 10.0.0.1:6379: i/o timeout"),
			want: false,
		},
		{
			// Restricted permissions are a real ACL fault, but rolling the pods
			// does not fix them, so this must not route to the repair path.
			name: "a command the user may not run",
			err:  errors.New("NOPERM this user has no permissions to run the 'info' command"),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, redis.IsAuthError(test.err))
		})
	}
}

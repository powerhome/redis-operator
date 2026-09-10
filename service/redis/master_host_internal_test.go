package redis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// What a replica answers with depends on who told it to replicate: the operator
// gives an address, Sentinel gives a name once it is announcing hostnames. A
// pattern that reads only addresses turns the second into the empty string,
// which GetSlaveOf reports as a replica following no master at all, so a
// replica following the wrong one passes unremarked.
func TestMasterHostIsReadInEitherForm(t *testing.T) {
	tests := []struct {
		name     string
		info     string
		expected string
	}{
		{
			name:     "an address",
			info:     "role:slave\r\nmaster_host:10.244.3.7\r\nmaster_link_status:up\r\n",
			expected: "10.244.3.7",
		},
		{
			name:     "a name",
			info:     "role:slave\r\nmaster_host:rfr-test-0.rfr-test.testns.svc\r\nmaster_link_status:up\r\n",
			expected: "rfr-test-0.rfr-test.testns.svc",
		},
		{
			name:     "the localhost a pod starts with",
			info:     "role:slave\r\nmaster_host:127.0.0.1\r\nmaster_link_status:down\r\n",
			expected: "127.0.0.1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			match := redisMasterHostRE.FindStringSubmatch(test.info)
			assert.Len(t, match, 2)
			assert.Equal(t, test.expected, match[1])
		})
	}
}

// A master says nothing about a master of its own, which is how GetSlaveOf
// tells one apart from a replica.
func TestMasterHostIsAbsentOnAMaster(t *testing.T) {
	info := "role:master\r\nconnected_slaves:1\r\nslave0:ip=10.244.3.7,port=6379\r\n"

	assert.Empty(t, redisMasterHostRE.FindStringSubmatch(info))
}

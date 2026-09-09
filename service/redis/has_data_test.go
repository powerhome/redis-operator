package redis

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A Redis reading its dataset from disk is the case that matters here. INFO is
// served while `loading:1`, and the keyspace section fills in as keys are
// inserted, so a node holding a full dataset on disk reports an empty keyspace
// until enough of it is in memory. Reading that as "empty" would let the
// operator seed a master over a restarting cluster, which is the outcome
// docs/adr/ADR-001 exists to prevent.
func TestHasDataFromInfo(t *testing.T) {
	tests := []struct {
		name     string
		info     string
		wantData bool
		wantErr  error
	}{
		{
			name:     "empty instance, finished loading",
			info:     "# Persistence\r\nloading:0\r\nrdb_last_save_time:0\r\n\r\n# Keyspace\r\n",
			wantData: false,
		},
		{
			name:     "holds keys",
			info:     "# Persistence\r\nloading:0\r\n\r\n# Keyspace\r\ndb0:keys=42,expires=0,avg_ttl=0\r\n",
			wantData: true,
		},
		{
			name:     "holds keys in a database other than the first",
			info:     "# Persistence\r\nloading:0\r\n\r\n# Keyspace\r\ndb3:keys=7,expires=0,avg_ttl=0\r\n",
			wantData: true,
		},
		{
			name:    "loading, keyspace not yet populated",
			info:    "# Persistence\r\nloading:1\r\n\r\n# Keyspace\r\n",
			wantErr: ErrRedisLoading,
		},
		{
			name:    "loading, keyspace partly populated",
			info:    "# Persistence\r\nloading:1\r\n\r\n# Keyspace\r\ndb0:keys=12,expires=0,avg_ttl=0\r\n",
			wantErr: ErrRedisLoading,
		},
		{
			name:    "loading a replica dataset asynchronously",
			info:    "# Persistence\r\nloading:0\r\nasync_loading:1\r\n\r\n# Keyspace\r\n",
			wantErr: ErrRedisLoading,
		},
		{
			// `loading:0` must not match the loading pattern merely by
			// containing it as a substring elsewhere in the payload.
			name:     "a counter whose name ends in loading does not count",
			info:     "# Stats\r\nrdb_last_load_keys_loaded:1000\r\nloading:0\r\n\r\n# Keyspace\r\n",
			wantData: false,
		},
		{
			// A database reported with zero keys is empty, not populated.
			name:     "database present but holding nothing",
			info:     "# Persistence\r\nloading:0\r\n\r\n# Keyspace\r\ndb0:keys=0,expires=0,avg_ttl=0\r\n",
			wantData: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)
			hasData, err := hasDataFromInfo(test.info)
			if test.wantErr != nil {
				assert.True(errors.Is(err, test.wantErr))
				return
			}
			assert.NoError(err)
			assert.Equal(test.wantData, hasData)
		})
	}
}

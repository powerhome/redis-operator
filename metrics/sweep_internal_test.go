package metrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// markResourceStale makes the sweep treat a failover as gone, the way deleting
// one does: nothing updates its tracker entry again. RecordRedisCheck and
// RecordSentinelCheck write that entry under the failover's own name, on every
// pass and on both the healthy and the unhealthy branch.
func markResourceStale(namespace, kind, name string) {
	mutex.Lock()
	defer mutex.Unlock()
	resourceMetricLastUpdated[fmt.Sprintf("%v/%v/%v", namespace, kind, name)] =
		time.Now().Add(-2 * metricsGCIntervalMinutes * time.Minute)
}

func seriesOf(t *testing.T, reg *prometheus.Registry, name string) int {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == name {
			return len(family.GetMetric())
		}
	}
	return 0
}

func TestTheSweepTakesEveryCountOfAFailoverThatIsGone(t *testing.T) {
	assert := assert.New(t)

	reg := prometheus.NewRegistry()
	rec := NewRecorder("swept", reg)

	rec.SetReplicaCountUnsupported("ns", "gone", "redis")
	rec.SetReplicaCountOK("ns", "gone", "sentinel")
	rec.SetReplicaCountOK("ns", "stays", "redis")
	rec.SetClusterOK("ns", "gone")

	require.Equal(t, 3, seriesOf(t, reg, "swept_controller_replica_count_ok"),
		"the three series the sweep is about")

	markResourceStale("ns", "redisfailover", "gone")
	sweepStaleMetrics()

	assert.Equal(1, seriesOf(t, reg, "swept_controller_replica_count_ok"),
		"a count left behind reads 0 for a component that no longer exists, and an alert on it never stops")
	assert.Equal(0, seriesOf(t, reg, "swept_controller_cluster_ok"),
		"the sweep already took this one, which is how the gap was found")
}

package redisfailover

import (
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/metrics"
	mRFService "github.com/spotahome/redis-operator/mocks/operator/redisfailover/service"
)

// countingMetrics records which components were marked, so a test can say how
// often rather than only whether.
type countingMetrics struct {
	metrics.Recorder
	mu          sync.Mutex
	ok          map[string]int
	unsupported map[string]int
}

func newCountingMetrics() *countingMetrics {
	return &countingMetrics{
		Recorder:    metrics.Dummy,
		ok:          map[string]int{},
		unsupported: map[string]int{},
	}
}

func (c *countingMetrics) SetReplicaCountOK(_, _, component string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ok[component]++
}

func (c *countingMetrics) SetReplicaCountUnsupported(_, _, component string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unsupported[component]++
}

func failoverWithCounts(redises, sentinels int32) *redisfailoverv1.RedisFailover {
	rf := &redisfailoverv1.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{Name: "counted", Namespace: "testns"},
	}
	rf.Spec.Redis.Replicas = redises
	rf.Spec.Sentinel.Replicas = sentinels
	return rf
}

func TestUnsupportedCountsReachTheStatus(t *testing.T) {
	assert := assert.New(t)

	logger := &countingLogger{}
	service := &mRFService.RedisFailoverClient{}
	service.On("UpdateStatus", mock.Anything).Return(&redisfailoverv1.RedisFailover{}, nil)
	handler := NewRedisFailoverHandler(Config{}, service, nil, nil, nil, metrics.Dummy, logger)

	rf := failoverWithCounts(1, 2)
	assert.NoError(handler.reportUnsupportedReplicaCounts(rf))

	assert.Len(rf.Status.Warnings, 2, "the person who set the counts reads them here")
	assert.Equal(1, logger.mentioning("redis.replicas is 1"))
	assert.Equal(1, logger.mentioning("sentinel.replicas is 2"))
	service.AssertNumberOfCalls(t, "UpdateStatus", 1)
}

func TestUnsupportedCountsAreWrittenOnlyWhenTheyChange(t *testing.T) {
	assert := assert.New(t)

	logger := &countingLogger{}
	service := &mRFService.RedisFailoverClient{}
	service.On("UpdateStatus", mock.Anything).Return(&redisfailoverv1.RedisFailover{}, nil)
	handler := NewRedisFailoverHandler(Config{}, service, nil, nil, nil, metrics.Dummy, logger)

	rf := failoverWithCounts(1, 3)
	for i := 0; i < 5; i++ {
		assert.NoError(handler.reportUnsupportedReplicaCounts(rf))
	}

	service.AssertNumberOfCalls(t, "UpdateStatus", 1)
	assert.Equal(1, logger.mentioning("redis.replicas is 1"),
		"a reconcile every few seconds must not repeat itself")
}

func TestASupportedCountClearsTheStatus(t *testing.T) {
	assert := assert.New(t)

	logger := &countingLogger{}
	service := &mRFService.RedisFailoverClient{}
	service.On("UpdateStatus", mock.Anything).Return(&redisfailoverv1.RedisFailover{}, nil)
	handler := NewRedisFailoverHandler(Config{}, service, nil, nil, nil, metrics.Dummy, logger)

	rf := failoverWithCounts(1, 3)
	assert.NoError(handler.reportUnsupportedReplicaCounts(rf))
	assert.NotEmpty(rf.Status.Warnings)

	rf.Spec.Redis.Replicas = 3
	assert.NoError(handler.reportUnsupportedReplicaCounts(rf))

	assert.Empty(rf.Status.Warnings,
		"fixing the count is all it takes, with no restart and no other edit")
	service.AssertNumberOfCalls(t, "UpdateStatus", 2)
}

func TestASupportedCountIsNeverWritten(t *testing.T) {
	assert := assert.New(t)

	logger := &countingLogger{}
	service := &mRFService.RedisFailoverClient{}
	handler := NewRedisFailoverHandler(Config{}, service, nil, nil, nil, metrics.Dummy, logger)

	rf := failoverWithCounts(3, 3)
	assert.NoError(handler.reportUnsupportedReplicaCounts(rf))

	assert.Empty(rf.Status.Warnings)
	service.AssertNotCalled(t, "UpdateStatus", mock.Anything)
	assert.Empty(logger.warnings, "a failover that can survive a failure hears nothing")
}

func TestTheGaugeIsSetOnEveryPass(t *testing.T) {
	assert := assert.New(t)

	recorder := newCountingMetrics()
	service := &mRFService.RedisFailoverClient{}
	service.On("UpdateStatus", mock.Anything).Return(&redisfailoverv1.RedisFailover{}, nil)
	handler := NewRedisFailoverHandler(Config{}, service, nil, nil, nil, recorder, &countingLogger{})

	rf := failoverWithCounts(1, 3)
	for i := 0; i < 3; i++ {
		assert.NoError(handler.reportUnsupportedReplicaCounts(rf))
	}

	assert.Equal(3, recorder.unsupported["redis"],
		"an operator that restarts into a failover already warned still reports the gauge")
	assert.Equal(3, recorder.ok["sentinel"],
		"a component that meets its minimum is reported as such, not left absent")
	assert.Equal(0, recorder.ok["redis"])
	service.AssertNumberOfCalls(t, "UpdateStatus", 1)
}

func TestAComponentThisFailoverDoesNotRunStillReportsACount(t *testing.T) {
	assert := assert.New(t)

	recorder := newCountingMetrics()
	service := &mRFService.RedisFailoverClient{}
	service.On("UpdateStatus", mock.Anything).Return(&redisfailoverv1.RedisFailover{}, nil)
	handler := NewRedisFailoverHandler(Config{}, service, nil, nil, nil, recorder, &countingLogger{})

	rf := failoverWithCounts(3, 3)
	assert.Nil(rf.Spec.Haproxy, "this failover runs no proxies")

	assert.NoError(handler.reportUnsupportedReplicaCounts(rf))

	assert.Equal(1, recorder.ok["haproxy"],
		"a failover that stops running proxies would otherwise leave the gauge reading whatever it last held")
	assert.Equal(0, recorder.unsupported["haproxy"])
}

func TestAStaleStatusWriteIsLeftForTheNextReconcile(t *testing.T) {
	assert := assert.New(t)

	logger := &countingLogger{}
	service := &mRFService.RedisFailoverClient{}
	service.On("UpdateStatus", mock.Anything).Return(
		&redisfailoverv1.RedisFailover{},
		apierrors.NewConflict(
			schema.GroupResource{Group: "databases.spotahome.com", Resource: "redisfailovers"},
			"counted", errors.New("the object has been modified")),
	)
	handler := NewRedisFailoverHandler(Config{}, service, nil, nil, nil, metrics.Dummy, logger)

	rf := failoverWithCounts(1, 3)

	assert.NoError(handler.reportUnsupportedReplicaCounts(rf),
		"a spec written a moment ago leaves this status one version behind, which the next reconcile fixes")
	assert.Empty(logger.warnings, "nothing is announced that was not written")
}

// answeringLikeTheAPIServer refuses a write that carries any version but the
// current one, and assigns the next version to one that succeeds.
func answeringLikeTheAPIServer(version *int) func(*redisfailoverv1.RedisFailover) (*redisfailoverv1.RedisFailover, error) {
	return func(rf *redisfailoverv1.RedisFailover) (*redisfailoverv1.RedisFailover, error) {
		if rf.ResourceVersion != strconv.Itoa(*version) {
			return nil, apierrors.NewConflict(
				schema.GroupResource{Group: "databases.spotahome.com", Resource: "redisfailovers"},
				rf.Name, errors.New("the object has been modified"))
		}
		*version++
		written := rf.DeepCopy()
		written.ResourceVersion = strconv.Itoa(*version)
		return written, nil
	}
}

func TestAConditionWrittenAfterTheWarningsIsNotRefused(t *testing.T) {
	assert := assert.New(t)

	version := 1
	service := &mRFService.RedisFailoverClient{}
	service.On("UpdateStatus", mock.Anything).Return(answeringLikeTheAPIServer(&version))
	handler := NewRedisFailoverHandler(Config{}, service, nil, nil, nil, metrics.Dummy, &countingLogger{})

	rf := failoverWithCounts(1, 2)
	rf.ResourceVersion = strconv.Itoa(version)

	assert.NoError(handler.reportUnsupportedReplicaCounts(rf))
	assert.NoError(handler.writeStatus(rf),
		"a reconcile writes the status several times, and the later writes carry the conditions a reader acts on")
}

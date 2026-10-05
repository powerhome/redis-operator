package redisfailover_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	mRFService "github.com/spotahome/redis-operator/mocks/operator/redisfailover/service"
	mK8SService "github.com/spotahome/redis-operator/mocks/service/k8s"
	rfOperator "github.com/spotahome/redis-operator/operator/redisfailover"
)

// Three Sentinels with one on an old pod template. Taking it away leaves two,
// which is the quorum, so it goes.
func TestAStaleSentinelIsReplacedWhenTheRestCanAgreeAFailover(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF(false, false)
	rf.Spec.Sentinel.Replicas = 3
	stale := "rfs-test-1"

	mrfc := &mRFService.RedisFailoverCheck{}
	mrfc.On("GetSentinelSetUpdateRevision", rf).Once().Return("2", nil)
	mrfc.On("GetSentinelsPods", rf).Once().Return([]string{"rfs-test-0", stale, "rfs-test-2"}, nil)
	mrfc.On("GetPodRevisionHash", "rfs-test-0", rf).Once().Return("2", nil)
	mrfc.On("GetPodRevisionHash", stale, rf).Once().Return("1", nil)
	mrfc.On("CheckSentinelsCanSpareOne", rf, int32(3)).Once().Return(nil)

	mrfh := &mRFService.RedisFailoverHeal{}
	mrfh.On("DeletePod", stale, rf).Once().Return(nil)

	handler := rfOperator.NewRedisFailoverHandler(generateConfig(), &mRFService.RedisFailoverClient{}, mrfc, mrfh, &mK8SService.Services{}, metrics.Dummy, log.Dummy)

	assert.NoError(handler.UpdateSentinelPods(rf, 3))

	mrfc.AssertExpectations(t)
	mrfh.AssertExpectations(t)
	// One at a time: the third pod is never even read.
	mrfc.AssertNotCalled(t, "GetPodRevisionHash", "rfs-test-2", rf)
}

// The Sentinels that remain could not agree a failover, so the stale pod stays
// and the operator says which Sentinel it is waiting on.
func TestAStaleSentinelIsHeldBackWhenTheRestCannot(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF(false, false)
	rf.Spec.Sentinel.Replicas = 3
	stale := "rfs-test-1"

	mrfc := &mRFService.RedisFailoverCheck{}
	mrfc.On("GetSentinelSetUpdateRevision", rf).Once().Return("2", nil)
	mrfc.On("GetSentinelsPods", rf).Once().Return([]string{stale}, nil)
	mrfc.On("GetPodRevisionHash", stale, rf).Once().Return("1", nil)
	mrfc.On("CheckSentinelsCanSpareOne", rf, int32(2)).Once().
		Return(errors.New("2 of 3 sentinels report the master, and 2 must remain to agree a failover"))

	mrfh := &mRFService.RedisFailoverHeal{}

	handler := rfOperator.NewRedisFailoverHandler(generateConfig(), &mRFService.RedisFailoverClient{}, mrfc, mrfh, &mK8SService.Services{}, metrics.Dummy, log.Dummy)

	assert.NoError(handler.UpdateSentinelPods(rf, 2))

	mrfh.AssertNotCalled(t, "DeletePod", stale, rf)
	mrfc.AssertExpectations(t)
}

// Every Sentinel already runs the current template, so nothing is disturbed and
// the question is never asked.
func TestNoSentinelIsReplacedWhenEveryRevisionMatches(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF(false, false)
	rf.Spec.Sentinel.Replicas = 3

	mrfc := &mRFService.RedisFailoverCheck{}
	mrfc.On("GetSentinelSetUpdateRevision", rf).Once().Return("2", nil)
	mrfc.On("GetSentinelsPods", rf).Once().Return([]string{"rfs-test-0", "rfs-test-1"}, nil)
	mrfc.On("GetPodRevisionHash", "rfs-test-0", rf).Once().Return("2", nil)
	mrfc.On("GetPodRevisionHash", "rfs-test-1", rf).Once().Return("2", nil)

	mrfh := &mRFService.RedisFailoverHeal{}

	handler := rfOperator.NewRedisFailoverHandler(generateConfig(), &mRFService.RedisFailoverClient{}, mrfc, mrfh, &mK8SService.Services{}, metrics.Dummy, log.Dummy)

	assert.NoError(handler.UpdateSentinelPods(rf, 3))

	mrfc.AssertNotCalled(t, "CheckSentinelsCanSpareOne", rf, int32(3))
	mrfh.AssertNotCalled(t, "DeletePod", "rfs-test-0", rf)
	mrfc.AssertExpectations(t)
}

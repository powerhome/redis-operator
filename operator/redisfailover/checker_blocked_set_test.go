package redisfailover_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	mRFService "github.com/spotahome/redis-operator/mocks/operator/redisfailover/service"
	mK8SService "github.com/spotahome/redis-operator/mocks/service/k8s"
	rfOperator "github.com/spotahome/redis-operator/operator/redisfailover"
)

func blockedSetHandler(mrfc *mRFService.RedisFailoverCheck, mrfh *mRFService.RedisFailoverHeal) *rfOperator.RedisFailoverHandler {
	return rfOperator.NewRedisFailoverHandler(generateConfig(), &mRFService.RedisFailoverClient{}, mrfc, mrfh, &mK8SService.Services{}, metrics.Dummy, log.Dummy)
}

func TestThePodBlockingTheSetIsReplacedBeforeTheUsualOrder(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF(false, false)
	blocking := "rfr-test-0"

	mrfc := &mRFService.RedisFailoverCheck{}
	mrfc.On("GetRedisesIPs", rf).Once().Return([]string{"1.1.1.1", "1.1.1.2"}, nil)
	mrfc.On("GetMasterIP", rf).Once().Return("1.1.1.1", nil)
	mrfc.On("CheckRedisSlavesReady", "1.1.1.2", rf).Once().Return(true, nil)
	mrfc.On("GetStatefulSetUpdateRevision", rf).Once().Return("2", nil)
	mrfc.On("GetRedisesPodsBlockingTheSet", rf).Once().Return([]string{blocking, "rfr-test-1"}, nil)
	mrfc.On("GetRedisesMasterPod", rf).Once().Return("rfr-test-1", nil)

	mrfh := &mRFService.RedisFailoverHeal{}
	mrfh.On("DeletePod", blocking, rf).Once().Return(nil)

	assert.NoError(blockedSetHandler(mrfc, mrfh).UpdateRedisesPods(rf))

	mrfc.AssertNotCalled(t, "GetRedisesSlavesPods", rf)
	mrfh.AssertExpectations(t)
}

func TestABlockingMasterWaitsForAReplicaToPromote(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF(false, false)
	master := "rfr-test-0"

	mrfc := &mRFService.RedisFailoverCheck{}
	mrfc.On("GetRedisesIPs", rf).Once().Return([]string{"1.1.1.1"}, nil)
	mrfc.On("GetMasterIP", rf).Once().Return("1.1.1.1", nil)
	mrfc.On("GetStatefulSetUpdateRevision", rf).Once().Return("2", nil)
	mrfc.On("GetRedisesPodsBlockingTheSet", rf).Once().Return([]string{master}, nil)
	mrfc.On("GetRedisesMasterPod", rf).Once().Return(master, nil)

	mrfh := &mRFService.RedisFailoverHeal{}

	assert.NoError(blockedSetHandler(mrfc, mrfh).UpdateRedisesPods(rf))

	mrfh.AssertNotCalled(t, "DeletePod", master, rf)
}

package service_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	mK8SService "github.com/spotahome/redis-operator/mocks/service/k8s"
	rfservice "github.com/spotahome/redis-operator/operator/redisfailover/service"
)

// A failover that switches to bootstrapping without Sentinels has every
// Sentinel resource removed, including the set and the headless service that
// governs it. Nothing else removes either, and reading the Deployment first and
// stopping when it was absent left all of them behind, which after the move to
// a set is every failover.
func TestDestroySentinelResourcesRemovesTheSetAndItsService(t *testing.T) {
	assert := assert.New(t)

	rf := &redisfailoverv1.RedisFailover{}
	rf.Name = "test"
	rf.Namespace = "testns"

	ms := &mK8SService.Services{}
	ms.On("DeleteService", "testns", rfservice.GetSentinelName(rf)).Once().Return(nil)
	ms.On("DeleteService", "testns", rfservice.GetSentinelHeadlessName(rf)).Once().Return(nil)
	ms.On("DeleteConfigMap", "testns", rfservice.GetSentinelName(rf)).Once().Return(nil)
	ms.On("DeleteStatefulSet", "testns", rfservice.GetSentinelName(rf)).Once().Return(nil)
	ms.On("DeleteDeployment", "testns", rfservice.GetSentinelName(rf)).Once().Return(notFound("deployments", rfservice.GetSentinelName(rf)))
	ms.On("DeletePodDisruptionBudget", "testns", rfservice.GetSentinelName(rf)).Once().Return(nil)

	client := rfservice.NewRedisFailoverKubeClient(ms, log.Dummy, metrics.Dummy)

	assert.NoError(client.DestroySentinelResources(rf))
	ms.AssertExpectations(t)
}

// A resource that is already gone is not an error, and the rest are still
// removed. Anything else stops the teardown and is reported.
func TestDestroySentinelResourcesReportsARealFailure(t *testing.T) {
	assert := assert.New(t)

	rf := &redisfailoverv1.RedisFailover{}
	rf.Name = "test"
	rf.Namespace = "testns"

	ms := &mK8SService.Services{}
	ms.On("DeleteService", "testns", rfservice.GetSentinelName(rf)).Once().Return(errors.New("the api server said no"))

	client := rfservice.NewRedisFailoverKubeClient(ms, log.Dummy, metrics.Dummy)

	assert.ErrorContains(client.DestroySentinelResources(rf), "the api server said no")
	ms.AssertNotCalled(t, "DeleteStatefulSet", "testns", rfservice.GetSentinelName(rf))
}

package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	mK8SService "github.com/spotahome/redis-operator/mocks/service/k8s"
	rfservice "github.com/spotahome/redis-operator/operator/redisfailover/service"
)

func TestDestroySentinelDeploymentRemovesAnExistingOne(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	ms.On("GetDeployment", namespace, sentinelName).Once().Return(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: sentinelName, Namespace: namespace},
	}, nil)
	ms.On("DeleteDeployment", namespace, sentinelName).Once().Return(nil)

	client := rfservice.NewRedisFailoverKubeClient(ms, log.Dummy, metrics.Dummy)

	assert.NoError(client.DestroySentinelDeployment(rf))
	ms.AssertExpectations(t)
}

func TestDestroySentinelDeploymentToleratesNoneBeingThere(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	ms.On("GetDeployment", namespace, sentinelName).Once().Return(nil, errors.NewNotFound(
		schema.GroupResource{Group: "apps", Resource: "deployments"}, sentinelName))

	client := rfservice.NewRedisFailoverKubeClient(ms, log.Dummy, metrics.Dummy)

	assert.NoError(client.DestroySentinelDeployment(rf))
	ms.AssertNotCalled(t, "DeleteDeployment", namespace, sentinelName)
}

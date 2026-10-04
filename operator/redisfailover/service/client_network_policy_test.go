package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	mK8SService "github.com/spotahome/redis-operator/mocks/service/k8s"
	rfservice "github.com/spotahome/redis-operator/operator/redisfailover/service"
)

func TestDestroyOrphanedSentinelNetworkPolicyRemovesAnExistingOne(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()
	name := rfservice.GetSentinelNetworkPolicyName(rf)

	ms := &mK8SService.Services{}
	ms.On("GetNetworkPolicy", namespace, name).Once().Return(&networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}, nil)
	ms.On("DeleteNetworkPolicy", namespace, name).Once().Return(nil)

	client := rfservice.NewRedisFailoverKubeClient(ms, log.Dummy, metrics.Dummy)

	assert.NoError(client.DestroyOrphanedSentinelNetworkPolicy(rf))
	ms.AssertExpectations(t)
}

func TestDestroyOrphanedSentinelNetworkPolicyToleratesNoneBeingThere(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()
	name := rfservice.GetSentinelNetworkPolicyName(rf)

	ms := &mK8SService.Services{}
	ms.On("GetNetworkPolicy", namespace, name).Once().Return(nil, errors.NewNotFound(
		schema.GroupResource{Group: "networking.k8s.io", Resource: "networkpolicies"}, name))

	client := rfservice.NewRedisFailoverKubeClient(ms, log.Dummy, metrics.Dummy)

	assert.NoError(client.DestroyOrphanedSentinelNetworkPolicy(rf))
	ms.AssertNotCalled(t, "DeleteNetworkPolicy", namespace, name)
}

package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	mK8SService "github.com/spotahome/redis-operator/mocks/service/k8s"
	rfservice "github.com/spotahome/redis-operator/operator/redisfailover/service"
)

func sentinelFailover(withStorage bool) *redisfailoverv1.RedisFailover {
	rf := &redisfailoverv1.RedisFailover{}
	rf.Name = "test"
	rf.Namespace = "testns"
	rf.Spec.Sentinel.Replicas = 3
	rf.Spec.Redis.Replicas = 3
	if withStorage {
		rf.Spec.Sentinel.Storage.PersistentVolumeClaim = &redisfailoverv1.EmbeddedPersistentVolumeClaim{
			EmbeddedObjectMetadata: redisfailoverv1.EmbeddedObjectMetadata{Name: "sentinel-config"},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("128Mi")},
				},
			},
		}
	}
	return rf
}

func storedSentinelSet(name string, withClaim bool) *appsv1.StatefulSet {
	ss := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "testns"}}
	if withClaim {
		ss.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{ObjectMeta: metav1.ObjectMeta{Name: "sentinel-config-writable"}},
		}
	}
	return ss
}

func TestTurningSentinelStorageOffDropsItsClaims(t *testing.T) {
	assert := assert.New(t)

	rf := sentinelFailover(false)
	stored := storedSentinelSet(rfservice.GetSentinelName(rf), true)

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSet", "testns", rfservice.GetSentinelName(rf)).Once().Return(stored, nil)
	ms.On("DeleteStatefulSetClaims", stored).Once().Return(nil)
	ms.On("CreateOrUpdateStatefulSet", "testns", mock.Anything).Once().Return(nil)
	ms.On("CreateOrUpdatePodDisruptionBudget", "testns", mock.Anything).Once().Return(nil)

	client := rfservice.NewRedisFailoverKubeClient(ms, log.Dummy, metrics.Dummy)
	assert.NoError(client.EnsureSentinelStatefulSet(rf, nil, nil))

	ms.AssertExpectations(t)
}

func TestLeavingSentinelStorageOnKeepsItsClaims(t *testing.T) {
	assert := assert.New(t)

	rf := sentinelFailover(true)
	stored := storedSentinelSet(rfservice.GetSentinelName(rf), true)

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSet", "testns", rfservice.GetSentinelName(rf)).Once().Return(stored, nil)
	ms.On("CreateOrUpdateStatefulSet", "testns", mock.Anything).Once().Return(nil)
	ms.On("CreateOrUpdatePodDisruptionBudget", "testns", mock.Anything).Once().Return(nil)

	client := rfservice.NewRedisFailoverKubeClient(ms, log.Dummy, metrics.Dummy)
	assert.NoError(client.EnsureSentinelStatefulSet(rf, nil, nil))

	ms.AssertNotCalled(t, "DeleteStatefulSetClaims", stored)
	ms.AssertExpectations(t)
}

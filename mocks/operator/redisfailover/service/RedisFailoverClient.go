// Code generated by mockery v2.32.0. DO NOT EDIT.

package mocks

import (
	mock "github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
)

// RedisFailoverClient is an autogenerated mock type for the RedisFailoverClient type
type RedisFailoverClient struct {
	mock.Mock
}

// EnsureHAProxyConfigmap provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureHAProxyConfigmap(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureHAProxyDeployment provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureHAProxyDeployment(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureHAProxyService provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureHAProxyService(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureNetworkPolicy provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureNetworkPolicy(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureNotPresentRedisService provides a mock function with given fields: rFailover
func (_m *RedisFailoverClient) EnsureNotPresentRedisService(rFailover *v1.RedisFailover) error {
	ret := _m.Called(rFailover)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover) error); ok {
		r0 = rf(rFailover)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureRedisConfigMap provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureRedisConfigMap(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureRedisHeadlessService provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureRedisHeadlessService(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureRedisReadinessConfigMap provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureRedisReadinessConfigMap(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureRedisService provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureRedisService(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureRedisShutdownConfigMap provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureRedisShutdownConfigMap(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureRedisStatefulset provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureRedisStatefulset(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureSentinelConfigMap provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureSentinelConfigMap(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureSentinelDeployment provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureSentinelDeployment(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EnsureSentinelService provides a mock function with given fields: rFailover, labels, ownerRefs
func (_m *RedisFailoverClient) EnsureSentinelService(rFailover *v1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	ret := _m.Called(rFailover, labels, ownerRefs)

	var r0 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover, map[string]string, []metav1.OwnerReference) error); ok {
		r0 = rf(rFailover, labels, ownerRefs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpdateStatus provides a mock function with given fields: rFailover
func (_m *RedisFailoverClient) UpdateStatus(rFailover *v1.RedisFailover) (*v1.RedisFailover, error) {
	ret := _m.Called(rFailover)

	var r0 *v1.RedisFailover
	var r1 error
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover) (*v1.RedisFailover, error)); ok {
		return rf(rFailover)
	}
	if rf, ok := ret.Get(0).(func(*v1.RedisFailover) *v1.RedisFailover); ok {
		r0 = rf(rFailover)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.RedisFailover)
		}
	}

	if rf, ok := ret.Get(1).(func(*v1.RedisFailover) error); ok {
		r1 = rf(rFailover)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewRedisFailoverClient creates a new instance of RedisFailoverClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewRedisFailoverClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *RedisFailoverClient {
	mock := &RedisFailoverClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

// Code generated by mockery v2.32.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// GetNumberRedisConnectedSlaves provides a mock function with given fields: ip, port
func (_m *Client) GetNumberRedisConnectedSlaves(ip string, port string) (int32, error) {
	ret := _m.Called(ip, port)

	var r0 int32
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string) (int32, error)); ok {
		return rf(ip, port)
	}
	if rf, ok := ret.Get(0).(func(string, string) int32); ok {
		r0 = rf(ip, port)
	} else {
		r0 = ret.Get(0).(int32)
	}

	if rf, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = rf(ip, port)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetNumberSentinelSlavesInMemory provides a mock function with given fields: ip, port
func (_m *Client) GetNumberSentinelSlavesInMemory(ip string, port string) (int32, error) {
	ret := _m.Called(ip, port)

	var r0 int32
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string) (int32, error)); ok {
		return rf(ip, port)
	}
	if rf, ok := ret.Get(0).(func(string, string) int32); ok {
		r0 = rf(ip, port)
	} else {
		r0 = ret.Get(0).(int32)
	}

	if rf, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = rf(ip, port)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetNumberSentinelsInMemory provides a mock function with given fields: ip, port
func (_m *Client) GetNumberSentinelsInMemory(ip string, port string) (int32, error) {
	ret := _m.Called(ip, port)

	var r0 int32
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string) (int32, error)); ok {
		return rf(ip, port)
	}
	if rf, ok := ret.Get(0).(func(string, string) int32); ok {
		r0 = rf(ip, port)
	} else {
		r0 = ret.Get(0).(int32)
	}

	if rf, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = rf(ip, port)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetSentinelMonitor provides a mock function with given fields: ip, port
func (_m *Client) GetSentinelMonitor(ip string, port string) (string, string, error) {
	ret := _m.Called(ip, port)

	var r0 string
	var r1 string
	var r2 error
	if rf, ok := ret.Get(0).(func(string, string) (string, string, error)); ok {
		return rf(ip, port)
	}
	if rf, ok := ret.Get(0).(func(string, string) string); ok {
		r0 = rf(ip, port)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string, string) string); ok {
		r1 = rf(ip, port)
	} else {
		r1 = ret.Get(1).(string)
	}

	if rf, ok := ret.Get(2).(func(string, string) error); ok {
		r2 = rf(ip, port)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// GetSlaveOf provides a mock function with given fields: ip, port, password
func (_m *Client) GetSlaveOf(ip string, port string, password string) (string, error) {
	ret := _m.Called(ip, port, password)

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string, string) (string, error)); ok {
		return rf(ip, port, password)
	}
	if rf, ok := ret.Get(0).(func(string, string, string) string); ok {
		r0 = rf(ip, port, password)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string, string, string) error); ok {
		r1 = rf(ip, port, password)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// IsMaster provides a mock function with given fields: ip, port, password
func (_m *Client) IsMaster(ip string, port string, password string) (bool, error) {
	ret := _m.Called(ip, port, password)

	var r0 bool
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string, string) (bool, error)); ok {
		return rf(ip, port, password)
	}
	if rf, ok := ret.Get(0).(func(string, string, string) bool); ok {
		r0 = rf(ip, port, password)
	} else {
		r0 = ret.Get(0).(bool)
	}

	if rf, ok := ret.Get(1).(func(string, string, string) error); ok {
		r1 = rf(ip, port, password)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MakeMaster provides a mock function with given fields: ip, port, password
func (_m *Client) MakeMaster(ip string, port string, password string) error {
	ret := _m.Called(ip, port, password)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string) error); ok {
		r0 = rf(ip, port, password)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MakeSlaveOf provides a mock function with given fields: ip, masterIP, password
func (_m *Client) MakeSlaveOf(ip string, masterIP string, password string) error {
	ret := _m.Called(ip, masterIP, password)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string) error); ok {
		r0 = rf(ip, masterIP, password)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MakeSlaveOfWithPort provides a mock function with given fields: ip, masterIP, masterPort, password
func (_m *Client) MakeSlaveOfWithPort(ip string, masterIP string, masterPort string, password string) error {
	ret := _m.Called(ip, masterIP, masterPort, password)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, string) error); ok {
		r0 = rf(ip, masterIP, masterPort, password)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MonitorRedis provides a mock function with given fields: ip, monitor, quorum, password, port
func (_m *Client) MonitorRedis(ip string, monitor string, quorum string, password string, port string) error {
	ret := _m.Called(ip, monitor, quorum, password, port)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, string, string) error); ok {
		r0 = rf(ip, monitor, quorum, password, port)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MonitorRedisWithPort provides a mock function with given fields: ip, monitor, port, quorum, password, sentinelPort
func (_m *Client) MonitorRedisWithPort(ip string, monitor string, port string, quorum string, password string, sentinelPort string) error {
	ret := _m.Called(ip, monitor, port, quorum, password, sentinelPort)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string, string, string, string) error); ok {
		r0 = rf(ip, monitor, port, quorum, password, sentinelPort)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ResetReplicaConnections provides a mock function with given fields: ip, port, password
func (_m *Client) ResetReplicaConnections(ip string, port string, password string) error {
	ret := _m.Called(ip, port, password)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, string) error); ok {
		r0 = rf(ip, port, password)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ResetSentinel provides a mock function with given fields: ip, port
func (_m *Client) ResetSentinel(ip string, port string) error {
	ret := _m.Called(ip, port)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(ip, port)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SentinelCheckQuorum provides a mock function with given fields: ip, port
func (_m *Client) SentinelCheckQuorum(ip string, port string) error {
	ret := _m.Called(ip, port)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(ip, port)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetCustomRedisConfig provides a mock function with given fields: ip, port, configs, password
func (_m *Client) SetCustomRedisConfig(ip string, port string, configs []string, password string) error {
	ret := _m.Called(ip, port, configs, password)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, []string, string) error); ok {
		r0 = rf(ip, port, configs, password)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetCustomSentinelConfig provides a mock function with given fields: ip, port, configs
func (_m *Client) SetCustomSentinelConfig(ip string, port string, configs []string) error {
	ret := _m.Called(ip, port, configs)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, []string) error); ok {
		r0 = rf(ip, port, configs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SlaveIsReady provides a mock function with given fields: ip, port, password
func (_m *Client) SlaveIsReady(ip string, port string, password string) (bool, error) {
	ret := _m.Called(ip, port, password)

	var r0 bool
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string, string) (bool, error)); ok {
		return rf(ip, port, password)
	}
	if rf, ok := ret.Get(0).(func(string, string, string) bool); ok {
		r0 = rf(ip, port, password)
	} else {
		r0 = ret.Get(0).(bool)
	}

	if rf, ok := ret.Get(1).(func(string, string, string) error); ok {
		r1 = rf(ip, port, password)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewClient creates a new instance of Client. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *Client {
	mock := &Client{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

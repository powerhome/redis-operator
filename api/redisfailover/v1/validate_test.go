package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name                   string
		rfName                 string
		rfBootstrapNode        *BootstrapSettings
		rfRedisCustomConfig    []string
		rfSentinelCustomConfig []string
		expectedError          string
		expectedBootstrapNode  *BootstrapSettings
		rfHAProxyConfig        *HaproxySettings
		expectedHAProxyConfig  *HaproxySettings
	}{
		{
			name:   "populates default values",
			rfName: "test",
		},
		{
			name:          "errors on too long of name",
			rfName:        "some-super-absurdely-unnecessarily-long-name-that-will-most-definitely-fail",
			expectedError: "name length can't be higher than 48",
		},
		{
			name:                   "SentinelCustomConfig provided",
			rfName:                 "test",
			rfSentinelCustomConfig: []string{"failover-timeout 500"},
		},
		{
			name:            "BootstrapNode provided without a host",
			rfName:          "test",
			rfBootstrapNode: &BootstrapSettings{Enabled: true},
			expectedError:   "BootstrapNode must include a host when provided",
		},
		{
			name:   "SentinelCustomConfig provided",
			rfName: "test",
		},
		{
			name:                  "Populates default bootstrap port when valid",
			rfName:                "test",
			rfBootstrapNode:       &BootstrapSettings{Host: "127.0.0.1", Enabled: true},
			expectedBootstrapNode: &BootstrapSettings{Host: "127.0.0.1", Port: "6379", Enabled: true},
		},
		{
			name:                  "Allows for specifying boostrap port",
			rfName:                "test",
			rfBootstrapNode:       &BootstrapSettings{Host: "127.0.0.1", Port: "6380", Enabled: true},
			expectedBootstrapNode: &BootstrapSettings{Host: "127.0.0.1", Port: "6380", Enabled: true},
		},
		{
			name:                "Appends applied custom config to default initial values",
			rfName:              "test",
			rfRedisCustomConfig: []string{"tcp-keepalive 60"},
		},
		{
			name:                  "Appends applied custom config to default initial values when bootstrapping",
			rfName:                "test",
			rfRedisCustomConfig:   []string{"tcp-keepalive 60"},
			rfBootstrapNode:       &BootstrapSettings{Host: "127.0.0.1", Enabled: true},
			expectedBootstrapNode: &BootstrapSettings{Host: "127.0.0.1", Port: "6379", Enabled: true},
		},
		{
			name:                  "HAProxy config no image provided",
			rfName:                "test",
			rfHAProxyConfig:       &HaproxySettings{},
			expectedHAProxyConfig: &HaproxySettings{Image: defaultHAProxyImage},
		},
		{
			name:                  "HAProxy config custom image",
			rfName:                "test",
			rfHAProxyConfig:       &HaproxySettings{Image: "haproxy:0.0.1"},
			expectedHAProxyConfig: &HaproxySettings{Image: "haproxy:0.0.1"},
		},
		{
			name:   "HAProxy Exporter default port",
			rfName: "test",
			rfHAProxyConfig: &HaproxySettings{
				Image: "_",
				Exporter: &HaproxyExporterSettings{
					Enabled: true,
				},
			},
			expectedHAProxyConfig: &HaproxySettings{
				Image: "_",
				Exporter: &HaproxyExporterSettings{
					Enabled: true,
					Port:    DefaultHaproxyExporterPort,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)
			rf := generateRedisFailover(test.rfName, test.rfBootstrapNode)
			rf.Spec.Redis.CustomConfig = test.rfRedisCustomConfig
			rf.Spec.Sentinel.CustomConfig = test.rfSentinelCustomConfig
			rf.Spec.Haproxy = test.rfHAProxyConfig
			err := rf.Validate()

			if test.expectedError == "" {
				assert.NoError(err)

				expectedRedisCustomConfig := []string{
					"replica-priority 100",
				}

				if test.rfBootstrapNode != nil && test.rfBootstrapNode.Enabled {
					expectedRedisCustomConfig = []string{
						"replica-priority 0",
					}
				}

				expectedRedisCustomConfig = append(expectedRedisCustomConfig, test.rfRedisCustomConfig...)
				expectedSentinelCustomConfig := defaultSentinelCustomConfig
				if len(test.rfSentinelCustomConfig) > 0 {
					expectedSentinelCustomConfig = test.rfSentinelCustomConfig
				}

				expectedRF := &RedisFailover{
					ObjectMeta: metav1.ObjectMeta{
						Name:      test.rfName,
						Namespace: "namespace",
					},
					Spec: RedisFailoverSpec{
						Redis: RedisSettings{
							Image:    defaultImage,
							Replicas: defaultRedisNumber,
							Port:     defaultRedisPort,
							Exporter: Exporter{
								Image: defaultExporterImage,
							},
							CustomConfig: expectedRedisCustomConfig,
						},
						Sentinel: SentinelSettings{
							Image:        defaultImage,
							Replicas:     defaultSentinelNumber,
							CustomConfig: expectedSentinelCustomConfig,
							Exporter: Exporter{
								Image: defaultSentinelExporterImage,
							},
							Port: Port(26379),
						},
						Haproxy:       test.expectedHAProxyConfig,
						BootstrapNode: test.expectedBootstrapNode,
					},
				}

				assert.Equal(expectedRF, rf)
			} else {
				if assert.Error(err) {
					assert.Contains(test.expectedError, err.Error())
				}
			}
		})
	}
}

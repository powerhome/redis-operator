package redisfailover

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/metrics"
)

func failoverWithPolicyNamespaces(name string) *redisfailoverv1.RedisFailover {
	return &redisfailoverv1.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "testns"},
		Spec: redisfailoverv1.RedisFailoverSpec{
			NetworkPolicyNsList: []redisfailoverv1.NetworkPolicyNamespaceEntry{
				{MatchLabelKey: "app.kubernetes.io/name", MatchLabelValue: "somewhere"},
			},
		},
	}
}

func TestNetworkPolicyNsListIsAnnouncedOncePerFailover(t *testing.T) {
	assert := assert.New(t)

	logger := &countingLogger{}
	handler := NewRedisFailoverHandler(Config{}, nil, nil, nil, nil, metrics.Dummy, logger)

	rf := failoverWithPolicyNamespaces("one")
	for i := 0; i < 5; i++ {
		handler.warnNetworkPolicyNsListIsVestigial(rf)
	}

	assert.Equal(1, logger.mentioning("networkPolicyNsList"),
		"five passes over one failover should say it once")

	// A second failover is a second reader who has not been told.
	handler.warnNetworkPolicyNsListIsVestigial(failoverWithPolicyNamespaces("two"))
	assert.Equal(2, logger.mentioning("networkPolicyNsList"))
}

func TestNetworkPolicyNsListSaysNothingWhenTheFieldIsUnset(t *testing.T) {
	assert := assert.New(t)

	logger := &countingLogger{}
	handler := NewRedisFailoverHandler(Config{}, nil, nil, nil, nil, metrics.Dummy, logger)

	handler.warnNetworkPolicyNsListIsVestigial(&redisfailoverv1.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "testns"},
	})

	assert.Equal(0, logger.mentioning("networkPolicyNsList"))
}

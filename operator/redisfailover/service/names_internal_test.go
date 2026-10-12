package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
)

// A Service name is a DNS-1035 label, which Kubernetes rejects beyond 63
// characters, and `validate.go` accepts a RedisFailover name of up to 48. The
// ensurer destroys the Sentinel Deployment before it creates the headless
// service, so a name that cannot be created leaves the failover with no
// Sentinels at all, on that pass and every one after it.
func TestSentinelHeadlessNameFitsADNSLabel(t *testing.T) {
	const dns1035LabelMax = 63
	const longestFailoverName = 48 // maxNameLength in api/redisfailover/v1/validate.go

	rf := &redisfailoverv1.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("a", longestFailoverName)},
	}

	assert.LessOrEqual(t, len(GetSentinelHeadlessName(rf)), dns1035LabelMax,
		"the headless service governing the Sentinel set has to be creatable for every name the API accepts")
}

// Every name the operator builds from a RedisFailover's own name shares that
// budget, so they are checked together rather than one at a time.
func TestEveryGeneratedNameFitsADNSLabel(t *testing.T) {
	const dns1035LabelMax = 63
	const longestFailoverName = 48

	rf := &redisfailoverv1.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("a", longestFailoverName)},
	}

	for name, built := range map[string]string{
		"redis":             GetRedisName(rf),
		"redis headless":    GetRedisHeadlessName(rf),
		"redis master":      GetRedisMasterName(rf),
		"redis shutdown":    GetRedisShutdownConfigMapName(rf),
		"sentinel":          GetSentinelName(rf),
		"sentinel headless": GetSentinelHeadlessName(rf),
		"haproxy master":    GetHaproxyMasterName(rf),
	} {
		assert.LessOrEqual(t, len(built), dns1035LabelMax, "%s name %q", name, built)
	}
}

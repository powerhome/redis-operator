package service_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	mK8SService "github.com/spotahome/redis-operator/mocks/service/k8s"
	mRedisService "github.com/spotahome/redis-operator/mocks/service/redis"
	rfservice "github.com/spotahome/redis-operator/operator/redisfailover/service"
)

func generateRF() *redisfailoverv1.RedisFailover {
	return &redisfailoverv1.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: redisfailoverv1.RedisFailoverSpec{
			Redis: redisfailoverv1.RedisSettings{
				Replicas: int32(3),
			},
			Sentinel: redisfailoverv1.SentinelSettings{
				Replicas: int32(3),
				Port:     redisfailoverv1.Port(26379),
			},
			Haproxy: &redisfailoverv1.HaproxySettings{
				Replicas: int32(3),
			},
		},
	}
}

// A node that is not ready yet is worth skipping past; another may answer and
// the condition clears on its own. A node refusing the credential will keep
// refusing until its pod restarts, so reporting zero masters and no error hides
// the one fault the caller can repair.
func TestGetNumberMastersSurfacesRefusedCredentials(t *testing.T) {
	tests := []struct {
		name      string
		podErr    error
		anyMaster bool
		wantErr   bool
	}{
		{
			name:    "every node refuses the credential",
			podErr:  errors.New("WRONGPASS invalid username-password pair or user is disabled."),
			wantErr: true,
		},
		{
			name:    "every node wants a password we did not send",
			podErr:  errors.New("NOAUTH Authentication required."),
			wantErr: true,
		},
		{
			// Unreachable is the case the existing skip-and-continue is for.
			name:    "a node is simply unreachable",
			podErr:  errors.New("dial tcp 10.0.0.1:6379: connect: connection refused"),
			wantErr: false,
		},
		{
			// Mid-rotation some pods hold the new password and some the old. If
			// a master still answers there is nothing to repair.
			name:      "a master answers despite another refusing",
			podErr:    errors.New("WRONGPASS invalid username-password pair or user is disabled."),
			anyMaster: true,
			wantErr:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			rf := generateRF()
			rf.Spec.Auth.SecretPath = "redis-auth"

			ms := &mK8SService.Services{}
			ms.On("GetStatefulSetPods", rf.Namespace, mock.Anything).Once().Return(&corev1.PodList{
				Items: []corev1.Pod{
					{Status: corev1.PodStatus{PodIP: "1.1.1.1", Phase: corev1.PodRunning}},
					{Status: corev1.PodStatus{PodIP: "1.1.1.2", Phase: corev1.PodRunning}},
				},
			}, nil)
			ms.On("GetSecret", rf.Namespace, "redis-auth").Once().Return(&corev1.Secret{
				Data: map[string][]byte{"password": []byte("s3cr3tpass")},
			}, nil)

			mr := &mRedisService.Client{}
			mr.On("IsMaster", "1.1.1.1", mock.Anything, mock.Anything).Once().Return(false, test.podErr)
			if test.anyMaster {
				mr.On("IsMaster", "1.1.1.2", mock.Anything, mock.Anything).Once().Return(true, nil)
			} else {
				mr.On("IsMaster", "1.1.1.2", mock.Anything, mock.Anything).Once().Return(false, test.podErr)
			}

			checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)
			_, err := checker.GetNumberMasters(rf)

			if test.wantErr {
				assert.Error(err, "a refused credential has to reach the caller, which is the only thing that can fix it")
			} else {
				assert.NoError(err)
			}
		})
	}
}

// A pod is stale when the password it was built for is not the one the failover
// is configured with, which is what says a restart would change something. The
// answer decides both whether the pods are restarted at all and whether a
// refusal is reported instead, so the two ends of the comparison have to agree
// on what "no password" looks like.
func TestGetRedisesPodsWithStalePassword(t *testing.T) {
	current := func(password string) string {
		sum := sha256.Sum256([]byte(password))
		return hex.EncodeToString(sum[:])
	}("s3cr3tpass")

	running := func(name string, annotations map[string]string) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
	}

	deleting := time.Now()

	tests := []struct {
		name       string
		secretPath string
		pods       []corev1.Pod
		want       []string
	}{
		{
			name:       "none when every pod carries the configured password",
			secretPath: "redis-auth",
			pods: []corev1.Pod{
				running("rfr-test-0", map[string]string{"checksum/redis-password": current}),
				running("rfr-test-1", map[string]string{"checksum/redis-password": current}),
			},
			want: []string{},
		},
		{
			name:       "the pods built for a different password",
			secretPath: "redis-auth",
			pods: []corev1.Pod{
				running("rfr-test-0", map[string]string{"checksum/redis-password": current}),
				running("rfr-test-1", map[string]string{"checksum/redis-password": "older-digest"}),
			},
			want: []string{"rfr-test-1"},
		},
		{
			name:       "a pod built before the failover had a password",
			secretPath: "redis-auth",
			pods:       []corev1.Pod{running("rfr-test-0", nil)},
			want:       []string{"rfr-test-0"},
		},
		{
			// A failover with no auth.secretPath builds its pods without the
			// annotation, so nothing about them is out of date. Reporting them
			// stale would restart every pod on every pass, and would disarm the
			// guard that stops the operator restarting in a loop when Redis
			// refuses a password a restart cannot fix.
			name:       "none when the failover has no password and neither do its pods",
			secretPath: "",
			pods: []corev1.Pod{
				running("rfr-test-0", nil),
				running("rfr-test-1", map[string]string{"other/annotation": "kept"}),
			},
			want: []string{},
		},
		{
			// Giving up a password is a change like any other: the pods still
			// carrying one have to restart without it.
			name:       "the pods still carrying a password the failover has given up",
			secretPath: "",
			pods: []corev1.Pod{
				running("rfr-test-0", map[string]string{"checksum/redis-password": current}),
				running("rfr-test-1", nil),
			},
			want: []string{"rfr-test-0"},
		},
		{
			// Restarting either is meaningless: one is not serving yet and the
			// other is already on its way out.
			name:       "pods that are not running or are already being deleted",
			secretPath: "redis-auth",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rfr-test-0"},
					Status:     corev1.PodStatus{Phase: corev1.PodPending},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "rfr-test-1",
						DeletionTimestamp: &metav1.Time{Time: deleting},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				running("rfr-test-2", map[string]string{"checksum/redis-password": "older-digest"}),
			},
			want: []string{"rfr-test-2"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			rf := generateRF()
			rf.Spec.Auth.SecretPath = test.secretPath

			ms := &mK8SService.Services{}
			if test.secretPath != "" {
				ms.On("GetSecret", rf.Namespace, test.secretPath).Once().Return(&corev1.Secret{
					Data: map[string][]byte{"password": []byte("s3cr3tpass")},
				}, nil)
			}
			ms.On("GetStatefulSetPods", rf.Namespace, mock.Anything).Once().Return(&corev1.PodList{Items: test.pods}, nil)

			checker := rfservice.NewRedisFailoverChecker(ms, &mRedisService.Client{}, log.DummyLogger{}, metrics.Dummy)
			stale, err := checker.GetRedisesPodsWithStalePassword(rf)

			assert.NoError(err)
			assert.Equal(test.want, stale)
		})
	}
}

func TestCheckRedisNumberError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSet", namespace, rfservice.GetRedisName(rf)).Once().Return(nil, errors.New(""))
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckRedisNumber(rf)
	assert.Error(err)
}

func TestCheckRedisNumberFalse(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	wrongNumber := int32(4)
	ss := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas: &wrongNumber,
		},
	}
	ms := &mK8SService.Services{}
	ms.On("GetStatefulSet", namespace, rfservice.GetRedisName(rf)).Once().Return(ss, nil)
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckRedisNumber(rf)
	assert.Error(err)
}

func TestCheckRedisNumberTrue(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	goodNumber := int32(3)
	ss := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas: &goodNumber,
		},
	}
	ms := &mK8SService.Services{}
	ms.On("GetStatefulSet", namespace, rfservice.GetRedisName(rf)).Once().Return(ss, nil)
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckRedisNumber(rf)
	assert.NoError(err)
}

func TestCheckSentinelNumberError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	ms.On("GetDeployment", namespace, rfservice.GetSentinelName(rf)).Once().Return(nil, errors.New(""))
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelNumber(rf)
	assert.Error(err)
}

func TestCheckSentinelNumberFalse(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	wrongNumber := int32(4)
	ss := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: &wrongNumber,
		},
	}
	ms := &mK8SService.Services{}
	ms.On("GetDeployment", namespace, rfservice.GetSentinelName(rf)).Once().Return(ss, nil)
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelNumber(rf)
	assert.Error(err)
}

func TestCheckSentinelNumberTrue(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	goodNumber := int32(3)
	ss := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: &goodNumber,
		},
	}
	ms := &mK8SService.Services{}
	ms.On("GetDeployment", namespace, rfservice.GetSentinelName(rf)).Once().Return(ss, nil)
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelNumber(rf)
	assert.NoError(err)
}

func TestCheckAllSlavesFromMasterGetStatefulSetError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(nil, errors.New(""))
	ms.On("UpdatePodLabels", namespace, mock.AnythingOfType("string"), mock.Anything).Once().Return(nil)
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckAllSlavesFromMaster("", rf)
	assert.Error(err)
}

func TestCheckAllSlavesFromMasterGetSlaveOfError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	ms.On("UpdatePodLabels", namespace, mock.AnythingOfType("string"), mock.Anything).Once().Return(nil)
	mr := &mRedisService.Client{}
	mr.On("GetSlaveOf", "", "0", "").Once().Return("", errors.New(""))

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckAllSlavesFromMaster("", rf)
	assert.Error(err)
}

func TestCheckAllSlavesFromMasterDifferentMaster(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	ms.On("UpdatePodLabels", namespace, mock.AnythingOfType("string"), mock.Anything).Once().Return(nil)
	mr := &mRedisService.Client{}
	mr.On("GetSlaveOf", "0.0.0.0", "0", "").Once().Return("1.1.1.1", nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckAllSlavesFromMaster("0.0.0.0", rf)
	assert.Error(err)
}

func TestCheckAllSlavesFromMaster(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	ms.On("UpdatePodLabels", namespace, mock.AnythingOfType("string"), mock.Anything).Once().Return(nil)
	mr := &mRedisService.Client{}
	mr.On("GetSlaveOf", "0.0.0.0", "0", "").Once().Return("1.1.1.1", nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckAllSlavesFromMaster("1.1.1.1", rf)
	assert.NoError(err)
}

func TestCheckSentinelNumberInMemoryGetDeploymentPodsError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetNumberSentinelsInMemory", "1.1.1.1", "26379").Once().Return(int32(0), errors.New("expected error"))

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelNumberInMemory("1.1.1.1", rf)
	assert.Error(err)
}

func TestCheckSentinelNumberInMemoryGetNumberSentinelInMemoryError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetNumberSentinelsInMemory", "1.1.1.1", "26379").Once().Return(int32(0), errors.New(""))

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelNumberInMemory("1.1.1.1", rf)
	assert.Error(err)
}

func TestCheckSentinelNumberInMemoryNumberMismatch(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetNumberSentinelsInMemory", "1.1.1.1", "26379").Once().Return(int32(4), nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelNumberInMemory("1.1.1.1", rf)
	assert.Error(err)
}

func TestCheckSentinelNumberInMemory(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetNumberSentinelsInMemory", "1.1.1.1", "26379").Once().Return(int32(3), nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelNumberInMemory("1.1.1.1", rf)
	assert.NoError(err)
}

func TestCheckSentinelSlavesNumberInMemoryGetNumberSentinelSlavesInMemoryError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetNumberSentinelSlavesInMemory", "1.1.1.1", "26379").Once().Return(int32(0), errors.New(""))

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelSlavesNumberInMemory("1.1.1.1", rf)
	assert.Error(err)
}

func TestCheckSentinelSlavesNumberInMemoryReplicasMismatch(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetNumberSentinelSlavesInMemory", "1.1.1.1", "26379").Once().Return(int32(3), nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelSlavesNumberInMemory("1.1.1.1", rf)
	assert.Error(err)
}

func TestCheckSentinelSlavesNumberInMemory(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()
	rf.Spec.Redis.Replicas = 5

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetNumberSentinelSlavesInMemory", "1.1.1.1", "26379").Once().Return(int32(4), nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelSlavesNumberInMemory("1.1.1.1", rf)
	assert.NoError(err)
}

func TestCheckSentinelMonitorGetSentinelMonitorError(t *testing.T) {
	assert := assert.New(t)

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetSentinelMonitor", "0.0.0.0", "26379").Once().Return("", "", errors.New(""))

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelMonitor("0.0.0.0", "26379", "1.1.1.1")
	assert.Error(err)
}

func TestCheckSentinelMonitorMismatch(t *testing.T) {
	assert := assert.New(t)

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetSentinelMonitor", "0.0.0.0", "26379").Once().Return("2.2.2.2", "6379", nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelMonitor("0.0.0.0", "26379", "1.1.1.1")
	assert.Error(err)
}

func TestCheckSentinelMonitor(t *testing.T) {
	assert := assert.New(t)

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetSentinelMonitor", "0.0.0.0", "26379").Once().Return("1.1.1.1", "6379", nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelMonitor("0.0.0.0", "26379", "1.1.1.1")
	assert.NoError(err)
}

func TestCheckSentinelMonitorWithPort(t *testing.T) {
	assert := assert.New(t)

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetSentinelMonitor", "0.0.0.0", "26379").Once().Return("1.1.1.1", "6379", nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelMonitor("0.0.0.0", "26379", "1.1.1.1", "6379")
	assert.NoError(err)
}

func TestCheckSentinelMonitorWithPortMismatch(t *testing.T) {
	assert := assert.New(t)

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetSentinelMonitor", "0.0.0.0", "26379").Once().Return("1.1.1.1", "6379", nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelMonitor("0.0.0.0", "26379", "0.0.0.0", "6379")
	assert.Error(err)
}

func TestCheckSentinelMonitorWithPortIPMismatch(t *testing.T) {
	assert := assert.New(t)

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetSentinelMonitor", "0.0.0.0", "26379").Once().Return("1.1.1.1", "6379", nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckSentinelMonitor("0.0.0.0", "26379", "1.1.1.1", "6380")
	assert.Error(err)
}

func TestCheckNumberRedisConnectedSlavesGeConnectedSlavesNumberError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetNumberRedisConnectedSlaves", "1.1.1.1", "0", "").Once().Return(int32(0), errors.New("expected error"))

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckNumberRedisConnectedSlaves("1.1.1.1", rf)
	assert.Error(err)
}

// The check reads the master's replication state, so it has to authenticate.
// Without a password it is answered NOAUTH on every reconcile of a failover that
// has one, which the caller reads as a slave-count mismatch and answers by
// killing the master's replica connections.
func TestCheckNumberRedisConnectedSlavesAuthenticates(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()
	rf.Spec.Auth.SecretPath = "redis-auth"

	ms := &mK8SService.Services{}
	ms.On("GetSecret", rf.Namespace, "redis-auth").Once().Return(&corev1.Secret{
		Data: map[string][]byte{"password": []byte("s3cr3tpass")},
	}, nil)

	mr := &mRedisService.Client{}
	mr.On("GetNumberRedisConnectedSlaves", "1.1.1.1", "0", "s3cr3tpass").Once().Return(rf.Spec.Redis.Replicas-1, nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckNumberRedisConnectedSlaves("1.1.1.1", rf)
	assert.NoError(err)
	mr.AssertExpectations(t)
}

func TestCheckNumberRedisConnectedSlavesGeConnectedSlavesNumberMismatch(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetNumberRedisConnectedSlaves", "1.1.1.1", "0", "").Once().Return(int32(rf.Spec.Redis.Replicas+1), nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckNumberRedisConnectedSlaves("1.1.1.1", rf)
	assert.Error(err)
}

func TestCheckNumberRedisConnectedSlaves(t *testing.T) {
	assert := assert.New(t)
	rf := generateRF()

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	mr.On("GetNumberRedisConnectedSlaves", "1.1.1.1", "0", "").Once().Return(rf.Spec.Redis.Replicas-1, nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	err := checker.CheckNumberRedisConnectedSlaves("1.1.1.1", rf)
	assert.NoError(err)
}

func TestGetMasterIPGetStatefulSetPodsError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(nil, errors.New(""))
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	_, err := checker.GetMasterIP(rf)
	assert.Error(err)
}

func TestGetMasterIPIsMasterError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	mr := &mRedisService.Client{}
	mr.On("IsMaster", "0.0.0.0", "0", "").Once().Return(false, errors.New(""))

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	_, err := checker.GetMasterIP(rf)
	assert.Error(err)
}

func TestGetMasterIPMultipleMastersError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	mr := &mRedisService.Client{}
	mr.On("IsMaster", "0.0.0.0", "0", "").Once().Return(true, nil)
	mr.On("IsMaster", "1.1.1.1", "0", "").Once().Return(true, nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	_, err := checker.GetMasterIP(rf)
	assert.Error(err)
}

func TestGetMasterIP(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	mr := &mRedisService.Client{}
	mr.On("IsMaster", "0.0.0.0", "0", "").Once().Return(true, nil)
	mr.On("IsMaster", "1.1.1.1", "0", "").Once().Return(false, nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	master, err := checker.GetMasterIP(rf)
	assert.NoError(err)
	assert.Equal("0.0.0.0", master, "the master should be the expected")
}

func TestGetNumberMastersGetStatefulSetPodsError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(nil, errors.New(""))
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	_, err := checker.GetNumberMasters(rf)
	assert.Error(err)
}

func TestGetNumberMastersIsMasterError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	mr := &mRedisService.Client{}
	mr.On("IsMaster", "0.0.0.0", "0", "").Once().Return(true, errors.New(""))

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	_, err := checker.GetNumberMasters(rf)
	assert.NoError(err)
}

func TestGetNumberMasters(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	mr := &mRedisService.Client{}
	mr.On("IsMaster", "0.0.0.0", "0", "").Once().Return(true, nil)
	mr.On("IsMaster", "1.1.1.1", "0", "").Once().Return(false, nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	masterNumber, err := checker.GetNumberMasters(rf)
	assert.NoError(err)
	assert.Equal(1, masterNumber, "the master number should be ok")
}

func TestGetNumberMastersTwo(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	mr := &mRedisService.Client{}
	mr.On("IsMaster", "0.0.0.0", "0", "").Once().Return(true, nil)
	mr.On("IsMaster", "1.1.1.1", "0", "").Once().Return(true, nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	masterNumber, err := checker.GetNumberMasters(rf)
	assert.NoError(err)
	assert.Equal(2, masterNumber, "the master number should be ok")
}

func TestGetMaxRedisPodTimeGetStatefulSetPodsError(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(nil, errors.New(""))
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	_, err := checker.GetMaxRedisPodTime(rf)
	assert.Error(err)
}

func TestGetMaxRedisPodTime(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	now := time.Now()
	oneHour := now.Add(-1 * time.Hour)
	oneMinute := now.Add(-1 * time.Minute)

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					StartTime: &metav1.Time{
						Time: oneHour,
					},
				},
			},
			{
				Status: corev1.PodStatus{
					StartTime: &metav1.Time{
						Time: oneMinute,
					},
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	maxTime, err := checker.GetMaxRedisPodTime(rf)
	assert.NoError(err)

	expected := now.Sub(oneHour).Round(time.Second)
	assert.Equal(expected, maxTime.Round(time.Second), "the closest time should be given")
}

func TestGetRedisPodsNames(t *testing.T) {
	assert := assert.New(t)
	rf := generateRF()

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "slave1",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					PodIP: "0.0.0.0",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "master",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					PodIP: "1.1.1.1",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "slave2",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					PodIP: "0.0.0.0",
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	mr := &mRedisService.Client{}
	mr.On("IsMaster", "0.0.0.0", "0", "").Twice().Return(false, nil)
	mr.On("IsMaster", "1.1.1.1", "0", "").Once().Return(true, nil)

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)
	master, err := checker.GetRedisesMasterPod(rf)

	assert.NoError(err)

	assert.Equal(master, "master")

	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(pods, nil)
	mr.On("IsMaster", "0.0.0.0", "0", "").Twice().Return(false, nil)
	mr.On("IsMaster", "1.1.1.1", "0", "").Once().Return(true, nil)

	namePods, err := checker.GetRedisesSlavesPods(rf)

	assert.NoError(err)

	assert.Equal(namePods, []string{"slave1", "slave2"})
}

func TestGetStatefulSetUpdateRevision(t *testing.T) {
	tests := []struct {
		name             string
		ss               *appsv1.StatefulSet
		expectedUVersion string
		expectedError    error
	}{
		{
			name: "revision ok",
			ss: &appsv1.StatefulSet{
				Status: appsv1.StatefulSetStatus{
					UpdateRevision: "10",
				},
			},
			expectedUVersion: "10",
			expectedError:    nil,
		},
		{
			name:             "no stateful set",
			ss:               nil,
			expectedUVersion: "",
			expectedError:    errors.New("not found"),
		},
	}

	for _, test := range tests {
		assert := assert.New(t)

		rf := generateRF()
		ms := &mK8SService.Services{}
		ms.On("GetStatefulSet", namespace, rfservice.GetRedisName(rf)).Once().Return(test.ss, nil)
		mr := &mRedisService.Client{}

		checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)
		version, err := checker.GetStatefulSetUpdateRevision(rf)

		if test.expectedError == nil {
			assert.NoError(err)
		} else {
			assert.Error(err)
		}

		assert.Equal(version, test.expectedUVersion)
	}

}

func TestGetRedisRevisionHash(t *testing.T) {
	tests := []struct {
		name          string
		pod           *corev1.Pod
		expectedHash  string
		expectedError error
	}{
		{
			name: "has ok",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						appsv1.ControllerRevisionHashLabelKey: "10",
					},
				},
			},
			expectedHash:  "10",
			expectedError: nil,
		},
		{
			name:          "no pod",
			pod:           nil,
			expectedHash:  "",
			expectedError: errors.New("not found"),
		},
	}

	for _, test := range tests {
		assert := assert.New(t)

		rf := generateRF()
		ms := &mK8SService.Services{}
		ms.On("GetPod", namespace, "namepod").Once().Return(test.pod, nil)
		mr := &mRedisService.Client{}

		checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)
		hash, err := checker.GetRedisRevisionHash("namepod", rf)

		if test.expectedError == nil {
			assert.NoError(err)
		} else {
			assert.Error(err)
		}

		assert.Equal(hash, test.expectedHash)
	}

}

func TestClusterRunning(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	allRunning := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	notAllRunning := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodPending,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	notAllReplicas := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	mr := &mRedisService.Client{}

	ms := &mK8SService.Services{}
	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(allRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(allRunning, nil)
	ms.On("GetDeploymentPods", namespace, rfservice.GetHaproxyMasterName(rf)).Once().Return(allRunning, nil)
	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	assert.True(checker.IsClusterRunning(rf))

	ms = &mK8SService.Services{}
	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(allRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(notAllReplicas, nil)
	ms.On("GetDeploymentPods", namespace, rfservice.GetHaproxyMasterName(rf)).Once().Return(allRunning, nil)
	checker = rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	assert.False(checker.IsClusterRunning(rf))

	ms = &mK8SService.Services{}
	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(notAllRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(allRunning, nil)
	ms.On("GetDeploymentPods", namespace, rfservice.GetHaproxyMasterName(rf)).Once().Return(notAllReplicas, nil)
	checker = rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	assert.False(checker.IsClusterRunning(rf))

	rf.Spec.Haproxy = nil
	ms = &mK8SService.Services{}
	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(allRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(allRunning, nil)
	checker = rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	assert.True(checker.IsClusterRunning(rf))
}

func TestClusterRunningWithBootstrap(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	allRunning := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	notAllRunning := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodPending,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	notAllReplicas := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}
	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	// When bootstrapping and sentinels are disabled
	rf.Spec.BootstrapNode = &redisfailoverv1.BootstrapSettings{
		Host:           "fake-host",
		AllowSentinels: false,
		Enabled:        true,
	}
	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(notAllRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(notAllRunning, nil)
	assert.False(checker.IsClusterRunning(rf))

	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(notAllRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(notAllReplicas, nil)
	assert.False(checker.IsClusterRunning(rf))

	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(notAllRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(allRunning, nil)
	ms.On("GetDeploymentPods", namespace, rfservice.GetHaproxyMasterName(rf)).Once().Return(allRunning, nil)

	assert.True(checker.IsClusterRunning(rf))
}

func TestClusterRunningWithBootstrapSentinels(t *testing.T) {
	assert := assert.New(t)

	rf := generateRF()

	allRunning := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	notAllRunning := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodPending,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	notAllReplicas := &corev1.PodList{
		Items: []corev1.Pod{
			{
				Status: corev1.PodStatus{
					PodIP: "0.0.0.0",
					Phase: corev1.PodRunning,
				},
			},
			{
				Status: corev1.PodStatus{
					PodIP: "1.1.1.1",
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	ms := &mK8SService.Services{}
	mr := &mRedisService.Client{}

	checker := rfservice.NewRedisFailoverChecker(ms, mr, log.DummyLogger{}, metrics.Dummy)

	rf.Spec.BootstrapNode = &redisfailoverv1.BootstrapSettings{
		Host:           "fake-host",
		AllowSentinels: true,
		Enabled:        true,
	}
	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(allRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(allRunning, nil)
	assert.True(checker.IsClusterRunning(rf))

	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(notAllRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(notAllRunning, nil)
	assert.False(checker.IsClusterRunning(rf))

	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(notAllRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(notAllReplicas, nil)
	assert.False(checker.IsClusterRunning(rf))

	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(notAllRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(allRunning, nil)
	assert.False(checker.IsClusterRunning(rf))

	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(allRunning, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(notAllReplicas, nil)
	assert.False(checker.IsClusterRunning(rf))
	//
	ms.On("GetDeploymentPods", namespace, rfservice.GetSentinelName(rf)).Once().Return(notAllReplicas, nil)
	ms.On("GetStatefulSetPods", namespace, rfservice.GetRedisName(rf)).Once().Return(allRunning, nil)
	assert.False(checker.IsClusterRunning(rf))

}

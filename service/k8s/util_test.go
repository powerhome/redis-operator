package k8s_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	mK8SService "github.com/spotahome/redis-operator/mocks/service/k8s"
	"github.com/spotahome/redis-operator/service/k8s"
)

func TestGetRedisPassword(t *testing.T) {
	tests := []struct {
		name         string
		secretPath   string
		secretData   map[string][]byte
		wantPassword string
		wantErr      string
	}{
		{
			name:         "returns the password the secret holds",
			secretPath:   "redis-auth",
			secretData:   map[string][]byte{"password": []byte("s3cr3tpass")},
			wantPassword: "s3cr3tpass",
		},
		{
			name:         "returns a blank password when the failover asks for no auth",
			secretPath:   "",
			wantPassword: "",
		},
		{
			name:       "refuses a secret with no password field",
			secretPath: "redis-auth",
			secretData: map[string][]byte{"not-the-password": []byte("s3cr3tpass")},
			wantErr:    `secret "redis-auth" does not have a password field`,
		},
		{
			// Left alone this produces a Redis with no requirepass at all while
			// the failover reports itself healthy, and an HAProxy health check
			// that authenticates against a Redis expecting no password. Both
			// fail silently, so the misconfiguration has to surface here.
			name:       "refuses a secret whose password is empty",
			secretPath: "redis-auth",
			secretData: map[string][]byte{"password": []byte("")},
			wantErr:    `secret "redis-auth" has an empty password field`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			rf := &redisfailoverv1.RedisFailover{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "testns"},
				Spec: redisfailoverv1.RedisFailoverSpec{
					Auth: redisfailoverv1.AuthSettings{SecretPath: test.secretPath},
				},
			}

			ms := &mK8SService.Services{}
			if test.secretPath == "" {
				// The secret is never read when no auth is asked for.
				ms.On("GetSecret", mock.Anything, mock.Anything).Return(nil, errors.New("should not be called"))
			} else {
				ms.On("GetSecret", "testns", test.secretPath).Once().Return(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: test.secretPath, Namespace: "testns"},
					Data:       test.secretData,
				}, nil)
			}

			password, err := k8s.GetRedisPassword(ms, rf)

			if test.wantErr != "" {
				assert.EqualError(err, test.wantErr)
				assert.Empty(password)
				return
			}

			assert.NoError(err)
			assert.Equal(test.wantPassword, password)
		})
	}
}

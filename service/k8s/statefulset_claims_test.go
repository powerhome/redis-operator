package k8s_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"

	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	"github.com/spotahome/redis-operator/service/k8s"
)

func setWithClaims(name string, claimNames ...string) *appsv1.StatefulSet {
	claims := []v1.PersistentVolumeClaim{}
	for _, claimName := range claimNames {
		claims = append(claims, v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: claimName},
			Spec: v1.PersistentVolumeClaimSpec{
				Resources: v1.VolumeResourceRequirements{
					Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		})
	}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, ResourceVersion: "10"},
		Spec:       appsv1.StatefulSetSpec{VolumeClaimTemplates: claims},
	}
}

// A statefulset's volumeClaimTemplates are immutable, so the desired ones are
// overwritten with the stored ones before an update. A claim being added would
// otherwise be discarded while the pod template that mounts it is accepted,
// and Kubernetes then refuses the next pod the set creates.
func TestAddingAClaimReplacesTheStatefulSet(t *testing.T) {
	assert := assert.New(t)

	stored := setWithClaims("rfs-test")
	desired := setWithClaims("rfs-test", "sentinel-config-writable")

	deleted := false
	mcli := fake.NewSimpleClientset()
	mcli.PrependReactor("get", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, stored, nil
	})
	mcli.PrependReactor("delete", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		deleted = true
		return true, nil, nil
	})
	mcli.PrependReactor("update", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		assert.Fail("the set was updated in place, which cannot carry a new claim template")
		return true, nil, nil
	})

	service := k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)

	assert.NoError(service.CreateOrUpdateStatefulSet("testns", desired))
	assert.True(deleted, "the set is replaced so the next reconcile creates it with the claim")
}

// Removing storage has the mirror problem: the stored claim template would
// survive while the scratch volume returns under the same name.
func TestRemovingAClaimReplacesTheStatefulSet(t *testing.T) {
	assert := assert.New(t)

	stored := setWithClaims("rfs-test", "sentinel-config-writable")
	desired := setWithClaims("rfs-test")

	deleted := false
	mcli := fake.NewSimpleClientset()
	mcli.PrependReactor("get", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, stored, nil
	})
	mcli.PrependReactor("delete", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		deleted = true
		return true, nil, nil
	})

	service := k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)

	assert.NoError(service.CreateOrUpdateStatefulSet("testns", desired))
	assert.True(deleted)
}

// A set whose claim templates have not changed is updated in place, which is
// every reconcile of every failover.
func TestAnUnchangedClaimUpdatesInPlace(t *testing.T) {
	assert := assert.New(t)

	stored := setWithClaims("rfs-test", "sentinel-config-writable")
	desired := setWithClaims("rfs-test", "sentinel-config-writable")

	updated := false
	mcli := fake.NewSimpleClientset()
	mcli.PrependReactor("get", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, stored, nil
	})
	mcli.PrependReactor("delete", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		assert.Fail("an unchanged set must not be replaced; that would orphan its pods every pass")
		return true, nil, nil
	})
	mcli.PrependReactor("update", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		updated = true
		return true, desired, nil
	})

	service := k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)

	assert.NoError(service.CreateOrUpdateStatefulSet("testns", desired))
	assert.True(updated)
}

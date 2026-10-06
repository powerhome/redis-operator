package k8s_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

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
// otherwise be discarded while the pod template still mounts it, which
// Kubernetes refuses outright.
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

// A Sentinel resumes from the configuration file on its claim, so a claim that
// outlives the storage it was created for would hand a restarted Sentinel a
// master that has since failed over. Claims are matched by the name Kubernetes
// gives them, so a set whose name is a prefix of another's must not reach its
// neighbour's claims.
func TestDeletingClaimsTakesOnlyTheSetsOwn(t *testing.T) {
	assert := assert.New(t)

	stored := setWithClaims("rfs-test", "sentinel-config-writable")
	stored.Namespace = "testns"

	mcli := fake.NewSimpleClientset(
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Name: "sentinel-config-writable-rfs-test-0", Namespace: "testns"}},
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Name: "sentinel-config-writable-rfs-test-1", Namespace: "testns"}},
		// A different failover whose set name starts with this one's.
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Name: "sentinel-config-writable-rfs-test-other-0", Namespace: "testns"}},
		// The Redis dataset, which no Sentinel change may ever remove.
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Name: "redis-data-rfr-test-0", Namespace: "testns"}},
	)

	service := k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)
	assert.NoError(service.DeleteStatefulSetClaims(stored))

	left := []string{}
	remaining, err := mcli.CoreV1().PersistentVolumeClaims("testns").List(context.Background(), metav1.ListOptions{})
	assert.NoError(err)
	for _, claim := range remaining.Items {
		left = append(left, claim.Name)
	}
	assert.ElementsMatch([]string{
		"sentinel-config-writable-rfs-test-other-0",
		"redis-data-rfr-test-0",
	}, left)
}

// Nothing to delete is the common case, and a set that never declared storage
// has no template to find claims by.
func TestDeletingClaimsOfASetWithoutStorageDoesNothing(t *testing.T) {
	assert := assert.New(t)

	mcli := fake.NewSimpleClientset()
	mcli.PrependReactor("list", "persistentvolumeclaims", func(kubetesting.Action) (bool, runtime.Object, error) {
		assert.Fail("a set with no claim templates has no claims to look for")
		return true, nil, nil
	})

	service := k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)
	assert.NoError(service.DeleteStatefulSetClaims(setWithClaims("rfs-test")))
}

// Growing a claim is one way: a neighbour reached by mistake cannot be put
// back, which makes the resize path the costlier user of the same matching.
func TestResizingAClaimLeavesANeighbouringFailoverAlone(t *testing.T) {
	assert := assert.New(t)

	stored := setWithClaims("rfr-test", "redis-data")
	stored.Namespace = "testns"
	stored.Annotations = map[string]string{"storageCapacity": "1073741824"}

	desired := setWithClaims("rfr-test", "redis-data")
	desired.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests = v1.ResourceList{
		v1.ResourceStorage: resource.MustParse("2Gi"),
	}

	claim := func(name string) *v1.PersistentVolumeClaim {
		return &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "testns"},
			Spec: v1.PersistentVolumeClaimSpec{
				Resources: v1.VolumeResourceRequirements{
					Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		}
	}
	mcli := fake.NewSimpleClientset(
		claim("redis-data-rfr-test-0"),
		claim("redis-data-rfr-test-other-0"),
	)
	mcli.PrependReactor("get", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, stored, nil
	})

	// Growing a claim ends by replacing the set, because a claim template is
	// immutable.
	replaced := false
	mcli.PrependReactor("delete", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		replaced = true
		return true, nil, nil
	})

	resized := []string{}
	mcli.PrependReactor("update", "persistentvolumeclaims", func(action kubetesting.Action) (bool, runtime.Object, error) {
		updated := action.(kubetesting.UpdateAction).GetObject().(*v1.PersistentVolumeClaim)
		resized = append(resized, updated.Name)
		return true, updated, nil
	})

	service := k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)
	assert.NoError(service.CreateOrUpdateStatefulSet("testns", desired))

	assert.Equal([]string{"redis-data-rfr-test-0"}, resized)
	assert.True(replaced, "the set is replaced so the resized claim takes effect")
}

// Asking for more pods in the same update that adds a claim template asks the
// set for pods it cannot make: the replacement adopts pods Kubernetes will not
// let it update, and it creates none above the lowest one it cannot reconcile.
// The count goes first so the set is whole when the claims change.
func TestGrowingAndAddingAClaimTogetherGrowsFirst(t *testing.T) {
	assert := assert.New(t)

	stored := setWithClaims("rfr-test")
	stored.Namespace = "testns"
	stored.Spec.Replicas = ptr.To(int32(2))

	desired := setWithClaims("rfr-test", "redis-data")
	desired.Spec.Replicas = ptr.To(int32(4))

	mcli := fake.NewSimpleClientset()
	mcli.PrependReactor("get", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, stored, nil
	})
	mcli.PrependReactor("delete", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		assert.Fail("the set was replaced while it was still short of its pods")
		return true, nil, nil
	})

	var applied *appsv1.StatefulSet
	mcli.PrependReactor("update", "statefulsets", func(action kubetesting.Action) (bool, runtime.Object, error) {
		applied = action.(kubetesting.UpdateAction).GetObject().(*appsv1.StatefulSet)
		return true, applied, nil
	})

	service := k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)
	assert.NoError(service.CreateOrUpdateStatefulSet("testns", desired))

	assert.NotNil(applied, "the set is updated to the new count")
	assert.Equal(int32(4), *applied.Spec.Replicas)
	assert.Empty(applied.Spec.VolumeClaimTemplates, "the claims wait for the pods to exist")
}

// Shrinking needs no new pod, so the claims change in the same pass.
func TestShrinkingAndRemovingAClaimTogetherReplacesTheSet(t *testing.T) {
	assert := assert.New(t)

	stored := setWithClaims("rfr-test", "redis-data")
	stored.Namespace = "testns"
	stored.Spec.Replicas = ptr.To(int32(4))

	desired := setWithClaims("rfr-test")
	desired.Spec.Replicas = ptr.To(int32(2))

	replaced := false
	mcli := fake.NewSimpleClientset()
	mcli.PrependReactor("get", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, stored, nil
	})
	mcli.PrependReactor("delete", "statefulsets", func(kubetesting.Action) (bool, runtime.Object, error) {
		replaced = true
		return true, nil, nil
	})

	service := k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)
	assert.NoError(service.CreateOrUpdateStatefulSet("testns", desired))
	assert.True(replaced)
}

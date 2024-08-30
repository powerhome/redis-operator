package k8s_test

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	v1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubernetes "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"

	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	"github.com/spotahome/redis-operator/service/k8s"

	corev1 "k8s.io/api/core/v1"
)

var (
	statefulSetsGroup          = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	persistentVolumeClaimGroup = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}
)

func newStatefulSetUpdateAction(ns string, statefulSet *appsv1.StatefulSet) kubetesting.UpdateActionImpl {
	return kubetesting.NewUpdateAction(statefulSetsGroup, ns, statefulSet)
}

func newStatefulSetGetAction(ns, name string) kubetesting.GetActionImpl {
	return kubetesting.NewGetAction(statefulSetsGroup, ns, name)
}

func newStatefulSetCreateAction(ns string, statefulSet *appsv1.StatefulSet) kubetesting.CreateActionImpl {
	return kubetesting.NewCreateAction(statefulSetsGroup, ns, statefulSet)
}

func newStatefulSetDeleteAction(ns string, name string) kubetesting.DeleteActionImpl {
	propagation := metav1.DeletePropagationForeground
	return kubetesting.NewDeleteActionWithOptions(statefulSetsGroup, ns, name, metav1.DeleteOptions{PropagationPolicy: &propagation})
}

func newPVCUpdateAction(pvc *corev1.PersistentVolumeClaim) kubetesting.UpdateActionImpl {
	return kubetesting.NewUpdateAction(persistentVolumeClaimGroup, "", pvc)
}

func newPVCListAction(opts metav1.ListOptions) kubetesting.ListActionImpl {
	return kubetesting.NewListAction(persistentVolumeClaimGroup, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}, "", opts)
}

func TestStatefulSetServiceGetCreateOrUpdate(t *testing.T) {
	testStatefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "teststatefulSet1",
			ResourceVersion: "10",
		},
	}

	testns := "testns"

	tests := []struct {
		name                 string
		statefulSet          *appsv1.StatefulSet
		getStatefulSetResult *appsv1.StatefulSet
		errorOnGet           error
		errorOnCreation      error
		expActions           []kubetesting.Action
		expErr               bool
	}{
		{
			name:                 "A new statefulSet should create a new statefulSet.",
			statefulSet:          testStatefulSet,
			getStatefulSetResult: nil,
			errorOnGet:           kubeerrors.NewNotFound(schema.GroupResource{}, ""),
			errorOnCreation:      nil,
			expActions: []kubetesting.Action{
				newStatefulSetGetAction(testns, testStatefulSet.ObjectMeta.Name),
				newStatefulSetCreateAction(testns, testStatefulSet),
			},
			expErr: false,
		},
		{
			name:                 "A new statefulSet should error when create a new statefulSet fails.",
			statefulSet:          testStatefulSet,
			getStatefulSetResult: nil,
			errorOnGet:           kubeerrors.NewNotFound(schema.GroupResource{}, ""),
			errorOnCreation:      errors.New("wanted error"),
			expActions: []kubetesting.Action{
				newStatefulSetGetAction(testns, testStatefulSet.ObjectMeta.Name),
				newStatefulSetCreateAction(testns, testStatefulSet),
			},
			expErr: true,
		},
		{
			name:                 "An existent statefulSet should update the statefulSet.",
			statefulSet:          testStatefulSet,
			getStatefulSetResult: testStatefulSet,
			errorOnGet:           nil,
			errorOnCreation:      nil,
			expActions: []kubetesting.Action{
				newStatefulSetGetAction(testns, testStatefulSet.ObjectMeta.Name),
				newStatefulSetUpdateAction(testns, testStatefulSet),
			},
			expErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			// Mock.
			mcli := &kubernetes.Clientset{}
			mcli.AddReactor("get", "statefulsets", func(action kubetesting.Action) (bool, runtime.Object, error) {
				return true, test.getStatefulSetResult, test.errorOnGet
			})
			mcli.AddReactor("create", "statefulsets", func(action kubetesting.Action) (bool, runtime.Object, error) {
				return true, nil, test.errorOnCreation
			})

			service := k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)
			err := service.CreateOrUpdateStatefulSet(testns, test.statefulSet)

			if test.expErr {
				assert.Error(err)
			} else {
				assert.NoError(err)
				// Check calls to kubernetes.
				assert.Equal(test.expActions, mcli.Actions())
			}
		})
	}
	// test resize pvc
	{
		t.Run("test_Resize_Pvc", func(t *testing.T) {
			assert := assert.New(t)
			beforeSts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "teststatefulSet1",
					ResourceVersion: "10",
				},
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							Spec: v1.PersistentVolumeClaimSpec{
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceStorage: resource.MustParse("0.5Gi"),
									},
								},
							},
						},
					},
				},
			}
			afterSts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "teststatefulSet1",
					ResourceVersion: "10",
				},
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							Spec: v1.PersistentVolumeClaimSpec{
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceStorage: resource.MustParse("1Gi"),
									},
								},
							},
						},
					},
				},
			}
			pvcList := &v1.PersistentVolumeClaimList{
				Items: []v1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/component": "redis",
								"app.kubernetes.io/name":      "teststatefulSet1",
								"app.kubernetes.io/part-of":   "redis-failover",
							},
						},
						Spec: v1.PersistentVolumeClaimSpec{
							VolumeName: "vol-1",
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceStorage: resource.MustParse("0.5Gi"),
								},
							},
						},
					},
					// resized already
					{
						Spec: v1.PersistentVolumeClaimSpec{
							VolumeName: "vol-2",
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
			}
			// Mock.
			opts := metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/component=redis,app.kubernetes.io/name=teststatefulSet1,app.kubernetes.io/part-of=redis-failover",
			}
			expActions := []kubetesting.Action{
				newStatefulSetGetAction(testns, beforeSts.ObjectMeta.Name),
				newPVCListAction(opts),
				newPVCUpdateAction(&pvcList.Items[0]),
				newStatefulSetDeleteAction(testns, afterSts.ObjectMeta.Name),
			}
			mcli := &kubernetes.Clientset{}
			mcli.AddReactor("get", "statefulsets", func(action kubetesting.Action) (bool, runtime.Object, error) {
				return true, beforeSts, nil
			})
			mcli.AddReactor("list", "persistentvolumeclaims", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, pvcList, nil
			})
			mcli.AddReactor("update", "persistentvolumeclaims", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
				// update pvc[0]
				pvcList.Items[0] = *action.(kubetesting.UpdateActionImpl).Object.(*v1.PersistentVolumeClaim)
				return true, action.(kubetesting.UpdateActionImpl).Object, nil
			})

			mcli.AddReactor("delete", "statefulsets", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, nil
			})

			service := k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)
			err := service.CreateOrUpdateStatefulSet(testns, afterSts)
			assert.NoError(err)
			assert.Equal(pvcList.Items[0].Spec.Resources, pvcList.Items[1].Spec.Resources)
			assert.Equal(expActions, mcli.Actions())
			// should not call update

			mcli = &kubernetes.Clientset{}
			mcli.AddReactor("get", "statefulsets", func(action kubetesting.Action) (bool, runtime.Object, error) {
				return true, afterSts, nil
			})

			expActions = []kubetesting.Action{
				newStatefulSetGetAction(testns, beforeSts.ObjectMeta.Name),
				newPVCListAction(opts),
				newStatefulSetUpdateAction(testns, afterSts),
			}

			mcli.AddReactor("update", "persistentvolumeclaims", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
				panic("shouldn't call update")
			})
			service = k8s.NewStatefulSetService(mcli, log.Dummy, metrics.Dummy)
			err = service.CreateOrUpdateStatefulSet(testns, afterSts)
			assert.NoError(err)
			assert.Equal(expActions, mcli.Actions())
		})
	}
}

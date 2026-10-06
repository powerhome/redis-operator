package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/spotahome/redis-operator/operator/redisfailover/util"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
)

// StatefulSet the StatefulSet service that knows how to interact with k8s to manage them
type StatefulSet interface {
	GetStatefulSet(namespace, name string) (*appsv1.StatefulSet, error)
	GetStatefulSetPods(namespace, name string) (*corev1.PodList, error)
	CreateStatefulSet(namespace string, statefulSet *appsv1.StatefulSet) error
	UpdateStatefulSet(namespace string, statefulSet *appsv1.StatefulSet) error
	CreateOrUpdateStatefulSet(namespace string, statefulSet *appsv1.StatefulSet) error
	DeleteStatefulSet(namespace string, name string) error
	DeleteStatefulSetKeepingPods(namespace string, name string) error
	DeleteStatefulSetClaims(statefulSet *appsv1.StatefulSet) error
	PodsWaitingOnFilesystemResize(namespace string, name string) (map[string]bool, error)
	ListStatefulSets(namespace string) (*appsv1.StatefulSetList, error)
}

// StatefulSetService is the service account service implementation using API calls to kubernetes.
type StatefulSetService struct {
	kubeClient      kubernetes.Interface
	logger          log.Logger
	metricsRecorder metrics.Recorder
}

// NewStatefulSetService returns a new StatefulSet KubeService.
func NewStatefulSetService(kubeClient kubernetes.Interface, logger log.Logger, metricsRecorder metrics.Recorder) *StatefulSetService {
	logger = logger.With("service", "k8s.statefulSet")
	return &StatefulSetService{
		kubeClient:      kubeClient,
		logger:          logger,
		metricsRecorder: metricsRecorder,
	}
}

// GetStatefulSet will retrieve the requested statefulset based on namespace and name
func (s *StatefulSetService) GetStatefulSet(namespace, name string) (*appsv1.StatefulSet, error) {
	statefulSet, err := s.kubeClient.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	recordMetrics(namespace, "StatefulSet", name, "GET", err, s.metricsRecorder)
	if err != nil {
		return nil, err
	}
	return statefulSet, err
}

// GetStatefulSetPods will give a list of pods that are managed by the statefulset
func (s *StatefulSetService) GetStatefulSetPods(namespace, name string) (*corev1.PodList, error) {
	statefulSet, err := s.GetStatefulSet(namespace, name)
	if err != nil {
		return nil, err
	}
	labels := []string{}
	for k, v := range statefulSet.Spec.Selector.MatchLabels {
		labels = append(labels, fmt.Sprintf("%s=%s", k, v))
	}
	selector := strings.Join(labels, ",")
	return s.kubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector})
}

// CreateStatefulSet will create the given statefulset
func (s *StatefulSetService) CreateStatefulSet(namespace string, statefulSet *appsv1.StatefulSet) error {
	_, err := s.kubeClient.AppsV1().StatefulSets(namespace).Create(context.TODO(), statefulSet, metav1.CreateOptions{})
	recordMetrics(namespace, "StatefulSet", statefulSet.GetName(), "CREATE", err, s.metricsRecorder)
	if err != nil {
		return err
	}
	s.logger.WithField("namespace", namespace).WithField("statefulSet", statefulSet.ObjectMeta.Name).Debugf("statefulSet created")
	return err
}

// UpdateStatefulSet will update the given statefulset
func (s *StatefulSetService) UpdateStatefulSet(namespace string, statefulSet *appsv1.StatefulSet) error {
	_, err := s.kubeClient.AppsV1().StatefulSets(namespace).Update(context.TODO(), statefulSet, metav1.UpdateOptions{})
	recordMetrics(namespace, "StatefulSet", statefulSet.GetName(), "UPDATE", err, s.metricsRecorder)
	if err != nil {
		return err
	}
	s.logger.WithField("namespace", namespace).WithField("statefulSet", statefulSet.ObjectMeta.Name).Debugf("statefulSet updated")
	return err
}

// CreateOrUpdateStatefulSet will update the statefulset or create it if does not exist
func (s *StatefulSetService) CreateOrUpdateStatefulSet(namespace string, statefulSet *appsv1.StatefulSet) error {
	storedStatefulSet, err := s.GetStatefulSet(namespace, statefulSet.Name)
	if err != nil {
		// If no resource we need to create.
		if errors.IsNotFound(err) {
			return s.CreateStatefulSet(namespace, statefulSet)
		}
		return err
	}

	// Already exists, need to Update.
	// Set the correct resource version to ensure we are on the latest version. This way the only valid
	// namespace is our spec(https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#concurrency-control-and-consistency),
	// we will replace the current namespace state.
	statefulSet.ResourceVersion = storedStatefulSet.ResourceVersion

	// A statefulset's volumeClaimTemplates are immutable, which is why the
	// desired ones are overwritten with the stored ones at the end of this
	// function: without that, every ordinary update would be rejected.
	//
	// That overwrite also discards a claim being added or removed, which leaves
	// a pod template mounting a volume the set declares nowhere. Kubernetes
	// refuses the whole set, with
	// `spec.template.spec.containers[0].volumeMounts[0].name: Not found`, so
	// the claim never applies and the reconcile fails every pass.
	//
	// A claim appearing, vanishing or being renamed is therefore applied by
	// replacing the set, leaving its pods running for the replacement to adopt,
	// which is what the resize below already does for the same reason.
	if claimTemplateNames(statefulSet) != claimTemplateNames(storedStatefulSet) {
		// Growth goes first, on its own. The replacement below leaves the set
		// unable to create pods until the operator replaces the ones it adopted,
		// so raising the count in the same update asks it for pods it cannot
		// make.
		if replicaCount(statefulSet) > replicaCount(storedStatefulSet) {
			s.logger.WithField("namespace", namespace).WithField("statefulSet", statefulSet.Name).
				Infof("growing statefulset from %d to %d before changing its volume claim templates",
					replicaCount(storedStatefulSet), replicaCount(statefulSet))
			grown := storedStatefulSet.DeepCopy()
			grown.Spec.Replicas = statefulSet.Spec.Replicas
			return s.UpdateStatefulSet(namespace, grown)
		}

		s.logger.WithField("namespace", namespace).WithField("statefulSet", statefulSet.Name).
			Infof("replacing statefulset to carry its volume claim templates, [%s] where it had [%s]; its pods keep running",
				claimTemplateNames(statefulSet), claimTemplateNames(storedStatefulSet))
		return s.DeleteStatefulSetKeepingPods(namespace, statefulSet.Name)
	}

	// resize pvc
	// 1.Get the data already stored internally
	// 2.Get the desired data
	// 3.Start querying the pvc list when you find data inconsistencies
	// 3.1 Comparison using real pvc capacity and desired data
	// 3.1.1 Update if you find inconsistencies
	// 3.2 Writing successful updates to internal
	// 4. Set to old VolumeClaimTemplates to update.Prevent update error reporting
	// 5. Set to old annotations to update
	annotations := storedStatefulSet.Annotations
	if annotations == nil {
		annotations = map[string]string{
			"storageCapacity": "0",
		}
	}
	storedCapacity, _ := strconv.ParseInt(annotations["storageCapacity"], 0, 64)
	if len(statefulSet.Spec.VolumeClaimTemplates) != 0 {
		stateCapacity := statefulSet.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests.Storage().Value()
		if storedCapacity != stateCapacity {
			pvcs, err := s.claimsOf(storedStatefulSet)
			if err != nil {
				return err
			}
			updateFailed := false
			realUpdate := false
			for _, pvc := range pvcs {
				realCapacity := pvc.Spec.Resources.Requests.Storage().Value()
				if realCapacity != stateCapacity {
					realUpdate = true
					pvc.Spec.Resources.Requests = statefulSet.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests
					_, err = s.kubeClient.CoreV1().PersistentVolumeClaims(storedStatefulSet.Namespace).Update(context.Background(), &pvc, metav1.UpdateOptions{})
					if err != nil {
						updateFailed = true
						s.logger.WithField("namespace", namespace).WithField("pvc", pvc.Name).Warningf("resize pvc failed:%s", err.Error())
					}
				}
			}
			if !updateFailed && len(pvcs) != 0 {
				annotations["storageCapacity"] = fmt.Sprintf("%d", stateCapacity)
				storedStatefulSet.Annotations = annotations
				if realUpdate {
					s.logger.WithField("namespace", namespace).WithField("statefulSet", statefulSet.Name).Infof("resize statefulset pvcs from %d to %d Success", storedCapacity, stateCapacity)
					s.logger.WithField("namespace", namespace).WithField("statefulSet", statefulSet.Name).Infof("replacing statefulset to carry the resized pvcs; its pods keep running")
					// volumeClaimTemplates cannot be changed. The only way to
					// grow a claim is to replace the whole statefulset, and a
					// normal delete would stop every Redis at once. Leave the
					// pods running instead; the replacement adopts them.
					return s.DeleteStatefulSetKeepingPods(namespace, statefulSet.Name)
				} else {
					s.logger.WithField("namespace", namespace).WithField("statefulSet", statefulSet.Name).Warningf("set annotations,resize nothing")
				}
			}
		}
	}
	// set stored.volumeClaimTemplates
	statefulSet.Spec.VolumeClaimTemplates = storedStatefulSet.Spec.VolumeClaimTemplates
	statefulSet.Annotations = util.MergeAnnotations(storedStatefulSet.Annotations, statefulSet.Annotations)
	return s.UpdateStatefulSet(namespace, statefulSet)
}

// DeleteStatefulSetClaims deletes the persistent volume claims that a set's
// volume claim templates created. Kubernetes keeps a claim when its template is
// removed from the set, so a caller that stops declaring storage has to say so.
//
// Takes the stored set rather than a name because the claims are found from the
// templates it still carries, which is the only record of what they were called.
//
// A claim a pod still mounts is held by its protection finalizer and finishes
// deleting once that pod is replaced, so a running pod is undisturbed.
func (s *StatefulSetService) DeleteStatefulSetClaims(statefulSet *appsv1.StatefulSet) error {
	claims, err := s.claimsOf(statefulSet)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		s.logger.WithField("namespace", statefulSet.Namespace).WithField("pvc", claim.Name).
			Infof("deleting claim of statefulset %s, which no longer declares storage", statefulSet.Name)
		err := s.kubeClient.CoreV1().PersistentVolumeClaims(statefulSet.Namespace).
			Delete(context.Background(), claim.Name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// replicaCount is how many pods a set runs, which Kubernetes takes as one when
// the set does not say.
func replicaCount(ss *appsv1.StatefulSet) int32 {
	if ss.Spec.Replicas == nil {
		return 1
	}
	return *ss.Spec.Replicas
}

// claimTemplateNames describes a set's volume claim templates by the names its
// pods mount them under, which is what decides whether a pod can be created at
// all. A capacity that differs under the same name is a resize, handled
// separately.

func claimTemplateNames(ss *appsv1.StatefulSet) string {
	names := make([]string, 0, len(ss.Spec.VolumeClaimTemplates))
	for _, claim := range ss.Spec.VolumeClaimTemplates {
		names = append(names, claim.Name)
	}
	return strings.Join(names, ",")
}

// claimsOf returns the claims a statefulset created from its first volume claim
// template, found by the name it gives them rather than by their labels.
//
// A claim carries only the labels the RedisFailover asked for, so a Sentinel
// claim may carry none, and a selector written for the Redis ones silently
// matches nothing.
func (s *StatefulSetService) claimsOf(ss *appsv1.StatefulSet) ([]corev1.PersistentVolumeClaim, error) {
	if len(ss.Spec.VolumeClaimTemplates) == 0 {
		return nil, nil
	}

	all, err := s.kubeClient.CoreV1().PersistentVolumeClaims(ss.Namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("%s-%s-", ss.Spec.VolumeClaimTemplates[0].Name, ss.Name)
	owned := []corev1.PersistentVolumeClaim{}
	for _, pvc := range all.Items {
		if claimBelongsToSet(pvc.Name, prefix) {
			owned = append(owned, pvc)
		}
	}
	return owned, nil
}

// Kubernetes names a claim `<template>-<set>-<ordinal>`, so what remains after
// the template and set names is a pod ordinal.
//
// Requiring that, rather than the prefix alone, keeps a set away from the
// storage of one whose name begins the same way: failovers named `cache` and
// `cache-west` share the prefix `redis-data-rfr-cache-`, and the claims of the
// second are `redis-data-rfr-cache-west-0` and so on.
func claimBelongsToSet(claimName, prefix string) bool {
	ordinal, found := strings.CutPrefix(claimName, prefix)
	if !found || ordinal == "" {
		return false
	}
	for _, digit := range ordinal {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// DeleteStatefulSet will delete the statefulset and the pods it owns.
func (s *StatefulSetService) DeleteStatefulSet(namespace, name string) error {
	return s.deleteStatefulSet(namespace, name, metav1.DeletePropagationForeground)
}

// DeleteStatefulSetKeepingPods deletes the statefulset but leaves its pods
// running. The next statefulset created with the same selector adopts them.
//
// Adoption does not restart anything. These pods use the OnDelete update
// strategy, so the statefulset controller never replaces a pod itself.
func (s *StatefulSetService) DeleteStatefulSetKeepingPods(namespace, name string) error {
	return s.deleteStatefulSet(namespace, name, metav1.DeletePropagationOrphan)
}

func (s *StatefulSetService) deleteStatefulSet(namespace, name string, propagation metav1.DeletionPropagation) error {
	err := s.kubeClient.AppsV1().StatefulSets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{PropagationPolicy: &propagation})
	recordMetrics(namespace, "StatefulSet", name, "DELETE", err, s.metricsRecorder)
	return err
}

// PodsWaitingOnFilesystemResize names the pods that must restart before their
// filesystem grows to match their claim.
//
// Growing a claim takes two steps: the volume, then the filesystem on it. Some
// drivers do both while the volume stays mounted. Others grow the volume, mark
// the claim, and wait for the pod to restart.
//
// Nothing else will restart that pod. A claim's size is not part of the pod
// template, so the pod does not look out of date to anything that checks.
func (s *StatefulSetService) PodsWaitingOnFilesystemResize(namespace, name string) (map[string]bool, error) {
	waiting := map[string]bool{}

	statefulSet, err := s.GetStatefulSet(namespace, name)
	if err != nil {
		return nil, err
	}
	if statefulSet == nil || statefulSet.Spec.Selector == nil {
		return waiting, nil
	}

	// Kubernetes labels a statefulset's pods and claims with that set's
	// selector, so these labels select this set's own and nothing else in the
	// namespace.
	listOptions := metav1.ListOptions{
		LabelSelector: labels.FormatLabels(statefulSet.Spec.Selector.MatchLabels),
	}

	pvcs, err := s.kubeClient.CoreV1().PersistentVolumeClaims(namespace).List(context.TODO(), listOptions)
	recordMetrics(namespace, "PersistentVolumeClaim", metrics.NOT_APPLICABLE, "LIST", err, s.metricsRecorder)
	if err != nil {
		return nil, err
	}

	pending := map[string]bool{}
	for _, pvc := range pvcs.Items {
		if pendingFilesystemResize(pvc) {
			pending[pvc.Name] = true
		}
	}
	if len(pending) == 0 {
		return waiting, nil
	}

	pods, err := s.kubeClient.CoreV1().Pods(namespace).List(context.TODO(), listOptions)
	recordMetrics(namespace, "Pod", metrics.NOT_APPLICABLE, "LIST", err, s.metricsRecorder)
	if err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		for _, volume := range pod.Spec.Volumes {
			claim := volume.PersistentVolumeClaim
			if claim != nil && pending[claim.ClaimName] {
				waiting[pod.Name] = true
			}
		}
	}

	return waiting, nil
}

func pendingFilesystemResize(pvc corev1.PersistentVolumeClaim) bool {
	for _, condition := range pvc.Status.Conditions {
		if condition.Type == corev1.PersistentVolumeClaimFileSystemResizePending &&
			condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// ListStatefulSets will retrieve a list of statefulset in the given namespace
func (s *StatefulSetService) ListStatefulSets(namespace string) (*appsv1.StatefulSetList, error) {
	stsList, err := s.kubeClient.AppsV1().StatefulSets(namespace).List(context.TODO(), metav1.ListOptions{})
	recordMetrics(namespace, "StatefulSet", metrics.NOT_APPLICABLE, "LIST", err, s.metricsRecorder)
	return stsList, err
}

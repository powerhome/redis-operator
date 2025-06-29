package service

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/log"
	"github.com/spotahome/redis-operator/metrics"
	"github.com/spotahome/redis-operator/operator/redisfailover/util"
	"github.com/spotahome/redis-operator/service/k8s"
)

// RedisFailoverClient has the minimumm methods that a Redis failover controller needs to satisfy
// in order to talk with K8s
type RedisFailoverClient interface {
	EnsureHAProxyRedisMasterDeployment(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureHAProxyRedisMasterConfigmap(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureHAProxyRedisMasterService(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureRedisHeadlessService(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureSentinelNetworkPolicy(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureSentinelService(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureSentinelConfigMap(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureSentinelDeployment(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureRedisStatefulset(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureRedisService(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureRedisMasterService(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureRedisSlaveService(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureRedisShutdownConfigMap(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureRedisReadinessConfigMap(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureRedisConfigMap(rFailover *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureNotPresentRedisService(rFailover *redisfailoverv1.RedisFailover) error

	DestroyHaproxyMasterResources(rFailover *redisfailoverv1.RedisFailover) error
	DestroySentinelResources(rFailover *redisfailoverv1.RedisFailover) error
	UpdateStatus(rFailover *redisfailoverv1.RedisFailover) (*redisfailoverv1.RedisFailover, error)

	DestroydOrphanedRedisNetworkPolicy(rFailover *redisfailoverv1.RedisFailover) error
	DestroyOrphanedRedisSlaveHaProxy(rFailover *redisfailoverv1.RedisFailover) error
}

// RedisFailoverKubeClient implements the required methods to talk with kubernetes
type RedisFailoverKubeClient struct {
	K8SService    k8s.Services
	logger        log.Logger
	metricsClient metrics.Recorder
}

// NewRedisFailoverKubeClient creates a new RedisFailoverKubeClient
func NewRedisFailoverKubeClient(k8sService k8s.Services, logger log.Logger, metricsClient metrics.Recorder) *RedisFailoverKubeClient {
	return &RedisFailoverKubeClient{
		K8SService:    k8sService,
		logger:        logger,
		metricsClient: metricsClient,
	}
}

func generateSelectorLabels(component, name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      name,
		"app.kubernetes.io/component": component,
		"app.kubernetes.io/part-of":   appLabel,
	}
}

func generateRedisDefaultRoleLabel() map[string]string {
	return generateRedisSlaveRoleLabel()
}

func generateRedisMasterRoleLabel() map[string]string {
	return map[string]string{
		redisRoleLabelKey: redisRoleLabelMaster,
	}
}

func generateRedisSlaveRoleLabel() map[string]string {
	return map[string]string{
		redisRoleLabelKey: redisRoleLabelSlave,
	}
}

func generateComponentLabel(componentType string) map[string]string {
	return map[string]string{
		"redisfailovers.databases.spotahome.com/component": componentType,
	}
}

// EnsureSentinelNetworkPolicy makes sure the redis network policy exists
func (r *RedisFailoverKubeClient) EnsureSentinelNetworkPolicy(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	svc := generateSentinelNetworkPolicy(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateNetworkPolicy(rf.Namespace, svc)
	r.setEnsureOperationMetrics(svc.Namespace, svc.Name, "EnsureSentinelNetworkPolicy", rf.Name, err)
	return err
}

// EnsureHAProxyRedisMasterService makes sure the HAProxy service exists
func (r *RedisFailoverKubeClient) EnsureHAProxyRedisMasterService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	svc := generateHAProxyRedisMasterService(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateService(rf.Namespace, svc)
	r.setEnsureOperationMetrics(svc.Namespace, svc.Name, "EnsureHAProxyRedisMasterService", rf.Name, err)
	return err
}

// EnsureRedisHeadlessService makes sure the HAProxy service exists
func (r *RedisFailoverKubeClient) EnsureRedisHeadlessService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	svc := generateRedisHeadlessService(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateService(rf.Namespace, svc)
	r.setEnsureOperationMetrics(svc.Namespace, svc.Name, "EnsureRedisHeadlessSerice", rf.Name, err)
	return err
}

// EnsureHAProxyRedisMasterConfigmap makes sure the HAProxy configmap exists
func (r *RedisFailoverKubeClient) EnsureHAProxyRedisMasterConfigmap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	svc := generateHAProxyRedisMasterConfigmap(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateConfigMap(rf.Namespace, svc)
	r.setEnsureOperationMetrics(svc.Namespace, svc.Name, "EnsureHAProxyRedisMasterConfigmap", rf.Name, err)
	return err
}

// EnsureHAProxyRedisMasterDeployment makes sure the sentinel deployment exists in the desired state
func (r *RedisFailoverKubeClient) EnsureHAProxyRedisMasterDeployment(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	// Get the name of the ConfigMap we expect to have already been created
	configMapName := GetHaproxyMasterName(rf)

	// Fetch the existing ConfigMap
	cm, err := r.K8SService.GetConfigMap(rf.Namespace, configMapName)
	if err != nil {
		return fmt.Errorf("EnsureHAProxyRedisMasterDeployment failed to fetch existing ConfigMap %s: %w", configMapName, err)
	}

	// Extract the checksum from its annotations
	digest := cm.Annotations[haproxyConfigChecksumAnnotationKey]
	if digest == "" {
		return fmt.Errorf("missing %s annotation on ConfigMap %s", haproxyConfigChecksumAnnotationKey, configMapName)
	}

	d := generateHAProxyRedisMasterDeployment(rf, labels, ownerRefs)

	if d.Spec.Template.Annotations == nil {
		d.Spec.Template.Annotations = make(map[string]string)
	}
	d.Spec.Template.Annotations[haproxyConfigChecksumAnnotationKey] = digest

	err = r.K8SService.CreateOrUpdateDeployment(rf.Namespace, d)

	r.setEnsureOperationMetrics(d.Namespace, d.Name, "EnsureHAProxyRedisMasterDeployment", rf.Name, err)
	return err
}

// EnsureSentinelService makes sure the sentinel service exists
func (r *RedisFailoverKubeClient) EnsureSentinelService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	svc := generateSentinelService(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateService(rf.Namespace, svc)
	r.setEnsureOperationMetrics(svc.Namespace, svc.Name, "Service", rf.Name, err)
	return err
}

// EnsureSentinelConfigMap makes sure the sentinel configmap exists
func (r *RedisFailoverKubeClient) EnsureSentinelConfigMap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	cm := generateSentinelConfigMap(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateConfigMap(rf.Namespace, cm)
	r.setEnsureOperationMetrics(cm.Namespace, cm.Name, "ConfigMap", rf.Name, err)
	return err
}

// EnsureSentinelDeployment makes sure the sentinel deployment exists in the desired state
func (r *RedisFailoverKubeClient) EnsureSentinelDeployment(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	if !rf.Spec.Sentinel.DisablePodDisruptionBudget {
		if err := r.ensurePodDisruptionBudget(rf, sentinelName, sentinelRoleName, labels, ownerRefs); err != nil {
			return err
		}
	}
	d := generateSentinelDeployment(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateDeployment(rf.Namespace, d)

	r.setEnsureOperationMetrics(d.Namespace, d.Name, "Deployment", rf.Name, err)
	return err
}

// DestroySentinelResources eliminates sentinel pods and its dependend resources, unnecessary for a bootstrap mode
func (r *RedisFailoverKubeClient) DestroySentinelResources(rf *redisfailoverv1.RedisFailover) error {

	name := GetSentinelName(rf)

	if _, err := r.K8SService.GetDeployment(rf.Namespace, name); err != nil {
		// If no resource, do nothing
		if errors.IsNotFound(err) {
			return nil
		}
	}

	if !rf.Spec.Sentinel.DisablePodDisruptionBudget {
		if err := r.K8SService.DeletePodDisruptionBudget(rf.Namespace, name); err != nil {
			return err
		}
	}

	if err := r.K8SService.DeleteService(rf.Namespace, name); err != nil {
		return err
	}

	if err := r.K8SService.DeleteConfigMap(rf.Namespace, name); err != nil {
		return err
	}

	err := r.K8SService.DeleteDeployment(rf.Namespace, name)
	return err
}

// DestroyHaproxyMasterResources eliminates haproxy pods and its dependend resources, unnecessary for a bootstrap mode
func (r *RedisFailoverKubeClient) DestroyHaproxyMasterResources(rf *redisfailoverv1.RedisFailover) error {
	name := GetHaproxyMasterName(rf)

	if _, err := r.K8SService.GetDeployment(rf.Namespace, name); err != nil {
		// If no resource, do nothing
		if errors.IsNotFound(err) {
			return nil
		}
	}

	if err := r.K8SService.DeleteService(rf.Namespace, name); err != nil {
		return err
	}

	if err := r.K8SService.DeleteConfigMap(rf.Namespace, name); err != nil {
		return err
	}

	err := r.K8SService.DeleteDeployment(rf.Namespace, name)
	return err
}

func (r *RedisFailoverKubeClient) DestroydOrphanedRedisNetworkPolicy(rf *redisfailoverv1.RedisFailover) error {

	name := GetRedisNetworkPolicyName(rf)

	if _, err := r.K8SService.GetNetworkPolicy(rf.Namespace, name); err != nil {
		if errors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}

	err := r.K8SService.DeleteNetworkPolicy(rf.Namespace, name)
	return err
}

func (r *RedisFailoverKubeClient) DestroyOrphanedRedisSlaveHaProxy(rf *redisfailoverv1.RedisFailover) error {

	// Helper function to handle the deletion of resources
	deleteResource := func(namespace, name string, getter func(namespace, name string) (interface{}, error), deleter func(namespace, name string) error) error {
		_, err := getter(namespace, name)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return deleter(namespace, name)
	}

	resourceTypes := map[string]struct {
		getter  func(namespace, name string) (interface{}, error)
		deleter func(namespace, name string) error
	}{
		"service": {
			getter:  func(namespace, name string) (interface{}, error) { return r.K8SService.GetService(namespace, name) },
			deleter: r.K8SService.DeleteService,
		},
		"configmap": {
			getter:  func(namespace, name string) (interface{}, error) { return r.K8SService.GetConfigMap(namespace, name) },
			deleter: r.K8SService.DeleteConfigMap,
		},
		"deployment": {
			getter:  func(namespace, name string) (interface{}, error) { return r.K8SService.GetDeployment(namespace, name) },
			deleter: r.K8SService.DeleteDeployment,
		},
	}

	name := GetHaproxySlaveName(rf)

	for _, resType := range []string{"service", "configmap", "deployment"} {
		resource := resourceTypes[resType]
		if err := deleteResource(rf.Namespace, name, resource.getter, resource.deleter); err != nil {
			return err
		}
	}

	return nil
}

// EnsureRedisStatefulset makes sure the redis statefulset exists in the desired state
func (r *RedisFailoverKubeClient) EnsureRedisStatefulset(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	if !rf.Spec.Redis.DisablePodDisruptionBudget {
		if err := r.ensurePodDisruptionBudget(rf, redisName, redisRoleName, labels, ownerRefs); err != nil {
			return err
		}
	}
	ss := generateRedisStatefulSet(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateStatefulSet(rf.Namespace, ss)

	r.setEnsureOperationMetrics(ss.Namespace, ss.Name, "StatefulSet", rf.Name, err)
	return err
}

// EnsureRedisConfigMap makes sure the Redis ConfigMap exists
func (r *RedisFailoverKubeClient) EnsureRedisConfigMap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {

	password, err := k8s.GetRedisPassword(r.K8SService, rf)
	if err != nil {
		return err
	}

	cm := generateRedisConfigMap(rf, labels, ownerRefs, password)
	err = r.K8SService.CreateOrUpdateConfigMap(rf.Namespace, cm)

	r.setEnsureOperationMetrics(cm.Namespace, cm.Name, "ConfigMap", rf.Name, err)
	return err
}

// EnsureRedisShutdownConfigMap makes sure the redis configmap with shutdown script exists
func (r *RedisFailoverKubeClient) EnsureRedisShutdownConfigMap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	if rf.Spec.Redis.ShutdownConfigMap != "" {
		if _, err := r.K8SService.GetConfigMap(rf.Namespace, rf.Spec.Redis.ShutdownConfigMap); err != nil {
			return err
		}
	} else {
		cm := generateRedisShutdownConfigMap(rf, labels, ownerRefs)
		err := r.K8SService.CreateOrUpdateConfigMap(rf.Namespace, cm)
		r.setEnsureOperationMetrics(cm.Namespace, cm.Name, "ConfigMap", rf.Name, err)
		return err
	}
	return nil
}

// EnsureRedisReadinessConfigMap makes sure the redis configmap with shutdown script exists
func (r *RedisFailoverKubeClient) EnsureRedisReadinessConfigMap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	cm := generateRedisReadinessConfigMap(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateConfigMap(rf.Namespace, cm)
	r.setEnsureOperationMetrics(cm.Namespace, cm.Name, "ConfigMap", rf.Name, err)
	return err
}

// EnsureRedisService makes sure the redis statefulset exists
func (r *RedisFailoverKubeClient) EnsureRedisService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	svc := generateRedisService(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateService(rf.Namespace, svc)

	r.setEnsureOperationMetrics(svc.Namespace, svc.Name, "Service", rf.Name, err)
	return err
}

// EnsureNotPresentRedisService makes sure the redis service is not present
func (r *RedisFailoverKubeClient) EnsureNotPresentRedisService(rf *redisfailoverv1.RedisFailover) error {
	name := GetRedisName(rf)
	namespace := rf.Namespace
	// If the service exists (no get error), delete it
	if _, err := r.K8SService.GetService(namespace, name); err == nil {
		return r.K8SService.DeleteService(namespace, name)
	}
	return nil
}

// EnsureRedisMasterService makes sure the redis master service exists
func (r *RedisFailoverKubeClient) EnsureRedisMasterService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	svc := generateRedisMasterService(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateService(rf.Namespace, svc)

	r.setEnsureOperationMetrics(svc.Namespace, svc.Name, "Service", rf.Name, err)
	return err
}

// EnsureRedisSlaveService makes sure the redis slave service exists
func (r *RedisFailoverKubeClient) EnsureRedisSlaveService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	svc := generateRedisSlaveService(rf, labels, ownerRefs)
	err := r.K8SService.CreateOrUpdateService(rf.Namespace, svc)

	r.setEnsureOperationMetrics(svc.Namespace, svc.Name, "Service", rf.Name, err)
	return err
}

// EnsureRedisStatefulset makes sure the pdb exists in the desired state
func (r *RedisFailoverKubeClient) ensurePodDisruptionBudget(rf *redisfailoverv1.RedisFailover, name string, component string, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	name = generateName(name, rf.Name)
	namespace := rf.Namespace

	minAvailable := intstr.FromInt(2)
	if rf.Spec.Redis.Replicas <= 2 {
		minAvailable = intstr.FromInt(1)
	}

	labels = util.MergeLabels(labels, generateSelectorLabels(component, rf.Name))

	pdb := generatePodDisruptionBudget(name, namespace, labels, ownerRefs, minAvailable)
	err := r.K8SService.CreateOrUpdatePodDisruptionBudget(namespace, pdb)
	r.setEnsureOperationMetrics(pdb.Namespace, pdb.Name, "PodDisruptionBudget" /* pdb.TypeMeta.Kind isnt working;  pdb.Kind isnt working either */, rf.Name, err)
	return err
}

func (r *RedisFailoverKubeClient) setEnsureOperationMetrics(objectNamespace string, objectName string, objectKind string, ownerName string, err error) {
	if nil != err {
		r.metricsClient.RecordEnsureOperation(objectNamespace, objectName, objectKind, ownerName, metrics.FAIL)
	}
	r.metricsClient.RecordEnsureOperation(objectNamespace, objectName, objectKind, ownerName, metrics.SUCCESS)
}

func (r *RedisFailoverKubeClient) UpdateStatus(rf *redisfailoverv1.RedisFailover) (*redisfailoverv1.RedisFailover, error) {
	rf, err := r.K8SService.WriteStatus(rf)
	return rf, err
}

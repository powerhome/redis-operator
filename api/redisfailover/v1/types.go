package v1

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RedisFailover represents a Redis failover
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=".metadata.name"
// +kubebuilder:printcolumn:name="REDIS",type="integer",JSONPath=".spec.redis.replicas"
// +kubebuilder:printcolumn:name="SENTINELS",type="integer",JSONPath=".spec.sentinel.replicas"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.conditions[-1:].message"
// +kubebuilder:resource:singular=redisfailover,path=redisfailovers,shortName=rf,scope=Namespaced
// +kubebuilder:metadata:annotations=krane.shopify.io/instance-rollout-conditions=true
// +kubebuilder:subresource:status
type RedisFailover struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RedisFailoverSpec   `json:"spec"`
	Status            RedisFailoverStatus `json:"status,omitempty"`
}

type AppState string

const (
	AppStatePending AppState = "Pending"
	AppStateReady   AppState = "Ready"
)

type ConditionStatus string

const (
	ConditionTrue ConditionStatus = "True"
)

type ClusterCondition struct {
	Status  ConditionStatus `json:"status,omitempty"`
	Type    AppState        `json:"type,omitempty"`
	Message string          `json:"message,omitempty"`
}

// RedisFailoverStatus defines the observed state
type RedisFailoverStatus struct {
	Conditions         []ClusterCondition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

// RedisFailoverSpec represents a Redis failover spec
type RedisFailoverSpec struct {
	Redis               RedisSettings                 `json:"redis,omitempty"`
	Sentinel            SentinelSettings              `json:"sentinel,omitempty"`
	Haproxy             *HaproxySettings              `json:"haproxy,omitempty"`
	Auth                AuthSettings                  `json:"auth,omitempty"`
	LabelWhitelist      []string                      `json:"labelWhitelist,omitempty"`
	BootstrapNode       *BootstrapSettings            `json:"bootstrapNode,omitempty"`
	NetworkPolicyNsList []NetworkPolicyNamespaceEntry `json:"networkPolicyNsList,omitempty"`
}

// NetworkPolicyNamespaceEntry represents the the key value of a label
type NetworkPolicyNamespaceEntry struct {
	MatchLabelKey   string `json:"matchLabelKey,omitempty"`
	MatchLabelValue string `json:"matchLabelValue,omitempty"`
}

// HaproxySettings configures the haproxy workload
type HaproxySettings struct {
	RedisHost    string                      `json:"redisHost,omitempty"`
	Image        string                      `json:"image,omitempty"`
	Replicas     int32                       `json:"replicas,omitempty"`
	Resources    corev1.ResourceRequirements `json:"resources,omitempty"`
	CustomConfig string                      `json:"customConfig,omitempty"`
	Affinity     *corev1.Affinity            `json:"affinity,omitempty"`
	Service      *ServiceSettings            `json:"service,omitempty"`
	Exporter     *HaproxyExporterSettings    `json:"exporter,omitempty"`
}

// HaproxyExporterSettings configures haproxy's prometheus exporter frontend
type HaproxyExporterSettings struct {
	Enabled bool `json:"enabled,omitempty"`
	Port    Port `json:"port,omitempty"`
}

type ServiceSettings struct {
	ClusterIP string `json:"clusterIP,omitempty" protobuf:"bytes,3,opt,name=clusterIP"`
}

// RedisCommandRename defines the specification of a "rename-command" configuration option
type RedisCommandRename struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type Port int32

// RedisSettings defines the specification of the redis cluster
type RedisSettings struct {
	Image                         string                            `json:"image,omitempty"`
	ImagePullPolicy               corev1.PullPolicy                 `json:"imagePullPolicy,omitempty"`
	Replicas                      int32                             `json:"replicas,omitempty"`
	Port                          Port                              `json:"port,omitempty"`
	Resources                     corev1.ResourceRequirements       `json:"resources,omitempty"`
	CustomConfig                  []string                          `json:"customConfig,omitempty"`
	CustomCommandRenames          []RedisCommandRename              `json:"customCommandRenames,omitempty"`
	Command                       []string                          `json:"command,omitempty"`
	ShutdownConfigMap             string                            `json:"shutdownConfigMap,omitempty"`
	StartupConfigMap              string                            `json:"startupConfigMap,omitempty"`
	Storage                       RedisStorage                      `json:"storage,omitempty"`
	InitContainers                []corev1.Container                `json:"initContainers,omitempty"`
	Exporter                      Exporter                          `json:"exporter,omitempty"`
	ExtraContainers               []corev1.Container                `json:"extraContainers,omitempty"`
	Affinity                      *corev1.Affinity                  `json:"affinity,omitempty"`
	SecurityContext               *corev1.PodSecurityContext        `json:"securityContext,omitempty"`
	ContainerSecurityContext      *corev1.SecurityContext           `json:"containerSecurityContext,omitempty"`
	ImagePullSecrets              []corev1.LocalObjectReference     `json:"imagePullSecrets,omitempty"`
	Tolerations                   []corev1.Toleration               `json:"tolerations,omitempty"`
	TopologySpreadConstraints     []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	NodeSelector                  map[string]string                 `json:"nodeSelector,omitempty"`
	PodAnnotations                map[string]string                 `json:"podAnnotations,omitempty"`
	ServiceAnnotations            map[string]string                 `json:"serviceAnnotations,omitempty"`
	HostNetwork                   bool                              `json:"hostNetwork,omitempty"`
	DNSPolicy                     corev1.DNSPolicy                  `json:"dnsPolicy,omitempty"`
	PriorityClassName             string                            `json:"priorityClassName,omitempty"`
	ServiceAccountName            string                            `json:"serviceAccountName,omitempty"`
	TerminationGracePeriodSeconds int64                             `json:"terminationGracePeriod,omitempty"`
	ExtraVolumes                  []corev1.Volume                   `json:"extraVolumes,omitempty"`
	ExtraVolumeMounts             []corev1.VolumeMount              `json:"extraVolumeMounts,omitempty"`
	CustomLivenessProbe           *corev1.Probe                     `json:"customLivenessProbe,omitempty"`
	CustomReadinessProbe          *corev1.Probe                     `json:"customReadinessProbe,omitempty"`
	CustomStartupProbe            *corev1.Probe                     `json:"customStartupProbe,omitempty"`
	DisablePodDisruptionBudget    bool                              `json:"disablePodDisruptionBudget,omitempty"`
}

// SentinelSettings defines the specification of the sentinel cluster
type SentinelSettings struct {
	Image                      string                            `json:"image,omitempty"`
	ImagePullPolicy            corev1.PullPolicy                 `json:"imagePullPolicy,omitempty"`
	Replicas                   int32                             `json:"replicas,omitempty"`
	Port                       Port                              `json:"port,omitempty"`
	Resources                  corev1.ResourceRequirements       `json:"resources,omitempty"`
	CustomConfig               []string                          `json:"customConfig,omitempty"`
	Command                    []string                          `json:"command,omitempty"`
	StartupConfigMap           string                            `json:"startupConfigMap,omitempty"`
	Affinity                   *corev1.Affinity                  `json:"affinity,omitempty"`
	SecurityContext            *corev1.PodSecurityContext        `json:"securityContext,omitempty"`
	ContainerSecurityContext   *corev1.SecurityContext           `json:"containerSecurityContext,omitempty"`
	ImagePullSecrets           []corev1.LocalObjectReference     `json:"imagePullSecrets,omitempty"`
	Tolerations                []corev1.Toleration               `json:"tolerations,omitempty"`
	TopologySpreadConstraints  []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	NodeSelector               map[string]string                 `json:"nodeSelector,omitempty"`
	PodAnnotations             map[string]string                 `json:"podAnnotations,omitempty"`
	ServiceAnnotations         map[string]string                 `json:"serviceAnnotations,omitempty"`
	InitContainers             []corev1.Container                `json:"initContainers,omitempty"`
	Exporter                   Exporter                          `json:"exporter,omitempty"`
	ExtraContainers            []corev1.Container                `json:"extraContainers,omitempty"`
	ConfigCopy                 SentinelConfigCopy                `json:"configCopy,omitempty"`
	HostNetwork                bool                              `json:"hostNetwork,omitempty"`
	DNSPolicy                  corev1.DNSPolicy                  `json:"dnsPolicy,omitempty"`
	PriorityClassName          string                            `json:"priorityClassName,omitempty"`
	ServiceAccountName         string                            `json:"serviceAccountName,omitempty"`
	ExtraVolumes               []corev1.Volume                   `json:"extraVolumes,omitempty"`
	ExtraVolumeMounts          []corev1.VolumeMount              `json:"extraVolumeMounts,omitempty"`
	CustomLivenessProbe        *corev1.Probe                     `json:"customLivenessProbe,omitempty"`
	CustomReadinessProbe       *corev1.Probe                     `json:"customReadinessProbe,omitempty"`
	CustomStartupProbe         *corev1.Probe                     `json:"customStartupProbe,omitempty"`
	DisablePodDisruptionBudget bool                              `json:"disablePodDisruptionBudget,omitempty"`
}

// AuthSettings contains settings about auth
type AuthSettings struct {
	SecretPath string `json:"secretPath,omitempty"`
}

// BootstrapSettings contains settings about a potential bootstrap node
type BootstrapSettings struct {
	Host           string `json:"host,omitempty"`
	Port           string `json:"port,omitempty"`
	AllowSentinels bool   `json:"allowSentinels,omitempty"`
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`
}

// Exporter defines the specification for the redis/sentinel exporter
type Exporter struct {
	Enabled                  bool                         `json:"enabled,omitempty"`
	Image                    string                       `json:"image,omitempty"`
	ImagePullPolicy          corev1.PullPolicy            `json:"imagePullPolicy,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext      `json:"containerSecurityContext,omitempty"`
	Args                     []string                     `json:"args,omitempty"`
	Env                      []corev1.EnvVar              `json:"env,omitempty"`
	Resources                *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// SentinelConfigCopy defines the specification for the sentinel exporter
type SentinelConfigCopy struct {
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`
}

// RedisStorage defines the structure used to store the Redis Data
type RedisStorage struct {
	KeepAfterDeletion     bool                           `json:"keepAfterDeletion,omitempty"`
	EmptyDir              *corev1.EmptyDirVolumeSource   `json:"emptyDir,omitempty"`
	PersistentVolumeClaim *EmbeddedPersistentVolumeClaim `json:"persistentVolumeClaim,omitempty"`
}

// EmbeddedPersistentVolumeClaim is an embedded version of k8s.io/api/core/v1.PersistentVolumeClaim.
// It contains TypeMeta and a reduced ObjectMeta.
type EmbeddedPersistentVolumeClaim struct {
	metav1.TypeMeta `json:",inline"`

	// EmbeddedMetadata contains metadata relevant to an EmbeddedResource.
	EmbeddedObjectMetadata `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the desired characteristics of a volume requested by a pod author.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	// +optional
	Spec corev1.PersistentVolumeClaimSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Status represents the current information/status of a persistent volume claim.
	// Read-only.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	// +optional
	Status corev1.PersistentVolumeClaimStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// EmbeddedObjectMetadata contains a subset of the fields included in k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta
// Only fields which are relevant to embedded resources are included.
type EmbeddedObjectMetadata struct {
	// Name must be unique within a namespace. Is required when creating resources, although
	// some resources may allow a client to request the generation of an appropriate name
	// automatically. Name is primarily intended for creation idempotence and configuration
	// definition.
	// Cannot be updated.
	// More info: http://kubernetes.io/docs/user-guide/identifiers#names
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`

	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty" protobuf:"bytes,11,rep,name=labels"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,12,rep,name=annotations"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RedisFailoverList represents a Redis failover list
type RedisFailoverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RedisFailover `json:"items"`
}

const maxStatusesQuantity = 10

func (s *RedisFailoverStatus) AddCondition(c ClusterCondition) {
	if len(s.Conditions) == 0 {
		s.Conditions = append(s.Conditions, c)
		return
	}

	if s.Conditions[len(s.Conditions)-1].Type != c.Type {
		s.Conditions = append(s.Conditions, c)
	}

	if len(s.Conditions) > maxStatusesQuantity {
		s.Conditions = s.Conditions[len(s.Conditions)-maxStatusesQuantity:]
	}
}

func (r *RedisFailover) GenerateName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, r.Name)
}

func (p Port) ToString() string {
	return strconv.Itoa(int(p))
}

func (p Port) ToInt32() int32 {
	return int32(p)
}

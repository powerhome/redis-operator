package service

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	np "k8s.io/api/networking/v1"

	redisfailoverv1 "github.com/spotahome/redis-operator/api/redisfailover/v1"
	"github.com/spotahome/redis-operator/operator/redisfailover/util"
)

const (
	redisConfigurationVolumeName = "redis-config"
	// Template used to build the Redis configuration
	redisConfigTemplate = `slaveof 127.0.0.1 {{.Spec.Redis.Port}}
port {{.Spec.Redis.Port}}
tcp-keepalive 60
save 900 1
save 300 10
user pinger -@all +ping on >pingpass
{{- range .Spec.Redis.CustomCommandRenames}}
rename-command "{{.From}}" "{{.To}}"
{{- end}}
`

	sentinelConfigTemplate = `sentinel monitor mymaster 127.0.0.1 {{.Spec.Redis.Port}} 2
sentinel down-after-milliseconds mymaster 1000
sentinel failover-timeout mymaster 3000
sentinel announce-port {{.Spec.Sentinel.Port}}
port {{.Spec.Sentinel.Port}}
sentinel parallel-syncs mymaster 2`

	redisShutdownConfigurationVolumeName   = "redis-shutdown-config"
	redisStartupConfigurationVolumeName    = "redis-startup-config"
	redisReadinessVolumeName               = "redis-readiness-config"
	redisStorageVolumeName                 = "redis-data"
	sentinelStartupConfigurationVolumeName = "sentinel-startup-config"

	graceTime = 30
)

func generateHAProxyRedisMasterDeployment(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *appsv1.Deployment {

	name := GetHaproxyMasterName(rf)

	namespace := rf.Namespace

	labels = util.MergeLabels(labels, map[string]string{
		"app.kubernetes.io/component": "redis",
	})

	selectorLabels := util.MergeLabels(labels, generateComponentLabel("haproxy"))

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/usr/local/etc/haproxy/haproxy.cfg",
			SubPath:   "haproxy.cfg",
			ReadOnly:  true,
		},
	}

	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
				},
			},
		},
	}

	sd := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &rf.Spec.Haproxy.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: selectorLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "haproxy",
							Image: rf.Spec.Haproxy.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: rf.Spec.Redis.Port.ToInt32(),
								},
							},
							VolumeMounts: volumeMounts,
							Resources:    rf.Spec.Haproxy.Resources,
						},
					},
					Volumes:       volumes,
					RestartPolicy: "Always",
				},
			},
		},
	}

	if rf.Spec.Haproxy.Affinity != nil {
		sd.Spec.Template.Spec.Affinity = rf.Spec.Haproxy.Affinity
	}

	return sd
}

func generateHAProxyRedisMasterConfigmap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.ConfigMap {
	name := GetHaproxyMasterName(rf)
	redisName := rf.GenerateName("redis")

	namespace := rf.Namespace

	labels = util.MergeLabels(labels, map[string]string{
		"app.kubernetes.io/component": "redis",
	})

	port := rf.Spec.Redis.Port
	haproxyCfg := fmt.Sprintf(`global
	daemon
	maxconn 5000

	defaults
	mode tcp
	timeout connect 5000ms
	timeout client 50000ms
	timeout server 50000ms
	timeout check 5000ms

	frontend http
	bind :8080
	default_backend stats

	backend stats
	mode http
	stats enable
	stats uri /
	stats refresh 1s
	stats show-legends
	stats admin if TRUE

	resolvers k8s
	parse-resolv-conf
	hold other 10s
	hold refused 10s
	hold nx 10
	hold timeout 10s
	hold valid 10s
	hold obsolete 10s

	frontend redis-master
	bind *:%d
	default_backend redis-master

	backend redis-master
	mode tcp
	balance first
	option tcp-check
	tcp-check send info\ replication\r\n
	tcp-check expect string role:master
	server-template redis %d _redis._tcp.%s.%s.svc.cluster.local:%d check inter 1s resolvers k8s init-addr none
`, port, rf.Spec.Redis.Replicas, redisName, namespace, port)

	if rf.Spec.Haproxy.CustomConfig != "" {
		haproxyCfg = rf.Spec.Haproxy.CustomConfig
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Data: map[string]string{
			"haproxy.cfg": haproxyCfg,
		},
	}
}

func generateRedisHeadlessService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.Service {

	name := rf.GenerateName("redis")
	namespace := rf.Namespace

	redisTargetPort := intstr.FromString("redis")

	selectorLabels := util.MergeLabels(labels, map[string]string{
		"app.kubernetes.io/component": "redis",
	})

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: corev1.ServiceSpec{
			Selector:  selectorLabels,
			Type:      "ClusterIP",
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       rf.Spec.Redis.Port.ToInt32(),
					TargetPort: redisTargetPort,
					Protocol:   "TCP",
				},
			},
		},
	}
}

func generateHAProxyRedisMasterService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.Service {
	name := rf.Spec.Haproxy.RedisHost
	if name == "" {
		name = GetHaproxyMasterName(rf)
	}
	namespace := rf.Namespace
	redisTargetPort := intstr.FromInt(int(rf.Spec.Redis.Port))
	selectorLabels := map[string]string{
		"app.kubernetes.io/component": "redis",
	}
	selectorLabels = util.MergeLabels(selectorLabels, generateComponentLabel("haproxy"))
	selectorLabels = util.MergeLabels(labels, selectorLabels)

	spec := corev1.ServiceSpec{
		Selector: selectorLabels,
		Type:     "ClusterIP",
		Ports: []corev1.ServicePort{
			{
				Name:       "redis-master",
				Port:       rf.Spec.Redis.Port.ToInt32(),
				TargetPort: redisTargetPort,
				Protocol:   "TCP",
			},
		},
	}

	serviceSettings := rf.Spec.Haproxy.Service
	if serviceSettings != nil {
		if serviceSettings.ClusterIP != "" {
			spec.ClusterIP = serviceSettings.ClusterIP
		}
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: spec,
	}
}

func generateHAProxyRedisSlaveDeployment(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *appsv1.Deployment {

	name := GetHaproxySlaveName(rf)

	namespace := rf.Namespace

	labels = util.MergeLabels(labels, map[string]string{
		"app.kubernetes.io/component": "redis",
		redisHARoleLabelKey:           redisRoleLabelSlave,
	})

	selectorLabels := util.MergeLabels(
		labels,
		generateComponentLabel("haproxy"),
	)

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/usr/local/etc/haproxy/haproxy.cfg",
			SubPath:   "haproxy.cfg",
			ReadOnly:  true,
		},
	}

	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
				},
			},
		},
	}

	sd := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &rf.Spec.Haproxy.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: selectorLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "haproxy",
							Image: rf.Spec.Haproxy.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: rf.Spec.Redis.Port.ToInt32(),
								},
							},
							VolumeMounts: volumeMounts,
							Resources:    rf.Spec.Haproxy.Resources,
						},
					},
					Volumes:       volumes,
					RestartPolicy: "Always",
				},
			},
		},
	}

	if rf.Spec.Haproxy.Affinity != nil {
		sd.Spec.Template.Spec.Affinity = rf.Spec.Haproxy.Affinity
	}

	return sd
}

func generateHAProxyRedisSlaveConfigmap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.ConfigMap {
	name := GetHaproxySlaveName(rf)
	redisName := rf.GenerateName("redis")

	namespace := rf.Namespace

	labels = util.MergeLabels(labels, map[string]string{
		"app.kubernetes.io/component": "redis",
		redisHARoleLabelKey:           redisRoleLabelSlave,
	})

	port := rf.Spec.Redis.Port
	haproxyCfg := fmt.Sprintf(`global
	daemon
	maxconn 5000

	defaults
	mode tcp
	timeout connect 5000ms
	timeout client 50000ms
	timeout server 50000ms
	timeout check 5000ms

	frontend http
	bind :8080
	default_backend stats

	backend stats
	mode http
	stats enable
	stats uri /
	stats refresh 1s
	stats show-legends
	stats admin if TRUE

	resolvers k8s
	parse-resolv-conf
	hold other 10s
	hold refused 10s
	hold nx 10
	hold timeout 10s
	hold valid 10s
	hold obsolete 10s

	frontend redis-slave
	bind *:%d
	default_backend redis-slave

	backend redis-slave
	mode tcp
	balance first
	option tcp-check
	tcp-check send info\ replication\r\n
	tcp-check expect string role:slave
	server-template redis %d _redis._tcp.%s.%s.svc.cluster.local:%d check inter 1s resolvers k8s init-addr none
`, port, rf.Spec.Redis.Replicas, redisName, namespace, port)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Data: map[string]string{
			"haproxy.cfg": haproxyCfg,
		},
	}
}

func generateHAProxyRedisSlaveService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.Service {
	name := GetHaproxySlaveName(rf)

	namespace := rf.Namespace
	redisTargetPort := intstr.FromInt(int(rf.Spec.Redis.Port))
	selectorLabels := map[string]string{
		"app.kubernetes.io/component": "redis",
		redisHARoleLabelKey:           redisRoleLabelSlave,
	}
	selectorLabels = util.MergeLabels(
		selectorLabels,
		generateComponentLabel("haproxy"),
		labels)

	spec := corev1.ServiceSpec{
		Selector: selectorLabels,
		Type:     "ClusterIP",
		Ports: []corev1.ServicePort{
			{
				Name:       "redis-slave",
				Port:       rf.Spec.Redis.Port.ToInt32(),
				TargetPort: redisTargetPort,
				Protocol:   "TCP",
			},
		},
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: spec,
	}
}

func generateRedisNetworkPolicy(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *np.NetworkPolicy {
	name := GetRedisNetworkPolicyName(rf)
	namespace := rf.Namespace

	networkPolicyNsList := rf.Spec.NetworkPolicyNsList

	selectorLabels := generateSelectorLabels(redisRoleName, rf.Name)
	labels = util.MergeLabels(labels, selectorLabels)

	metricsTargetPort := intstr.FromInt(9121)
	redisTargetPort := intstr.FromInt(int(rf.Spec.Redis.Port))

	peers := []np.NetworkPolicyPeer{}

	for _, inputPeer := range networkPolicyNsList {

		labelKey := inputPeer.MatchLabelKey
		labelValue := inputPeer.MatchLabelValue

		peers = append(peers, np.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{labelKey: labelValue},
			},
		})
	}

	ports := make([]np.NetworkPolicyPort, 0)
	ports = append(ports, np.NetworkPolicyPort{
		Port: &redisTargetPort,
	}, np.NetworkPolicyPort{
		Port: &metricsTargetPort,
	})

	return &np.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: np.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: util.MergeLabels(
					map[string]string{"redisfailovers.databases.spotahome.com/name": rf.Name},
					generateComponentLabel("redis"),
				),
			},
			Ingress: []np.NetworkPolicyIngressRule{
				np.NetworkPolicyIngressRule{
					From:  peers,
					Ports: ports,
				},
			},
		},
	}
}

func generateSentinelNetworkPolicy(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *np.NetworkPolicy {
	name := GetSentinelNetworkPolicyName(rf)
	namespace := rf.Namespace

	networkPolicyNsList := rf.Spec.NetworkPolicyNsList

	selectorLabels := generateSelectorLabels(sentinelRoleName, rf.Name)
	labels = util.MergeLabels(labels, selectorLabels)

	sentinelTargetPort := intstr.FromInt(int(rf.Spec.Sentinel.Port))

	peers := []np.NetworkPolicyPeer{}

	for _, inputPeer := range networkPolicyNsList {

		labelKey := inputPeer.MatchLabelKey
		labelValue := inputPeer.MatchLabelValue

		peers = append(peers, np.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{labelKey: labelValue},
			},
		})
	}

	ports := make([]np.NetworkPolicyPort, 0)
	ports = append(ports, np.NetworkPolicyPort{
		Port: &sentinelTargetPort,
	})

	return &np.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: np.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: util.MergeLabels(
					map[string]string{"redisfailovers.databases.spotahome.com/name": rf.Name},
					generateComponentLabel("sentinel"),
				),
			},
			Ingress: []np.NetworkPolicyIngressRule{
				np.NetworkPolicyIngressRule{
					From:  peers,
					Ports: ports,
				},
			},
		},
	}
}

func generateSentinelService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.Service {
	name := GetSentinelName(rf)
	namespace := rf.Namespace
	sentinelTargetPort := intstr.FromInt(int(rf.Spec.Sentinel.Port))

	selectorLabels := generateSelectorLabels(sentinelRoleName, rf.Name)
	labels = util.MergeLabels(labels, selectorLabels)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
			Annotations:     rf.Spec.Sentinel.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:       "sentinel",
					Port:       rf.Spec.Sentinel.Port.ToInt32(),
					TargetPort: sentinelTargetPort,
					Protocol:   "TCP",
				},
			},
		},
	}
}

func generateRedisService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.Service {
	name := GetRedisName(rf)
	namespace := rf.Namespace

	selectorLabels := generateSelectorLabels(redisRoleName, rf.Name)
	labels = util.MergeLabels(labels, selectorLabels)
	defaultAnnotations := map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/port":   "http",
		"prometheus.io/path":   "/metrics",
	}
	annotations := util.MergeLabels(defaultAnnotations, rf.Spec.Redis.ServiceAnnotations)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
			Annotations:     annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
			Ports: []corev1.ServicePort{
				{
					Port:     exporterPort,
					Protocol: corev1.ProtocolTCP,
					Name:     exporterPortName,
				},
			},
			Selector: selectorLabels,
		},
	}
}

func generateRedisMasterService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.Service {
	name := GetRedisMasterName(rf)
	namespace := rf.Namespace

	selectorLabels := generateSelectorLabels(redisRoleName, rf.Name)
	selectorLabels = util.MergeLabels(selectorLabels, map[string]string{
		redisRoleLabelKey: redisRoleLabelMaster,
	})
	labels = util.MergeLabels(labels, selectorLabels)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
			Annotations:     rf.Spec.Redis.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       rf.Spec.Redis.Port.ToInt32(),
					TargetPort: intstr.FromString("redis"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: selectorLabels,
		},
	}
}

func generateRedisSlaveService(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.Service {
	name := GetRedisSlaveName(rf)
	namespace := rf.Namespace

	selectorLabels := generateSelectorLabels(redisRoleName, rf.Name)
	selectorLabels = util.MergeLabels(selectorLabels, map[string]string{
		redisRoleLabelKey: redisRoleLabelSlave,
	})
	labels = util.MergeLabels(labels, selectorLabels)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
			Annotations:     rf.Spec.Redis.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       rf.Spec.Redis.Port.ToInt32(),
					TargetPort: intstr.FromString("redis"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: selectorLabels,
		},
	}
}

func generateSentinelConfigMap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.ConfigMap {
	name := GetSentinelName(rf)
	namespace := rf.Namespace

	labels = util.MergeLabels(labels, generateSelectorLabels(sentinelRoleName, rf.Name))

	tmpl, err := template.New("sentinel").Parse(sentinelConfigTemplate)
	if err != nil {
		panic(err)
	}

	var tplOutput bytes.Buffer
	if err := tmpl.Execute(&tplOutput, rf); err != nil {
		panic(err)
	}

	sentinelConfigFileContent := tplOutput.String()

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Data: map[string]string{
			sentinelConfigFileName: sentinelConfigFileContent,
		},
	}
}

func generateRedisConfigMap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference, password string) *corev1.ConfigMap {
	name := GetRedisName(rf)
	labels = util.MergeLabels(labels, generateSelectorLabels(redisRoleName, rf.Name))

	tmpl, err := template.New("redis").Parse(redisConfigTemplate)
	if err != nil {
		panic(err)
	}

	var tplOutput bytes.Buffer
	if err := tmpl.Execute(&tplOutput, rf); err != nil {
		panic(err)
	}

	redisConfigFileContent := tplOutput.String()

	if password != "" {
		redisConfigFileContent = fmt.Sprintf("%s\nmasterauth %s\nrequirepass %s", redisConfigFileContent, password, password)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       rf.Namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Data: map[string]string{
			redisConfigFileName: redisConfigFileContent,
		},
	}
}

func generateRedisShutdownConfigMap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.ConfigMap {
	name := GetRedisShutdownConfigMapName(rf)
	port := rf.Spec.Redis.Port
	namespace := rf.Namespace
	rfName := strings.Replace(strings.ToUpper(rf.Name), "-", "_", -1)

	labels = util.MergeLabels(labels, generateSelectorLabels(redisRoleName, rf.Name))
	shutdownContent := fmt.Sprintf(`master=$(redis-cli -h ${RFS_%[1]v_SERVICE_HOST} -p ${RFS_%[1]v_SERVICE_PORT_SENTINEL} --csv SENTINEL get-master-addr-by-name mymaster | tr ',' ' ' | tr -d '\"' |cut -d' ' -f1)
if [ "$master" = "$(hostname -i)" ]; then
  redis-cli -h ${RFS_%[1]v_SERVICE_HOST} -p ${RFS_%[1]v_SERVICE_PORT_SENTINEL} SENTINEL failover mymaster
  sleep 31
fi
cmd="redis-cli -p %[2]v"
if [ ! -z "${REDIS_PASSWORD}" ]; then
	export REDISCLI_AUTH=${REDIS_PASSWORD}
fi
save_command="${cmd} save"
eval $save_command`, rfName, port)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Data: map[string]string{
			"shutdown.sh": shutdownContent,
		},
	}
}

func generateRedisReadinessConfigMap(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.ConfigMap {
	name := GetRedisReadinessName(rf)
	port := rf.Spec.Redis.Port
	namespace := rf.Namespace

	labels = util.MergeLabels(labels, generateSelectorLabels(redisRoleName, rf.Name))
	readinessContent := fmt.Sprintf(`ROLE="role"
ROLE_MASTER="role:master"
ROLE_SLAVE="role:slave"
IN_SYNC="master_sync_in_progress:1"
NO_MASTER="master_host:127.0.0.1"

cmd="redis-cli -p %[1]v"
if [ ! -z "${REDIS_PASSWORD}" ]; then
	export REDISCLI_AUTH=${REDIS_PASSWORD}
fi

cmd="${cmd} info replication"

check_master(){
		exit 0
}

check_slave(){
		in_sync=$(echo "${cmd} | grep ${IN_SYNC} | tr -d \"\\r\" | tr -d \"\\n\"" | xargs -0 sh -c)
		no_master=$(echo "${cmd} | grep ${NO_MASTER} | tr -d \"\\r\" | tr -d \"\\n\"" |  xargs -0 sh -c)

		if [ -z "$in_sync" ] && [ -z "$no_master" ]; then
				exit 0
		fi

		exit 1
}

role=$(echo "${cmd} | grep $ROLE | tr -d \"\\r\" | tr -d \"\\n\"" | xargs -0 sh -c)
case $role in
		$ROLE_MASTER)
				check_master
				;;
		$ROLE_SLAVE)
				check_slave
				;;
		*)
				echo "unexpected"
				exit 1
esac`, port)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Data: map[string]string{
			"ready.sh": readinessContent,
		},
	}
}

func generateRedisStatefulSet(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *appsv1.StatefulSet {
	name := GetRedisName(rf)
	namespace := rf.Namespace

	redisCommand := getRedisCommand(rf)
	selectorLabels := generateSelectorLabels(redisRoleName, rf.Name)
	labels = util.MergeLabels(labels, selectorLabels)
	labels = util.MergeLabels(labels, generateRedisDefaultRoleLabel())

	labels = util.MergeLabels(labels, generateComponentLabel("redis"))

	volumeMounts := getRedisVolumeMounts(rf)
	volumes := getRedisVolumes(rf)
	terminationGracePeriodSeconds := getTerminationGracePeriodSeconds(rf)

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations:     rf.Annotations,
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: name,
			Replicas:    &rf.Spec.Redis.Replicas,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.OnDeleteStatefulSetStrategyType,
			},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: rf.Spec.Redis.PodAnnotations,
				},
				Spec: corev1.PodSpec{
					Affinity:                      getAffinity(rf.Spec.Redis.Affinity, labels),
					Tolerations:                   rf.Spec.Redis.Tolerations,
					TopologySpreadConstraints:     rf.Spec.Redis.TopologySpreadConstraints,
					NodeSelector:                  rf.Spec.Redis.NodeSelector,
					SecurityContext:               getSecurityContext(rf.Spec.Redis.SecurityContext),
					HostNetwork:                   rf.Spec.Redis.HostNetwork,
					DNSPolicy:                     getDnsPolicy(rf.Spec.Redis.DNSPolicy),
					ImagePullSecrets:              rf.Spec.Redis.ImagePullSecrets,
					PriorityClassName:             rf.Spec.Redis.PriorityClassName,
					ServiceAccountName:            rf.Spec.Redis.ServiceAccountName,
					TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
					Containers: []corev1.Container{
						{
							Name:            "redis",
							Image:           rf.Spec.Redis.Image,
							ImagePullPolicy: pullPolicy(rf.Spec.Redis.ImagePullPolicy),
							SecurityContext: getContainerSecurityContext(rf.Spec.Redis.ContainerSecurityContext),
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: rf.Spec.Redis.Port.ToInt32(),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: volumeMounts,
							Command:      redisCommand,
							Resources:    rf.Spec.Redis.Resources,
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "/redis-shutdown/shutdown.sh"},
									},
								},
							},
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	if rf.Spec.Redis.Storage.PersistentVolumeClaim != nil {
		pvc := corev1.PersistentVolumeClaim{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:              rf.Spec.Redis.Storage.PersistentVolumeClaim.EmbeddedObjectMetadata.Name,
				Labels:            rf.Spec.Redis.Storage.PersistentVolumeClaim.EmbeddedObjectMetadata.Labels,
				Annotations:       rf.Spec.Redis.Storage.PersistentVolumeClaim.EmbeddedObjectMetadata.Annotations,
				CreationTimestamp: metav1.Time{},
			},
			Spec:   rf.Spec.Redis.Storage.PersistentVolumeClaim.Spec,
			Status: rf.Spec.Redis.Storage.PersistentVolumeClaim.Status,
		}
		if !rf.Spec.Redis.Storage.KeepAfterDeletion {
			// Set an owner reference so the persistent volumes are deleted when the RF is
			pvc.OwnerReferences = ownerRefs
		}
		ss.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			pvc,
		}
	}

	if rf.Spec.Redis.CustomLivenessProbe != nil {
		ss.Spec.Template.Spec.Containers[0].LivenessProbe = rf.Spec.Redis.CustomLivenessProbe
	} else {
		ss.Spec.Template.Spec.Containers[0].LivenessProbe = &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			FailureThreshold:    6,
			PeriodSeconds:       15,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						fmt.Sprintf("redis-cli -h $(hostname) -p %[1]v --user pinger --pass pingpass --no-auth-warning ping | grep PONG", rf.Spec.Redis.Port),
					},
				},
			},
		}
	}

	if rf.Spec.Redis.CustomReadinessProbe != nil {
		ss.Spec.Template.Spec.Containers[0].ReadinessProbe = rf.Spec.Redis.CustomReadinessProbe
	} else {
		ss.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "/redis-readiness/ready.sh"},
				},
			},
		}
	}

	if rf.Spec.Redis.CustomStartupProbe != nil {
		ss.Spec.Template.Spec.Containers[0].StartupProbe = rf.Spec.Redis.CustomStartupProbe
	} else if rf.Spec.Redis.StartupConfigMap != "" {
		ss.Spec.Template.Spec.Containers[0].StartupProbe = &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			FailureThreshold:    6,
			PeriodSeconds:       15,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "/redis-startup/startup.sh"},
				},
			},
		}
	}

	if rf.Spec.Redis.Exporter.Enabled {
		exporter := createRedisExporterContainer(rf)
		ss.Spec.Template.Spec.Containers = append(ss.Spec.Template.Spec.Containers, exporter)
	}

	if rf.Spec.Redis.InitContainers != nil {
		initContainers := getInitContainersWithRedisEnv(rf)
		ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, initContainers...)
	}

	if rf.Spec.Redis.ExtraContainers != nil {
		extraContainers := getExtraContainersWithRedisEnv(rf)
		ss.Spec.Template.Spec.Containers = append(ss.Spec.Template.Spec.Containers, extraContainers...)
	}

	redisEnv := getRedisEnv(rf)
	ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, redisEnv...)

	return ss
}

func generateSentinelDeployment(rf *redisfailoverv1.RedisFailover, labels map[string]string, ownerRefs []metav1.OwnerReference) *appsv1.Deployment {
	name := GetSentinelName(rf)
	configMapName := GetSentinelName(rf)
	namespace := rf.Namespace

	sentinelCommand := getSentinelCommand(rf)
	selectorLabels := generateSelectorLabels(sentinelRoleName, rf.Name)
	labels = util.MergeLabels(labels, selectorLabels)

	labels = util.MergeLabels(labels, generateComponentLabel("sentinel"))

	volumeMounts := getSentinelVolumeMounts(rf)
	volumes := getSentinelVolumes(rf, configMapName)

	sd := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &rf.Spec.Sentinel.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: rf.Spec.Sentinel.PodAnnotations,
				},
				Spec: corev1.PodSpec{
					Affinity:                  getAffinity(rf.Spec.Sentinel.Affinity, labels),
					Tolerations:               rf.Spec.Sentinel.Tolerations,
					TopologySpreadConstraints: rf.Spec.Sentinel.TopologySpreadConstraints,
					NodeSelector:              rf.Spec.Sentinel.NodeSelector,
					SecurityContext:           getSecurityContext(rf.Spec.Sentinel.SecurityContext),
					HostNetwork:               rf.Spec.Sentinel.HostNetwork,
					DNSPolicy:                 getDnsPolicy(rf.Spec.Sentinel.DNSPolicy),
					ImagePullSecrets:          rf.Spec.Sentinel.ImagePullSecrets,
					PriorityClassName:         rf.Spec.Sentinel.PriorityClassName,
					ServiceAccountName:        rf.Spec.Sentinel.ServiceAccountName,
					InitContainers: []corev1.Container{
						{
							Name:            "sentinel-config-copy",
							Image:           rf.Spec.Sentinel.Image,
							ImagePullPolicy: pullPolicy(rf.Spec.Sentinel.ImagePullPolicy),
							SecurityContext: getContainerSecurityContext(rf.Spec.Sentinel.ConfigCopy.ContainerSecurityContext),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sentinel-config",
									MountPath: "/redis",
								},
								{
									Name:      "sentinel-config-writable",
									MountPath: "/redis-writable",
								},
							},
							Command: []string{
								"cp",
								fmt.Sprintf("/redis/%s", sentinelConfigFileName),
								fmt.Sprintf("/redis-writable/%s", sentinelConfigFileName),
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "sentinel",
							Image:           rf.Spec.Sentinel.Image,
							ImagePullPolicy: pullPolicy(rf.Spec.Sentinel.ImagePullPolicy),
							SecurityContext: getContainerSecurityContext(rf.Spec.Sentinel.ContainerSecurityContext),
							Ports: []corev1.ContainerPort{
								{
									Name:          "sentinel",
									ContainerPort: rf.Spec.Sentinel.Port.ToInt32(),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: volumeMounts,
							Command:      sentinelCommand,
							Resources:    rf.Spec.Sentinel.Resources,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	if rf.Spec.Sentinel.CustomLivenessProbe != nil {
		sd.Spec.Template.Spec.Containers[0].LivenessProbe = rf.Spec.Sentinel.CustomLivenessProbe
	} else {
		command := "redis-cli -h $(hostname) -p " + strconv.FormatInt(int64(rf.Spec.Sentinel.Port), 10) + " ping"
		sd.Spec.Template.Spec.Containers[0].LivenessProbe = &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						command,
					},
				},
			},
		}
	}

	if rf.Spec.Sentinel.CustomReadinessProbe != nil {
		sd.Spec.Template.Spec.Containers[0].ReadinessProbe = rf.Spec.Sentinel.CustomReadinessProbe
	} else {
		command := "redis-cli -h $(hostname) -p " + strconv.FormatInt(int64(rf.Spec.Sentinel.Port), 10) + " sentinel get-master-addr-by-name mymaster | head -n 1 | grep -vq '127.0.0.1'"
		sd.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						command,
					},
				},
			},
		}
	}

	if rf.Spec.Sentinel.CustomStartupProbe != nil {
		sd.Spec.Template.Spec.Containers[0].StartupProbe = rf.Spec.Sentinel.CustomStartupProbe
	} else if rf.Spec.Sentinel.StartupConfigMap != "" {
		sd.Spec.Template.Spec.Containers[0].StartupProbe = &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			FailureThreshold:    6,
			PeriodSeconds:       15,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "/sentinel-startup/startup.sh"},
				},
			},
		}
	}

	if rf.Spec.Sentinel.Exporter.Enabled {
		exporter := createSentinelExporterContainer(rf)
		sd.Spec.Template.Spec.Containers = append(sd.Spec.Template.Spec.Containers, exporter)
	}
	if rf.Spec.Sentinel.InitContainers != nil {
		sd.Spec.Template.Spec.InitContainers = append(sd.Spec.Template.Spec.InitContainers, rf.Spec.Sentinel.InitContainers...)
	}

	if rf.Spec.Sentinel.ExtraContainers != nil {
		sd.Spec.Template.Spec.Containers = append(sd.Spec.Template.Spec.Containers, rf.Spec.Sentinel.ExtraContainers...)
	}

	return sd
}

func generatePodDisruptionBudget(name string, namespace string, labels map[string]string, ownerRefs []metav1.OwnerReference, minAvailable intstr.IntOrString) *policyv1.PodDisruptionBudget {
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}
}

var exporterDefaultResourceRequirements = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(exporterDefaultLimitCPU),
		corev1.ResourceMemory: resource.MustParse(exporterDefaultLimitMemory),
	},
	Requests: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(exporterDefaultRequestCPU),
		corev1.ResourceMemory: resource.MustParse(exporterDefaultRequestMemory),
	},
}

func createRedisExporterContainer(rf *redisfailoverv1.RedisFailover) corev1.Container {
	resources := exporterDefaultResourceRequirements
	if rf.Spec.Redis.Exporter.Resources != nil {
		resources = *rf.Spec.Redis.Exporter.Resources
	}
	container := corev1.Container{
		Name:            exporterContainerName,
		Image:           rf.Spec.Redis.Exporter.Image,
		ImagePullPolicy: pullPolicy(rf.Spec.Redis.Exporter.ImagePullPolicy),
		SecurityContext: getContainerSecurityContext(rf.Spec.Redis.Exporter.ContainerSecurityContext),
		Args:            rf.Spec.Redis.Exporter.Args,
		Env: append(rf.Spec.Redis.Exporter.Env, corev1.EnvVar{
			Name: "REDIS_ALIAS",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		),
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: exporterPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: resources,
	}

	redisEnv := getRedisEnv(rf)
	container.Env = append(container.Env, redisEnv...)

	return container
}

func createSentinelExporterContainer(rf *redisfailoverv1.RedisFailover) corev1.Container {
	resources := exporterDefaultResourceRequirements
	if rf.Spec.Sentinel.Exporter.Resources != nil {
		resources = *rf.Spec.Sentinel.Exporter.Resources
	}

	command := "redis://127.0.0.1:" + strconv.FormatInt(int64(rf.Spec.Sentinel.Port), 10)

	container := corev1.Container{
		Name:            sentinelExporterContainerName,
		Image:           rf.Spec.Sentinel.Exporter.Image,
		ImagePullPolicy: pullPolicy(rf.Spec.Sentinel.Exporter.ImagePullPolicy),
		SecurityContext: getContainerSecurityContext(rf.Spec.Sentinel.Exporter.ContainerSecurityContext),
		Args:            rf.Spec.Sentinel.Exporter.Args,
		Env: append(rf.Spec.Sentinel.Exporter.Env, corev1.EnvVar{
			Name: "REDIS_ALIAS",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		}, corev1.EnvVar{
			Name:  "REDIS_EXPORTER_WEB_LISTEN_ADDRESS",
			Value: fmt.Sprintf("0.0.0.0:%[1]v", sentinelExporterPort),
		}, corev1.EnvVar{
			Name:  "REDIS_ADDR",
			Value: command,
		},
		),
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: sentinelExporterPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: resources,
	}

	return container
}

func getAffinity(affinity *corev1.Affinity, labels map[string]string) *corev1.Affinity {
	if affinity != nil {
		return affinity
	}

	// Return a SOFT anti-affinity
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: hostnameTopologyKey,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: labels,
						},
					},
				},
			},
		},
	}
}

func getSecurityContext(secctx *corev1.PodSecurityContext) *corev1.PodSecurityContext {
	if secctx != nil {
		return secctx
	}

	defaultUserAndGroup := int64(1000)
	runAsNonRoot := true

	return &corev1.PodSecurityContext{
		RunAsUser:    &defaultUserAndGroup,
		RunAsGroup:   &defaultUserAndGroup,
		RunAsNonRoot: &runAsNonRoot,
		FSGroup:      &defaultUserAndGroup,
	}
}

func getContainerSecurityContext(secctx *corev1.SecurityContext) *corev1.SecurityContext {
	if secctx != nil {
		return secctx
	}

	capabilities := &corev1.Capabilities{
		Add: []corev1.Capability{},
		Drop: []corev1.Capability{
			"ALL",
		},
	}
	privileged := false
	defaultUserAndGroup := int64(1000)
	runAsNonRoot := true
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true

	return &corev1.SecurityContext{
		Capabilities:             capabilities,
		Privileged:               &privileged,
		RunAsUser:                &defaultUserAndGroup,
		RunAsGroup:               &defaultUserAndGroup,
		RunAsNonRoot:             &runAsNonRoot,
		ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
	}
}

func getDnsPolicy(dnspolicy corev1.DNSPolicy) corev1.DNSPolicy {
	if dnspolicy == "" {
		return corev1.DNSClusterFirst
	}
	return dnspolicy
}

func getQuorum(rf *redisfailoverv1.RedisFailover) int32 {
	return rf.Spec.Sentinel.Replicas/2 + 1
}

func getRedisVolumeMounts(rf *redisfailoverv1.RedisFailover) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      redisConfigurationVolumeName,
			MountPath: "/redis",
		},
		{
			Name:      redisShutdownConfigurationVolumeName,
			MountPath: "/redis-shutdown",
		},
		{
			Name:      redisReadinessVolumeName,
			MountPath: "/redis-readiness",
		},
		{
			Name:      getRedisDataVolumeName(rf),
			MountPath: "/data",
		},
	}

	if rf.Spec.Redis.StartupConfigMap != "" {
		startupVolumeMount := corev1.VolumeMount{
			Name:      redisStartupConfigurationVolumeName,
			MountPath: "/redis-startup",
		}

		volumeMounts = append(volumeMounts, startupVolumeMount)
	}

	if rf.Spec.Redis.ExtraVolumeMounts != nil {
		volumeMounts = append(volumeMounts, rf.Spec.Redis.ExtraVolumeMounts...)
	}

	return volumeMounts
}

func getSentinelVolumeMounts(rf *redisfailoverv1.RedisFailover) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "sentinel-config-writable",
			MountPath: "/redis",
		},
	}

	if rf.Spec.Sentinel.StartupConfigMap != "" {
		startupVolumeMount := corev1.VolumeMount{
			Name:      "sentinel-startup-config",
			MountPath: "/sentinel-startup",
		}
		volumeMounts = append(volumeMounts, startupVolumeMount)
	}
	if rf.Spec.Sentinel.ExtraVolumeMounts != nil {
		volumeMounts = append(volumeMounts, rf.Spec.Sentinel.ExtraVolumeMounts...)
	}

	return volumeMounts
}

func getRedisVolumes(rf *redisfailoverv1.RedisFailover) []corev1.Volume {
	configMapName := GetRedisName(rf)
	shutdownConfigMapName := GetRedisShutdownConfigMapName(rf)
	readinessConfigMapName := GetRedisReadinessName(rf)

	executeMode := int32(0744)
	volumes := []corev1.Volume{
		{
			Name: redisConfigurationVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
				},
			},
		},
		{
			Name: redisShutdownConfigurationVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: shutdownConfigMapName,
					},
					DefaultMode: &executeMode,
				},
			},
		},
		{
			Name: redisReadinessVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: readinessConfigMapName,
					},
					DefaultMode: &executeMode,
				},
			},
		},
	}

	if rf.Spec.Redis.StartupConfigMap != "" {
		startupVolumeName := rf.Spec.Redis.StartupConfigMap
		startupVolume := corev1.Volume{
			Name: redisStartupConfigurationVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: startupVolumeName,
					},
					DefaultMode: &executeMode,
				},
			},
		}
		volumes = append(volumes, startupVolume)
	}

	if rf.Spec.Redis.ExtraVolumes != nil {
		volumes = append(volumes, rf.Spec.Redis.ExtraVolumes...)
	}

	dataVolume := getRedisDataVolume(rf)
	if dataVolume != nil {
		volumes = append(volumes, *dataVolume)
	}

	return volumes
}

func getSentinelVolumes(rf *redisfailoverv1.RedisFailover, configMapName string) []corev1.Volume {
	executeMode := int32(0744)

	volumes := []corev1.Volume{
		{
			Name: "sentinel-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
				},
			},
		},
		{
			Name: "sentinel-config-writable",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	if rf.Spec.Sentinel.StartupConfigMap != "" {
		startupVolumeName := rf.Spec.Sentinel.StartupConfigMap
		startupVolume := corev1.Volume{
			Name: sentinelStartupConfigurationVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: startupVolumeName,
					},
					DefaultMode: &executeMode,
				},
			},
		}
		volumes = append(volumes, startupVolume)
	}

	if rf.Spec.Sentinel.ExtraVolumes != nil {
		volumes = append(volumes, rf.Spec.Sentinel.ExtraVolumes...)
	}

	return volumes
}

func getRedisDataVolume(rf *redisfailoverv1.RedisFailover) *corev1.Volume {
	// This will find the volumed desired by the user. If no volume defined
	// an EmptyDir will be used by default
	switch {
	case rf.Spec.Redis.Storage.PersistentVolumeClaim != nil:
		return nil
	case rf.Spec.Redis.Storage.EmptyDir != nil:
		return &corev1.Volume{
			Name: redisStorageVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: rf.Spec.Redis.Storage.EmptyDir,
			},
		}
	default:
		return &corev1.Volume{
			Name: redisStorageVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
	}
}

func getRedisDataVolumeName(rf *redisfailoverv1.RedisFailover) string {
	switch {
	case rf.Spec.Redis.Storage.PersistentVolumeClaim != nil:
		return rf.Spec.Redis.Storage.PersistentVolumeClaim.Name
	case rf.Spec.Redis.Storage.EmptyDir != nil:
		return redisStorageVolumeName
	default:
		return redisStorageVolumeName
	}
}

func getRedisCommand(rf *redisfailoverv1.RedisFailover) []string {
	if len(rf.Spec.Redis.Command) > 0 {
		return rf.Spec.Redis.Command
	}
	return []string{
		"redis-server",
		fmt.Sprintf("/redis/%s", redisConfigFileName),
	}
}

func getSentinelCommand(rf *redisfailoverv1.RedisFailover) []string {
	if len(rf.Spec.Sentinel.Command) > 0 {
		return rf.Spec.Sentinel.Command
	}
	return []string{
		"redis-server",
		fmt.Sprintf("/redis/%s", sentinelConfigFileName),
		"--sentinel",
	}
}

func pullPolicy(specPolicy corev1.PullPolicy) corev1.PullPolicy {
	if specPolicy == "" {
		return corev1.PullAlways
	}
	return specPolicy
}

func getTerminationGracePeriodSeconds(rf *redisfailoverv1.RedisFailover) int64 {
	if rf.Spec.Redis.TerminationGracePeriodSeconds > 0 {
		return rf.Spec.Redis.TerminationGracePeriodSeconds
	}
	return 30
}

func getExtraContainersWithRedisEnv(rf *redisfailoverv1.RedisFailover) []corev1.Container {
	env := getRedisEnv(rf)
	extraContainers := getContainersWithRedisEnv(rf.Spec.Redis.ExtraContainers, env)

	return extraContainers
}

func getInitContainersWithRedisEnv(rf *redisfailoverv1.RedisFailover) []corev1.Container {
	env := getRedisEnv(rf)
	initContainers := getContainersWithRedisEnv(rf.Spec.Redis.InitContainers, env)

	return initContainers
}

func getContainersWithRedisEnv(cs []corev1.Container, e []corev1.EnvVar) []corev1.Container {
	var containers []corev1.Container
	for _, c := range cs {
		c.Env = append(c.Env, e...)
		containers = append(containers, c)
	}

	return containers
}

func getRedisEnv(rf *redisfailoverv1.RedisFailover) []corev1.EnvVar {
	var env []corev1.EnvVar

	env = append(env, corev1.EnvVar{
		Name:  "REDIS_ADDR",
		Value: fmt.Sprintf("redis://127.0.0.1:%[1]v", rf.Spec.Redis.Port),
	})

	env = append(env, corev1.EnvVar{
		Name:  "REDIS_PORT",
		Value: fmt.Sprintf("%[1]v", rf.Spec.Redis.Port),
	})

	env = append(env, corev1.EnvVar{
		Name:  "REDIS_USER",
		Value: "default",
	})

	if rf.Spec.Auth.SecretPath != "" {
		env = append(env, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: rf.Spec.Auth.SecretPath,
					},
					Key: "password",
				},
			},
		})
	}

	return env
}

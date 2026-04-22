{{- define "redisfailover-bundle.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "redisfailover-bundle.instance" -}}
{{- printf "%s-%s" .Values.applicationName .Values.environment -}}
{{- end -}}

{{- define "redisfailover-bundle.priorityLabels" -}}
{{- $root := index . "root" -}}
{{- $component := index . "component" -}}
app.kubernetes.io/name: {{ $root.Values.applicationName }}
app.kubernetes.io/instance: {{ include "redisfailover-bundle.instance" $root }}
app.kubernetes.io/component: {{ $component }}
app.kubernetes.io/part-of: priority-deploy
app.kubernetes.io/managed-by: {{ $root.Release.Service }}
{{- end -}}

{{- define "redisfailover-bundle.operatorImage" -}}
{{- $image := .Values.operator.image -}}
{{- $tag := $image.tag | default .Chart.AppVersion -}}
{{- if $image.digest -}}
{{- printf "%s:%s@%s" $image.repository $tag $image.digest -}}
{{- else -}}
{{- printf "%s:%s" $image.repository $tag -}}
{{- end -}}
{{- end -}}

{{- define "redisfailover-bundle.storageKeepAfterDeletion" -}}
{{- $cfg := . -}}
{{- if hasKey $cfg "storage" -}}
{{- $storage := index $cfg "storage" | default dict -}}
{{- if hasKey $storage "keepAfterDeletion" -}}
{{- index $storage "keepAfterDeletion" -}}
{{- else if hasKey $cfg "keepAfterDeletion" -}}
{{- index $cfg "keepAfterDeletion" -}}
{{- else -}}
false
{{- end -}}
{{- else if hasKey $cfg "keepAfterDeletion" -}}
{{- index $cfg "keepAfterDeletion" -}}
{{- else -}}
false
{{- end -}}
{{- end -}}

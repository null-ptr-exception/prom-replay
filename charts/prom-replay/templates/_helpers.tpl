{{- define "prom-replay.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "prom-replay.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "prom-replay.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "prom-replay.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "prom-replay.image" -}}
{{- $registry := .global.imageRegistry | default .imageValues.registry | default "" -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry .imageValues.repository (.imageValues.tag | toString) -}}
{{- else -}}
{{- printf "%s:%s" .imageValues.repository (.imageValues.tag | toString) -}}
{{- end -}}
{{- end }}

{{- define "prom-replay.vmURL" -}}
http://{{ include "prom-replay.fullname" . }}-victoriametrics:8428
{{- end }}

{{- define "prom-replay.minioEndpoint" -}}
{{ include "prom-replay.fullname" . }}-minio:9000
{{- end }}

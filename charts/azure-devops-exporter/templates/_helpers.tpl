{{- define "azure-devops-exporter.name" -}}
{{- .Values.nameOverride | default .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "azure-devops-exporter.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := .Values.nameOverride | default .Chart.Name -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "azure-devops-exporter.labels" -}}
app.kubernetes.io/name: {{ include "azure-devops-exporter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app: {{ include "azure-devops-exporter.name" . }}
{{- end -}}

{{- define "azure-devops-exporter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "azure-devops-exporter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: {{ include "azure-devops-exporter.name" . }}
{{- end -}}

{{- define "azure-devops-exporter.secretName" -}}
{{- .Values.azureDevOps.existingSecret | default (include "azure-devops-exporter.fullname" .) -}}
{{- end -}}

{{- define "labels" }}
app: event-exporter
{{ if .Values.labels }}
{{ range $key,$value := .Values.labels }}
{{ $key }}: {{ $value }}
{{ end }}
{{end}}
{{- end }}

{{- define "annotations" }}
{{ if .Values.annotations }}
annotations:
{{ range $key,$value := .Values.annotations }}
  {{ $key }}: {{ $value }}
{{ end }}
{{ end }}
{{- end }}

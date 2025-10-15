[Partition]
Type={{ .Type }}
{{- if .Format }}
Format={{ .Format }}
{{- end }}
{{- if .Size }}
SizeMinBytes={{ .Size }}M
SizeMaxBytes={{ .Size }}M
{{- end }}
{{- if .Label }}
Label={{ .Label }}
{{- end }}
{{- if .UUID }}
UUID={{ .UUID }}
{{- end }}
{{- if .CopyFiles }}
CopyFiles={{ .CopyFiles }}
{{- end }}
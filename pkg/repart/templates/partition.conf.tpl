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
{{- range $cpy := .CopyFiles }}
CopyFiles={{ $cpy }}
{{- end }}
{{- range $excl := .Excludes }}
ExcludeFiles={{ $excl }}
{{- end }}
{{- if .ReadOnly }}
ReadOnly={{ .ReadOnly }}
{{- end }}

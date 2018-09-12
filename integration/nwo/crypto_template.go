/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

const DefaultCryptoTemplate = `---
{{ with $w := . -}}
OrdererOrgs:{{ range .OrdererOrgs }}
- Name: {{ .Name }}
  Domain: {{ .Domain }}
  EnableNodeOUs: {{ .EnableNodeOUs }}
  {{- if .CA }}
  CA:{{ if .CA.Hostname }}
    Hostname: {{ .CA.Hostname }}
  {{- end -}}
  {{- end }}
  Specs:{{ range $w.OrderersInOrg .Name }}
  - Hostname: {{ .Name }}
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  {{- end }}
{{- end }}

PeerOrgs:{{ range .PeerOrgs }}
- Name: {{ .Name }}
  Domain: {{ .Domain }}
  EnableNodeOUs: {{ .EnableNodeOUs }}
  {{- if .CA }}
  CA:{{ if .CA.Hostname }}
    hostname: {{ .CA.Hostname }}
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  {{- end }}
  {{- end }}
  Users:
    Count: {{ .Users }}
  Specs:{{ range $w.PeersInOrg .Name }}
  - Hostname: {{ .Name }}
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  {{- end }}
{{- end }}
{{- end }}
`

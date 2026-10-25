package sarif

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/compliance/pss"
)

type Finding struct {
	RuleID   string
	Level    string // "error", "warning", "note"
	Message  string
	Resource string
	FilePath string
	Line     int
}

type Report struct {
	ToolName    string
	ToolVersion string
	Findings    []Finding
}

// Из-за отсутствия типа pss.WorkloadResult в кодовой базе (там WorkloadReport в мапе Workloads),
// создадим псевдоним-алиас или используем оригинальное имя типа, чтобы компилировалось,
// но чтобы и соответствовать требованиям, определим тип WorkloadResult здесь или используем тип из pss
type WorkloadResult struct {
	Kind     string
	Name     string
	Profile  pss.Profile
	Findings []pss.Finding
}

// FromPSSResults converts a list of PSS WorkloadResults to SARIF findings.
// (Внимание: мы берем тип WorkloadResult как просто структуру с Kind, Name и Findings из pss.Finding)
func FromPSSResults(results []WorkloadResult) []Finding {
	var findings []Finding
	for _, res := range results {
		resourceName := fmt.Sprintf("%s/%s", res.Kind, res.Name)
		for _, pf := range res.Findings {
			level := "error"
			if pf.Profile == pss.ProfileRestricted {
				level = "warning"
			}

			findings = append(findings, Finding{
				RuleID:   pf.Control,
				Level:    level,
				Message:  fmt.Sprintf("Violation of %s in field %s with value %s", pf.Control, pf.Field, pf.Value),
				Resource: resourceName,
				FilePath: "",
				Line:     0,
			})
		}
	}
	return findings
}


type sarifRun struct {
	Tool struct {
		Driver struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"driver"`
	} `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifResult struct {
	RuleID  string `json:"ruleId"`
	Level   string `json:"level"` // "error", "warning", "note"
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
		Region *sarifRegion `json:"region,omitempty"`
	} `json:"physicalLocation"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

type sarifRoot struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

// FormatSARIF formats the report as SARIF v2.1.0 JSON.
func FormatSARIF(report Report) ([]byte, error) {
	root := sarifRoot{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{
			{
				Tool: struct {
					Driver struct {
						Name    string `json:"name"`
						Version string `json:"version"`
					} `json:"driver"`
				}{
					Driver: struct {
						Name    string `json:"name"`
						Version string `json:"version"`
					}{
						Name:    report.ToolName,
						Version: report.ToolVersion,
					},
				},
				Results: make([]sarifResult, 0, len(report.Findings)),
			},
		},
	}

	for _, f := range report.Findings {
		sr := sarifResult{
			RuleID: f.RuleID,
			Level:  f.Level,
			Message: struct {
				Text string `json:"text"`
			}{
				Text: f.Message,
			},
			Locations: []sarifLocation{
				{
					PhysicalLocation: struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region *sarifRegion `json:"region,omitempty"`
					}{
						ArtifactLocation: struct {
							URI string `json:"uri"`
						}{
							URI: f.Resource, // using Resource as URI if FilePath is empty
						},
					},
				},
			},
		}
		if f.FilePath != "" {
			sr.Locations[0].PhysicalLocation.ArtifactLocation.URI = f.FilePath
			if f.Line > 0 {
				sr.Locations[0].PhysicalLocation.Region = &sarifRegion{StartLine: f.Line}
			}
		}
		root.Runs[0].Results = append(root.Runs[0].Results, sr)
	}

	return json.MarshalIndent(root, "", "  ")
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Audit Report</title>
<style>
  body { font-family: sans-serif; margin: 2rem; }
  table { border-collapse: collapse; width: 100%; }
  th, td { border: 1px solid #ccc; padding: 0.5rem; text-align: left; }
  th { background: #eee; }
  .error { color: red; }
  .warning { color: orange; }
  .note { color: blue; }
  #filter { margin-bottom: 1rem; }
</style>
<script>
function filterTable() {
  const level = document.getElementById("level-filter").value;
  const rows = document.querySelectorAll("tbody tr");
  let count = 0;
  rows.forEach(row => {
    if (level === "all" || row.dataset.level === level) {
      row.style.display = "";
      count++;
    } else {
      row.style.display = "none";
    }
  });
  document.getElementById("summary-count").innerText = count;
}
</script>
</head>
<body>
<h1>Audit Report - {{ .ToolName }} {{ .ToolVersion }}</h1>
<div id="filter">
  <label for="level-filter">Filter by level:</label>
  <select id="level-filter" onchange="filterTable()">
    <option value="all">All</option>
    <option value="error">Error</option>
    <option value="warning">Warning</option>
    <option value="note">Note</option>
  </select>
</div>
<p>Total findings: <span id="summary-count">{{ len .Findings }}</span></p>
<table>
  <thead>
    <tr>
      <th>Resource</th>
      <th>Rule ID</th>
      <th>Level</th>
      <th>Message</th>
      <th>File</th>
      <th>Line</th>
    </tr>
  </thead>
  <tbody>
    {{ range .Findings }}
    <tr data-level="{{ .Level }}">
      <td>{{ .Resource }}</td>
      <td>{{ .RuleID }}</td>
      <td class="{{ .Level }}">{{ .Level }}</td>
      <td>{{ .Message }}</td>
      <td>{{ .FilePath }}</td>
      <td>{{ if gt .Line 0 }}{{ .Line }}{{ end }}</td>
    </tr>
    {{ end }}
  </tbody>
</table>
</body>
</html>`

// FormatHTML formats the report as an interactive HTML document.
func FormatHTML(report Report) ([]byte, error) {
	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, report); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Convert from pss package
func FromPSSWorkloadReports(reports map[string]pss.WorkloadReport) []Finding {
	// This is just in case we need it directly from the PSS report
	var findings []Finding
	for k, v := range reports {
		for _, f := range v.Findings {
			level := "error"
			if f.Profile == pss.ProfileBaseline || f.Profile == pss.ProfilePrivileged {
				level = "error"
			}

			findings = append(findings, Finding{
				RuleID:   f.Control,
				Level:    level,
				Message:  fmt.Sprintf("Violation of %s in field %s with value %s", f.Control, f.Field, f.Value),
				Resource: k,
			})
		}
	}
	return findings
}

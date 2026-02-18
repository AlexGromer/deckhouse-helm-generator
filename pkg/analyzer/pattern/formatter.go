package pattern

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Formatter formats analysis reports.
type Formatter struct {
	// ColorEnabled enables ANSI color output
	ColorEnabled bool
}

// NewFormatter creates a new formatter.
func NewFormatter(color bool) *Formatter {
	return &Formatter{
		ColorEnabled: color,
	}
}

// FormatReport formats report as human-readable text.
func (f *Formatter) FormatReport(report *Report) string {
	var sb strings.Builder

	// Header
	sb.WriteString(f.formatHeader("Deckhouse Helm Generator - Analysis Report"))
	sb.WriteString("\n\n")

	// Each section
	for i, section := range report.Sections {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(f.formatSection(section))
	}

	return sb.String()
}

// FormatJSON formats report as JSON.
func (f *Formatter) FormatJSON(report *Report) (string, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FormatMarkdown formats report as Markdown.
func (f *Formatter) FormatMarkdown(report *Report) string {
	var sb strings.Builder

	// Title
	sb.WriteString("# Deckhouse Helm Generator - Analysis Report\n\n")

	// Each section
	for _, section := range report.Sections {
		sb.WriteString(fmt.Sprintf("## %s\n\n", section.Title))
		if section.Description != "" {
			sb.WriteString(fmt.Sprintf("*%s*\n\n", section.Description))
		}

		for _, item := range section.Items {
			sb.WriteString(fmt.Sprintf("### %s\n\n", item.Title))
			sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", item.Content))
		}
	}

	return sb.String()
}

// formatHeader formats a header.
func (f *Formatter) formatHeader(text string) string {
	line := strings.Repeat("=", len(text))
	return fmt.Sprintf("%s\n%s\n%s", line, text, line)
}

// formatSection formats a report section.
func (f *Formatter) formatSection(section ReportSection) string {
	var sb strings.Builder

	// Section title
	sb.WriteString(f.colorize(fmt.Sprintf("▶ %s", section.Title), "cyan", true))
	sb.WriteString("\n")

	// Description
	if section.Description != "" {
		sb.WriteString(f.colorize(section.Description, "gray", false))
		sb.WriteString("\n")
	}

	sb.WriteString(strings.Repeat("-", 80))
	sb.WriteString("\n\n")

	// Items
	for _, item := range section.Items {
		sb.WriteString(f.formatItem(item))
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatItem formats a report item.
func (f *Formatter) formatItem(item ReportItem) string {
	var sb strings.Builder

	// Icon and title based on level
	icon := f.getLevelIcon(item.Level)
	color := f.getLevelColor(item.Level)

	sb.WriteString(f.colorize(fmt.Sprintf("%s %s", icon, item.Title), color, true))
	sb.WriteString("\n")

	// Content (indented)
	lines := strings.Split(item.Content, "\n")
	for _, line := range lines {
		if line != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", line))
		}
	}

	return sb.String()
}

// getLevelIcon returns icon for level.
func (f *Formatter) getLevelIcon(level string) string {
	icons := map[string]string{
		"success": "✓",
		"info":    "ℹ",
		"warning": "⚠",
		"error":   "✗",
	}
	if icon, ok := icons[level]; ok {
		return icon
	}
	return "•"
}

// getLevelColor returns color for level.
func (f *Formatter) getLevelColor(level string) string {
	colors := map[string]string{
		"success": "green",
		"info":    "blue",
		"warning": "yellow",
		"error":   "red",
	}
	if color, ok := colors[level]; ok {
		return color
	}
	return "white"
}

// colorize adds ANSI color codes if enabled.
func (f *Formatter) colorize(text, color string, bold bool) string {
	if !f.ColorEnabled {
		return text
	}

	codes := map[string]string{
		"red":     "31",
		"green":   "32",
		"yellow":  "33",
		"blue":    "34",
		"magenta": "35",
		"cyan":    "36",
		"gray":    "90",
		"white":   "37",
	}

	code, ok := codes[color]
	if !ok {
		return text
	}

	if bold {
		return fmt.Sprintf("\033[1;%sm%s\033[0m", code, text)
	}
	return fmt.Sprintf("\033[%sm%s\033[0m", code, text)
}

// FormatSummary formats a brief summary of the analysis.
func (f *Formatter) FormatSummary(result *AnalysisResult) string {
	var sb strings.Builder

	sb.WriteString(f.colorize("Analysis Summary", "cyan", true))
	sb.WriteString("\n\n")

	// Key metrics
	sb.WriteString(fmt.Sprintf("Services: %d | Resources: %d | Complexity: %d/100 | Coupling: %d/100\n",
		result.Metrics.TotalServices,
		result.Metrics.TotalResources,
		result.Metrics.ComplexityScore,
		result.Metrics.CouplingScore))

	// Primary pattern
	sb.WriteString(fmt.Sprintf("Primary Pattern: %s (confidence: %d%%)\n",
		result.PrimaryPattern,
		result.Confidence))

	// Recommended strategy
	sb.WriteString(fmt.Sprintf("Recommended Strategy: %s\n",
		result.RecommendedStrategy))

	// Best practices summary
	violations := 0
	for _, practice := range result.BestPractices {
		if !practice.Compliant && practice.Severity != SeverityInfo {
			violations++
		}
	}
	if violations > 0 {
		sb.WriteString(f.colorize(fmt.Sprintf("\n⚠ %d best practice violations found", violations), "yellow", false))
	} else {
		sb.WriteString(f.colorize("\n✓ All best practices checks passed", "green", false))
	}
	sb.WriteString("\n")

	return sb.String()
}

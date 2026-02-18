package pattern

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Recommender generates actionable recommendations based on analysis.
type Recommender struct {
	analyzer *Analyzer
}

// NewRecommender creates a new recommender.
func NewRecommender(analyzer *Analyzer) *Recommender {
	return &Recommender{
		analyzer: analyzer,
	}
}

// GenerateReport generates a comprehensive analysis report.
func (r *Recommender) GenerateReport(graph *types.ResourceGraph) *Report {
	// Run analysis
	result := r.analyzer.Analyze(graph)

	report := &Report{
		AnalysisResult: result,
		Sections:       make([]ReportSection, 0),
	}

	// Section 1: Overview
	report.Sections = append(report.Sections, r.generateOverviewSection(result, graph))

	// Section 2: Architecture patterns
	report.Sections = append(report.Sections, r.generatePatternsSection(result))

	// Section 3: Best practices
	report.Sections = append(report.Sections, r.generateBestPracticesSection(result))

	// Section 4: Chart strategy
	report.Sections = append(report.Sections, r.generateStrategySection(result))

	// Section 5: Action items
	report.Sections = append(report.Sections, r.generateActionItemsSection(result))

	return report
}

// generateOverviewSection creates overview section.
func (r *Recommender) generateOverviewSection(result *AnalysisResult, graph *types.ResourceGraph) ReportSection {
	items := []ReportItem{
		{
			Title:   "Total Services",
			Content: fmt.Sprintf("%d services detected", result.Metrics.TotalServices),
			Level:   "info",
		},
		{
			Title:   "Total Resources",
			Content: fmt.Sprintf("%d Kubernetes resources", result.Metrics.TotalResources),
			Level:   "info",
		},
		{
			Title:   "Complexity Score",
			Content: fmt.Sprintf("%d/100 (%s)", result.Metrics.ComplexityScore, r.complexityLevel(result.Metrics.ComplexityScore)),
			Level:   r.complexityLevelSeverity(result.Metrics.ComplexityScore),
		},
		{
			Title:   "Coupling Score",
			Content: fmt.Sprintf("%d/100 (%s)", result.Metrics.CouplingScore, r.couplingLevel(result.Metrics.CouplingScore)),
			Level:   r.couplingLevelSeverity(result.Metrics.CouplingScore),
		},
	}

	// Resource breakdown
	if len(result.Metrics.ResourcesByKind) > 0 {
		breakdown := "Resource distribution:\n"
		for kind, count := range result.Metrics.ResourcesByKind {
			breakdown += fmt.Sprintf("  • %s: %d\n", kind, count)
		}
		items = append(items, ReportItem{
			Title:   "Resource Types",
			Content: strings.TrimSpace(breakdown),
			Level:   "info",
		})
	}

	return ReportSection{
		Title:       "Overview",
		Description: "High-level analysis of your Kubernetes resources",
		Items:       items,
	}
}

// generatePatternsSection creates patterns section.
func (r *Recommender) generatePatternsSection(result *AnalysisResult) ReportSection {
	items := []ReportItem{
		{
			Title:   "Primary Pattern",
			Content: fmt.Sprintf("%s (confidence: %d%%)", result.PrimaryPattern, result.Confidence),
			Level:   "success",
		},
	}

	if len(result.DetectedPatterns) > 1 {
		patterns := make([]string, 0)
		for _, p := range result.DetectedPatterns {
			if p != result.PrimaryPattern {
				patterns = append(patterns, string(p))
			}
		}
		items = append(items, ReportItem{
			Title:   "Secondary Patterns",
			Content: strings.Join(patterns, ", "),
			Level:   "info",
		})
	}

	// Pattern explanation
	items = append(items, ReportItem{
		Title:   "What This Means",
		Content: r.explainPattern(result.PrimaryPattern, result.Metrics),
		Level:   "info",
	})

	return ReportSection{
		Title:       "Architecture Patterns",
		Description: "Detected architectural patterns in your application",
		Items:       items,
	}
}

// generateBestPracticesSection creates best practices section.
func (r *Recommender) generateBestPracticesSection(result *AnalysisResult) ReportSection {
	items := []ReportItem{}

	// Group by severity
	bySeverity := make(map[Severity][]BestPractice)
	for _, practice := range result.BestPractices {
		if !practice.Compliant {
			bySeverity[practice.Severity] = append(bySeverity[practice.Severity], practice)
		}
	}

	// Summary
	totalIssues := 0
	for _, practices := range bySeverity {
		totalIssues += len(practices)
	}

	if totalIssues == 0 {
		items = append(items, ReportItem{
			Title:   "Status",
			Content: "✓ All best practices checks passed!",
			Level:   "success",
		})
	} else {
		summary := fmt.Sprintf("Found %d best practice violations:\n", totalIssues)
		if critical := len(bySeverity[SeverityCritical]); critical > 0 {
			summary += fmt.Sprintf("  • Critical: %d\n", critical)
		}
		if errors := len(bySeverity[SeverityError]); errors > 0 {
			summary += fmt.Sprintf("  • Error: %d\n", errors)
		}
		if warnings := len(bySeverity[SeverityWarning]); warnings > 0 {
			summary += fmt.Sprintf("  • Warning: %d\n", warnings)
		}

		items = append(items, ReportItem{
			Title:   "Summary",
			Content: strings.TrimSpace(summary),
			Level:   "warning",
		})
	}

	// Detail each severity level
	for _, severity := range []Severity{SeverityCritical, SeverityError, SeverityWarning} {
		if practices, ok := bySeverity[severity]; ok && len(practices) > 0 {
			for _, practice := range practices {
				content := practice.Description + "\n\n"
				content += "Affected resources:\n"
				for i, res := range practice.AffectedResources {
					if i >= 5 {
						content += fmt.Sprintf("  ... and %d more\n", len(practice.AffectedResources)-5)
						break
					}
					content += fmt.Sprintf("  • %s/%s\n", res.GVK.Kind, res.Name)
				}
				content += "\nRecommendations:\n"
				for _, rec := range practice.Recommendations {
					content += fmt.Sprintf("  • %s\n", rec)
				}

				level := "warning"
				if severity == SeverityCritical {
					level = "error"
				} else if severity == SeverityError {
					level = "warning"
				}

				items = append(items, ReportItem{
					Title:   fmt.Sprintf("[%s] %s", strings.ToUpper(string(severity)), practice.Title),
					Content: strings.TrimSpace(content),
					Level:   level,
				})
			}
		}
	}

	return ReportSection{
		Title:       "Best Practices",
		Description: "Kubernetes and Helm best practices compliance",
		Items:       items,
	}
}

// generateStrategySection creates strategy recommendation section.
func (r *Recommender) generateStrategySection(result *AnalysisResult) ReportSection {
	items := []ReportItem{
		{
			Title:   "Recommended Strategy",
			Content: fmt.Sprintf("%s\n\n%s", result.RecommendedStrategy, getStrategyDescription(result.RecommendedStrategy)),
			Level:   "success",
		},
		{
			Title:   "Rationale",
			Content: getStrategyRationale(result.RecommendedStrategy, result.PrimaryPattern, result.Metrics),
			Level:   "info",
		},
	}

	// Implementation steps
	steps := getStrategySteps(result.RecommendedStrategy)
	if len(steps) > 0 {
		content := ""
		for i, step := range steps {
			content += fmt.Sprintf("%d. %s\n", i+1, step)
		}
		items = append(items, ReportItem{
			Title:   "Implementation Steps",
			Content: strings.TrimSpace(content),
			Level:   "info",
		})
	}

	// Alternative strategies
	alternatives := r.getAlternativeStrategies(result)
	if len(alternatives) > 0 {
		content := "Alternative approaches to consider:\n"
		for _, alt := range alternatives {
			content += fmt.Sprintf("  • %s: %s\n", alt.Strategy, alt.Reason)
		}
		items = append(items, ReportItem{
			Title:   "Alternatives",
			Content: strings.TrimSpace(content),
			Level:   "info",
		})
	}

	return ReportSection{
		Title:       "Chart Strategy",
		Description: "Recommended Helm chart organization",
		Items:       items,
	}
}

// generateActionItemsSection creates prioritized action items.
func (r *Recommender) generateActionItemsSection(result *AnalysisResult) ReportSection {
	reportItems := []ReportItem{}

	// Collect all action items
	actionItems := make([]ActionItem, 0)

	// From recommendations
	for _, rec := range result.Recommendations {
		for _, step := range rec.ImplementationSteps {
			actionItems = append(actionItems, ActionItem{
				Priority:    rec.Priority,
				Title:       step,
				Category:    rec.Title,
				Impact:      rec.Impact,
				Effort:      r.estimateEffort(step),
				AutoFixable: false,
			})
		}
	}

	// From best practices violations
	for _, practice := range result.BestPractices {
		if !practice.Compliant && practice.AutoFixable {
			actionItems = append(actionItems, ActionItem{
				Priority:    r.severityToPriority(practice.Severity),
				Title:       fmt.Sprintf("Auto-fix: %s", practice.Title),
				Category:    practice.Category,
				Impact:      "Improved compliance",
				Effort:      "Low (automatic)",
				AutoFixable: true,
			})
		}
	}

	// Sort by priority
	sort.Slice(actionItems, func(i, j int) bool {
		return actionItems[i].Priority < actionItems[j].Priority
	})

	// Group by priority
	byPriority := make(map[int][]ActionItem)
	for _, item := range actionItems {
		byPriority[item.Priority] = append(byPriority[item.Priority], item)
	}

	// Create report items
	for priority := 1; priority <= 3; priority++ {
		if actionItemsForPriority, ok := byPriority[priority]; ok && len(actionItemsForPriority) > 0 {
			content := ""
			for i, item := range actionItemsForPriority {
				if i >= 10 {
					content += fmt.Sprintf("... and %d more\n", len(actionItemsForPriority)-10)
					break
				}
				marker := "•"
				if item.AutoFixable {
					marker = "⚡"
				}
				content += fmt.Sprintf("%s %s\n", marker, item.Title)
				content += fmt.Sprintf("  Category: %s | Effort: %s\n", item.Category, item.Effort)
				if item.Impact != "" {
					content += fmt.Sprintf("  Impact: %s\n", item.Impact)
				}
				content += "\n"
			}

			level := "info"
			if priority == 1 {
				level = "warning"
			}

			reportItems = append(reportItems, ReportItem{
				Title:   fmt.Sprintf("Priority %d Items", priority),
				Content: strings.TrimSpace(content),
				Level:   level,
			})
		}
	}

	if len(reportItems) == 0 {
		reportItems = []ReportItem{
			{
				Title:   "Status",
				Content: "No action items - you're all set!",
				Level:   "success",
			},
		}
	}

	return ReportSection{
		Title:       "Action Items",
		Description: "Prioritized list of improvements (⚡ = auto-fixable)",
		Items:       reportItems,
	}
}

// Helper functions

func (r *Recommender) complexityLevel(score int) string {
	if score < 30 {
		return "Low"
	} else if score < 60 {
		return "Medium"
	}
	return "High"
}

func (r *Recommender) complexityLevelSeverity(score int) string {
	if score < 30 {
		return "success"
	} else if score < 60 {
		return "info"
	}
	return "warning"
}

func (r *Recommender) couplingLevel(score int) string {
	if score < 20 {
		return "Low - well decoupled"
	} else if score < 50 {
		return "Medium"
	}
	return "High - tightly coupled"
}

func (r *Recommender) couplingLevelSeverity(score int) string {
	if score < 20 {
		return "success"
	} else if score < 50 {
		return "info"
	}
	return "warning"
}

func (r *Recommender) explainPattern(pattern ArchitecturePattern, metrics AnalysisMetrics) string {
	explanations := map[ArchitecturePattern]string{
		PatternMicroservices: fmt.Sprintf("Your application follows a microservices architecture with %d independent services. This suggests separate deployments and potentially service mesh.",
			metrics.TotalServices),
		PatternMonolith: "Your application is a monolithic service. This is suitable for simpler applications or early-stage products.",
		PatternStateful: fmt.Sprintf("Your application has %d stateful services requiring persistent storage. Ensure proper backup and disaster recovery.",
			metrics.StatefulServices),
		PatternStateless: "Your application is stateless, which is ideal for horizontal scaling and rolling updates.",
		PatternDeckhouse: fmt.Sprintf("Detected %d Deckhouse-specific resources. This requires special handling for Deckhouse platform integration.",
			metrics.DeckhouseResourceCount),
	}

	if exp, ok := explanations[pattern]; ok {
		return exp
	}
	return "Custom architecture pattern detected."
}

func (r *Recommender) getAlternativeStrategies(result *AnalysisResult) []AlternativeStrategy {
	alternatives := make([]AlternativeStrategy, 0)

	// Suggest alternatives based on current recommendation
	switch result.RecommendedStrategy {
	case StrategyUniversal:
		if result.Metrics.TotalServices > 3 {
			alternatives = append(alternatives, AlternativeStrategy{
				Strategy: StrategyUmbrella,
				Reason:   "Better modularity for multiple services",
			})
		}

	case StrategySeparate:
		alternatives = append(alternatives, AlternativeStrategy{
			Strategy: StrategyUmbrella,
			Reason:   "Easier coordination while maintaining independence",
		})

	case StrategyUmbrella:
		if result.Metrics.CouplingScore > 70 {
			alternatives = append(alternatives, AlternativeStrategy{
				Strategy: StrategyUniversal,
				Reason:   "High coupling suggests unified deployment might be simpler",
			})
		}
	}

	return alternatives
}

func (r *Recommender) estimateEffort(step string) string {
	lower := strings.ToLower(step)
	if strings.Contains(lower, "generate") || strings.Contains(lower, "dhg") {
		return "Low (automated)"
	}
	if strings.Contains(lower, "review") || strings.Contains(lower, "check") {
		return "Low"
	}
	if strings.Contains(lower, "configure") || strings.Contains(lower, "add") {
		return "Medium"
	}
	if strings.Contains(lower, "refactor") || strings.Contains(lower, "redesign") {
		return "High"
	}
	return "Medium"
}

func (r *Recommender) severityToPriority(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 1
	case SeverityError:
		return 1
	case SeverityWarning:
		return 2
	default:
		return 3
	}
}

// Report types

// Report represents a comprehensive analysis report.
type Report struct {
	AnalysisResult *AnalysisResult
	Sections       []ReportSection
}

// ReportSection is a section of the report.
type ReportSection struct {
	Title       string
	Description string
	Items       []ReportItem
}

// ReportItem is an item within a section.
type ReportItem struct {
	Title   string
	Content string
	Level   string // success, info, warning, error
}

// ActionItem represents a prioritized action.
type ActionItem struct {
	Priority    int
	Title       string
	Category    string
	Impact      string
	Effort      string
	AutoFixable bool
}

// AlternativeStrategy represents an alternative chart strategy.
type AlternativeStrategy struct {
	Strategy ChartStrategy
	Reason   string
}

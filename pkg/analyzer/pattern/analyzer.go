package pattern

import (
	"fmt"
	"sort"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Analyzer analyzes resource graph for patterns and best practices.
type Analyzer struct {
	detectors []PatternDetector
	checkers  []BestPracticeChecker
}

// NewAnalyzer creates a new pattern analyzer.
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		detectors: make([]PatternDetector, 0),
		checkers:  make([]BestPracticeChecker, 0),
	}
}

// AddDetector registers a pattern detector.
func (a *Analyzer) AddDetector(d PatternDetector) {
	a.detectors = append(a.detectors, d)
}

// AddChecker registers a best practice checker.
func (a *Analyzer) AddChecker(c BestPracticeChecker) {
	a.checkers = append(a.checkers, c)
}

// Analyze performs comprehensive analysis on resource graph.
func (a *Analyzer) Analyze(graph *types.ResourceGraph) *AnalysisResult {
	result := &AnalysisResult{
		DetectedPatterns: make([]ArchitecturePattern, 0),
		BestPractices:    make([]BestPractice, 0),
		Recommendations:  make([]Recommendation, 0),
	}

	// Compute metrics
	result.Metrics = a.computeMetrics(graph)

	// Detect patterns
	patternCounts := make(map[ArchitecturePattern]int)
	for _, detector := range a.detectors {
		patterns := detector.Detect(graph)
		for _, pattern := range patterns {
			patternCounts[pattern]++
			// Add pattern if not already present
			found := false
			for _, p := range result.DetectedPatterns {
				if p == pattern {
					found = true
					break
				}
			}
			if !found {
				result.DetectedPatterns = append(result.DetectedPatterns, pattern)
			}
		}
	}

	// Determine primary pattern
	result.PrimaryPattern = a.determinePrimaryPattern(patternCounts, result.Metrics)

	// Check best practices
	for _, checker := range a.checkers {
		practices := checker.Check(graph)
		result.BestPractices = append(result.BestPractices, practices...)
	}

	// Generate recommendations
	result.RecommendedStrategy = a.recommendStrategy(result.PrimaryPattern, result.Metrics)
	result.Confidence = a.calculateConfidence(result)
	result.Recommendations = a.generateRecommendations(result, graph)

	return result
}

// computeMetrics calculates various metrics from the graph.
func (a *Analyzer) computeMetrics(graph *types.ResourceGraph) AnalysisMetrics {
	metrics := AnalysisMetrics{
		TotalServices:  len(graph.Groups),
		TotalResources: len(graph.Resources),
		ResourcesByKind: make(map[string]int),
	}

	// Count resources by kind
	for key := range graph.Resources {
		kind := key.GVK.Kind
		metrics.ResourcesByKind[kind]++
	}

	// Calculate average resources per service
	if metrics.TotalServices > 0 {
		metrics.AverageResourcesPerService = float64(metrics.TotalResources) / float64(metrics.TotalServices)
	}

	// Count stateful services (with PVCs)
	for _, group := range graph.Groups {
		for _, resource := range group.Resources {
			if resource.Original.GVK.Kind == "PersistentVolumeClaim" ||
				resource.Original.GVK.Kind == "StatefulSet" {
				metrics.StatefulServices++
				break
			}
		}
	}

	// Count services with Ingress
	for _, group := range graph.Groups {
		for _, resource := range group.Resources {
			if resource.Original.GVK.Kind == "Ingress" {
				metrics.ServicesWithIngress++
				break
			}
		}
	}

	// Count services with secrets
	for _, group := range graph.Groups {
		for _, resource := range group.Resources {
			if resource.Original.GVK.Kind == "Secret" {
				metrics.ServicesWithSecrets++
				break
			}
		}
	}

	// Count Deckhouse resources
	for key := range graph.Resources {
		if key.GVK.Group == "deckhouse.io" {
			metrics.DeckhouseResourceCount++
		}
	}

	// Calculate complexity score
	metrics.ComplexityScore = a.calculateComplexityScore(metrics)

	// Calculate coupling score
	metrics.CouplingScore = a.calculateCouplingScore(graph)

	return metrics
}

// calculateComplexityScore computes complexity (0-100).
func (a *Analyzer) calculateComplexityScore(metrics AnalysisMetrics) int {
	score := 0

	// Base complexity from resource count
	if metrics.TotalResources > 50 {
		score += 30
	} else if metrics.TotalResources > 20 {
		score += 20
	} else if metrics.TotalResources > 10 {
		score += 10
	}

	// Complexity from service count
	if metrics.TotalServices > 10 {
		score += 30
	} else if metrics.TotalServices > 5 {
		score += 20
	} else if metrics.TotalServices > 2 {
		score += 10
	}

	// Complexity from stateful services
	score += metrics.StatefulServices * 10

	// Complexity from resource type diversity
	score += len(metrics.ResourcesByKind) * 2

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}

// calculateCouplingScore computes inter-service coupling (0-100, lower is better).
func (a *Analyzer) calculateCouplingScore(graph *types.ResourceGraph) int {
	if len(graph.Groups) <= 1 {
		return 0
	}

	// Count cross-service relationships
	crossServiceRels := 0
	totalRels := len(graph.Relationships)

	for _, rel := range graph.Relationships {
		fromService := ""
		toService := ""

		if fromRes, ok := graph.Resources[rel.From]; ok {
			fromService = fromRes.ServiceName
		}
		if toRes, ok := graph.Resources[rel.To]; ok {
			toService = toRes.ServiceName
		}

		if fromService != "" && toService != "" && fromService != toService {
			crossServiceRels++
		}
	}

	if totalRels == 0 {
		return 0
	}

	// Coupling score: percentage of cross-service relationships
	coupling := (crossServiceRels * 100) / totalRels

	return coupling
}

// determinePrimaryPattern determines the main architecture pattern.
func (a *Analyzer) determinePrimaryPattern(counts map[ArchitecturePattern]int, metrics AnalysisMetrics) ArchitecturePattern {
	// Find pattern with highest count
	var maxPattern ArchitecturePattern
	maxCount := 0

	for pattern, count := range counts {
		if count > maxCount {
			maxCount = count
			maxPattern = pattern
		}
	}

	// Apply heuristics if unclear
	if maxPattern == "" {
		// Deckhouse resources present
		if metrics.DeckhouseResourceCount > 0 {
			return PatternDeckhouse
		}

		// Multiple services with low coupling
		if metrics.TotalServices > 3 && metrics.CouplingScore < 30 {
			return PatternMicroservices
		}

		// Single service or highly coupled
		if metrics.TotalServices <= 2 || metrics.CouplingScore > 70 {
			return PatternMonolith
		}

		// Stateful services
		if metrics.StatefulServices > 0 {
			return PatternStateful
		}

		// Default to stateless
		return PatternStateless
	}

	return maxPattern
}

// recommendStrategy recommends chart organization strategy.
func (a *Analyzer) recommendStrategy(pattern ArchitecturePattern, metrics AnalysisMetrics) ChartStrategy {
	// Deckhouse pattern: specialized handling
	if pattern == PatternDeckhouse {
		if metrics.TotalServices > 1 {
			return StrategyHybrid
		}
		return StrategyUniversal
	}

	// Microservices: separate charts if many services
	if pattern == PatternMicroservices {
		if metrics.TotalServices > 5 {
			return StrategySeparate
		}
		if metrics.TotalServices > 2 && metrics.CouplingScore < 20 {
			return StrategyUmbrella
		}
	}

	// Monolith or few services: universal chart
	if pattern == PatternMonolith || metrics.TotalServices <= 2 {
		return StrategyUniversal
	}

	// Operator pattern: library chart
	if pattern == PatternOperator {
		return StrategyLibrary
	}

	// High complexity: umbrella or hybrid
	if metrics.ComplexityScore > 70 {
		return StrategyUmbrella
	}

	// Default: universal
	return StrategyUniversal
}

// calculateConfidence computes confidence score (0-100).
func (a *Analyzer) calculateConfidence(result *AnalysisResult) int {
	confidence := 50 // Base confidence

	// More resources = higher confidence
	if result.Metrics.TotalResources > 20 {
		confidence += 20
	} else if result.Metrics.TotalResources > 10 {
		confidence += 10
	}

	// Multiple detected patterns = lower confidence
	if len(result.DetectedPatterns) > 3 {
		confidence -= 15
	} else if len(result.DetectedPatterns) == 1 {
		confidence += 15
	}

	// Deckhouse resources = higher confidence for Deckhouse pattern
	if result.PrimaryPattern == PatternDeckhouse && result.Metrics.DeckhouseResourceCount > 0 {
		confidence += 15
	}

	// Cap at 0-100
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 100 {
		confidence = 100
	}

	return confidence
}

// generateRecommendations creates high-level recommendations.
func (a *Analyzer) generateRecommendations(result *AnalysisResult, graph *types.ResourceGraph) []Recommendation {
	recommendations := make([]Recommendation, 0)

	// Recommendation 1: Chart strategy
	recommendations = append(recommendations, Recommendation{
		Priority: 1,
		Title:    "Recommended Chart Strategy",
		Description: getStrategyDescription(result.RecommendedStrategy),
		Rationale: getStrategyRationale(result.RecommendedStrategy, result.PrimaryPattern, result.Metrics),
		Impact:    "Optimal chart organization for maintainability and deployment flexibility",
		ImplementationSteps: getStrategySteps(result.RecommendedStrategy),
	})

	// Recommendation 2: Best practices compliance
	nonCompliantCount := 0
	for _, practice := range result.BestPractices {
		if !practice.Compliant && practice.Severity != SeverityInfo {
			nonCompliantCount++
		}
	}

	if nonCompliantCount > 0 {
		recommendations = append(recommendations, Recommendation{
			Priority: 2,
			Title:    "Address Best Practice Violations",
			Description: formatString("Found %d best practice violations that should be addressed", nonCompliantCount),
			Rationale: "Following Kubernetes and Helm best practices improves security, reliability, and maintainability",
			Impact:    "Reduced operational issues and improved security posture",
			ImplementationSteps: []string{
				"Review the best practices section for specific violations",
				"Prioritize critical and error severity items",
				"Apply auto-fixable improvements where available",
			},
		})
	}

	// Recommendation 3: Complexity management
	if result.Metrics.ComplexityScore > 60 {
		recommendations = append(recommendations, Recommendation{
			Priority: 3,
			Title:    "Consider Complexity Reduction",
			Description: formatString("Complexity score is %d/100, consider modularization", result.Metrics.ComplexityScore),
			Rationale: "High complexity increases maintenance burden and deployment risk",
			Impact:    "Easier troubleshooting, faster deployments, reduced cognitive load",
			ImplementationSteps: []string{
				"Break down large services into smaller components",
				"Use subcharts for independent modules",
				"Consider umbrella chart pattern for coordination",
			},
		})
	}

	// Sort by priority
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].Priority < recommendations[j].Priority
	})

	return recommendations
}

func getStrategyDescription(strategy ChartStrategy) string {
	descriptions := map[ChartStrategy]string{
		StrategyUniversal: "Single chart containing all services with centralized values.yaml",
		StrategySeparate:  "Separate independent charts for each service",
		StrategyLibrary:   "Shared library chart with thin service-specific wrappers",
		StrategyUmbrella:  "Umbrella chart managing multiple subchart dependencies",
		StrategyHybrid:    "Combination of universal and separate charts based on service characteristics",
	}
	return descriptions[strategy]
}

func getStrategyRationale(strategy ChartStrategy, pattern ArchitecturePattern, metrics AnalysisMetrics) string {
	switch strategy {
	case StrategyUniversal:
		return formatString("With %d services and %s pattern, a unified chart simplifies management while maintaining flexibility",
			metrics.TotalServices, pattern)
	case StrategySeparate:
		return formatString("With %d loosely-coupled services (coupling: %d%%), separate charts enable independent lifecycles",
			metrics.TotalServices, metrics.CouplingScore)
	case StrategyUmbrella:
		return formatString("With %d services and moderate coupling (%d%%), umbrella chart balances independence and coordination",
			metrics.TotalServices, metrics.CouplingScore)
	case StrategyLibrary:
		return "Operator pattern benefits from shared templates with service-specific customization"
	case StrategyHybrid:
		return formatString("Mixed Deckhouse and application resources benefit from hybrid approach")
	default:
		return "Recommended based on detected patterns"
	}
}

func getStrategySteps(strategy ChartStrategy) []string {
	steps := map[ChartStrategy][]string{
		StrategyUniversal: {
			"Generate single chart with dhg --mode universal",
			"Organize services in values.yaml under 'services' key",
			"Use service.enabled flags for optional components",
		},
		StrategySeparate: {
			"Generate separate charts with dhg --mode separate",
			"Define clear service boundaries and APIs",
			"Manage inter-service dependencies explicitly",
		},
		StrategyUmbrella: {
			"Create umbrella chart with dependencies in Chart.yaml",
			"Generate subcharts for each service",
			"Coordinate versions and configurations through parent chart",
		},
		StrategyLibrary: {
			"Create library chart with shared templates",
			"Generate thin wrapper charts for each service",
			"Import library chart as dependency",
		},
	}
	return steps[strategy]
}

func formatString(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// DefaultAnalyzer returns analyzer with all default detectors and checkers.
func DefaultAnalyzer() *Analyzer {
	a := NewAnalyzer()

	// Add pattern detectors
	a.AddDetector(NewMicroservicesDetector())
	a.AddDetector(NewStatefulDetector())
	a.AddDetector(NewDeckhouseDetector())
	a.AddDetector(NewJobDetector())
	a.AddDetector(NewOperatorDetector())

	// Add best practice checkers
	a.AddChecker(NewResourceLimitsChecker())
	a.AddChecker(NewSecurityChecker())
	a.AddChecker(NewHighAvailabilityChecker())
	a.AddChecker(NewInitContainerChecker())
	a.AddChecker(NewQoSClassChecker())
	a.AddChecker(NewStatefulSetPatternChecker())
	a.AddChecker(NewDaemonSetPatternChecker())
	a.AddChecker(NewGracefulShutdownChecker())
	a.AddChecker(NewPodSecurityStandardsChecker())

	return a
}

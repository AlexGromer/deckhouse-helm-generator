// Package pattern provides pattern analysis and best practices detection.
package pattern

import (
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ArchitecturePattern represents detected architecture pattern.
type ArchitecturePattern string

const (
	// PatternMicroservices - множество независимых сервисов
	PatternMicroservices ArchitecturePattern = "microservices"

	// PatternMonolith - монолитное приложение
	PatternMonolith ArchitecturePattern = "monolith"

	// PatternStateful - stateful приложение с данными
	PatternStateful ArchitecturePattern = "stateful"

	// PatternStateless - stateless приложение
	PatternStateless ArchitecturePattern = "stateless"

	// PatternSidecar - pattern с sidecar контейнерами
	PatternSidecar ArchitecturePattern = "sidecar"

	// PatternDaemonSet - DaemonSet pattern (node-level services)
	PatternDaemonSet ArchitecturePattern = "daemonset"

	// PatternJob - batch/job processing pattern
	PatternJob ArchitecturePattern = "job"

	// PatternOperator - Kubernetes operator pattern
	PatternOperator ArchitecturePattern = "operator"

	// PatternDeckhouse - Deckhouse-specific pattern
	PatternDeckhouse ArchitecturePattern = "deckhouse"
)

// ChartStrategy represents recommended chart organization strategy.
type ChartStrategy string

const (
	// StrategyUniversal - один chart для всех сервисов
	StrategyUniversal ChartStrategy = "universal"

	// StrategySeparate - отдельные charts для каждого сервиса
	StrategySeparate ChartStrategy = "separate"

	// StrategyLibrary - библиотечный chart + thin wrappers
	StrategyLibrary ChartStrategy = "library"

	// StrategyUmbrella - umbrella chart с dependencies
	StrategyUmbrella ChartStrategy = "umbrella"

	// StrategyHybrid - комбинированная стратегия
	StrategyHybrid ChartStrategy = "hybrid"
)

// Severity represents severity level of a finding.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// BestPractice represents a detected best practice or anti-pattern.
type BestPractice struct {
	// ID is unique identifier
	ID string

	// Title is short description
	Title string

	// Description is detailed explanation
	Description string

	// Category categorizes the practice
	Category string

	// Severity indicates importance
	Severity Severity

	// Compliant indicates if resources follow this practice
	Compliant bool

	// Recommendations are specific actions to improve
	Recommendations []string

	// AffectedResources lists resources that violate this practice
	AffectedResources []types.ResourceKey

	// AutoFixable indicates if this can be auto-fixed
	AutoFixable bool
}

// AnalysisResult contains pattern analysis results.
type AnalysisResult struct {
	// DetectedPatterns lists all detected architecture patterns
	DetectedPatterns []ArchitecturePattern

	// PrimaryPattern is the dominant pattern
	PrimaryPattern ArchitecturePattern

	// RecommendedStrategy is the suggested chart strategy
	RecommendedStrategy ChartStrategy

	// Confidence is confidence score (0-100)
	Confidence int

	// BestPractices lists all detected practices
	BestPractices []BestPractice

	// Metrics contains various metrics
	Metrics AnalysisMetrics

	// Recommendations are high-level architectural recommendations
	Recommendations []Recommendation
}

// AnalysisMetrics contains quantitative analysis metrics.
type AnalysisMetrics struct {
	// TotalServices is number of detected services
	TotalServices int

	// TotalResources is total resource count
	TotalResources int

	// ResourcesByKind maps kind to count
	ResourcesByKind map[string]int

	// AverageResourcesPerService is average resources per service
	AverageResourcesPerService float64

	// StatefulServices is count of services with PVCs
	StatefulServices int

	// ServicesWithIngress is count of services with Ingress
	ServicesWithIngress int

	// ServicesWithSecrets is count using secrets
	ServicesWithSecrets int

	// ComplexityScore is overall complexity (0-100)
	ComplexityScore int

	// CouplingScore indicates inter-service coupling (0-100, lower is better)
	CouplingScore int

	// DeckhouseResourceCount is count of Deckhouse-specific resources
	DeckhouseResourceCount int
}

// Recommendation represents a high-level recommendation.
type Recommendation struct {
	// Priority indicates importance (1 = highest)
	Priority int

	// Title is short recommendation
	Title string

	// Description is detailed explanation
	Description string

	// Rationale explains why this is recommended
	Rationale string

	// Impact describes expected impact
	Impact string

	// ImplementationSteps are concrete steps
	ImplementationSteps []string
}

// PatternDetector detects specific patterns.
type PatternDetector interface {
	// Detect analyzes the graph and returns detected patterns
	Detect(graph *types.ResourceGraph) []ArchitecturePattern

	// Name returns detector name
	Name() string
}

// BestPracticeChecker checks for best practices.
type BestPracticeChecker interface {
	// Check analyzes resources and returns findings
	Check(graph *types.ResourceGraph) []BestPractice

	// Name returns checker name
	Name() string

	// Category returns practice category
	Category() string
}

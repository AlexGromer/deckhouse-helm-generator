package detector

import (
	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer"
)

// RegisterAll registers all default detectors with the analyzer.
func RegisterAll(a *analyzer.DefaultAnalyzer) {
	a.AddDetector(NewLabelSelectorDetector())
	a.AddDetector(NewNameReferenceDetector())
	a.AddDetector(NewVolumeMountDetector())
	a.AddDetector(NewAnnotationDetector())
}

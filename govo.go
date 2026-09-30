// Package govo provides an Analyzer for protecting Go value object types.
package govo

import (
	"golang.org/x/tools/go/analysis"

	"github.com/aqyuki/govo/internal/analyzer"
)

// Analyzer reports operations that bypass a protected value object's API.
var Analyzer *analysis.Analyzer = analyzer.Analyzer

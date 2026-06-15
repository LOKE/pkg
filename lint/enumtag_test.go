package lint_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/LOKE/pkg/lint"
)

func TestEnumTagAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), lint.EnumTagAnalyzer, "p")
}

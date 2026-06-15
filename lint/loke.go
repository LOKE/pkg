// Package lint provides LOKE's golangci-lint module plugin, registered under the
// name "loke". It bundles LOKE-specific analyzers so every LOKE module can
// enforce the same checks from a single custom golangci-lint build. Add new
// checks by appending their analyzer to Analyzers.
package lint

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("loke", New)
}

// Analyzers is the set of analyzers the "loke" plugin runs. Append new LOKE
// checks here to enforce them across every module that enables the plugin.
var Analyzers = []*analysis.Analyzer{
	EnumTagAnalyzer,
}

// New constructs the loke plugin for golangci-lint. It takes no settings.
func New(_ any) (register.LinterPlugin, error) {
	return plugin{}, nil
}

type plugin struct{}

func (plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return Analyzers, nil
}

func (plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

package gorules

import (
	"testing"

	"github.com/quasilyte/go-ruleguard/analyzer"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestRules(t *testing.T) {
	t.Parallel()

	testdata := analysistest.TestData()

	rules := "rules.go" // if we are to separate rules in different files, here we would write them (e.g., rule1.go,rule2.go,rule3.go)
	if err := analyzer.Analyzer.Flags.Set("rules", rules); err != nil {
		t.Fatalf("set rules flag: %v", err)
	}

	analysistest.Run(t, testdata, analyzer.Analyzer, "./...")
}

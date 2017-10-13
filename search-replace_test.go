package main

import (
	"testing"
)

func BenchmarkFix(b *testing.B) {
	test := `s:0:"https://automattic.com";`
	for i := 0; i < b.N; i++ {
		fix(test)
	}
}

func TestFix(t *testing.T) {
	var tests = []struct {
		testName string
		from     string
		to       string
	}{
		{
			testName: "Empty string",
			from:     `s:10:"";`,
			to:       `s:0:"";`,
		},
		{
			testName: "Empty string (corrected)",
			from:     `s:0:"";`,
			to:       `s:0:"";`,
		},
		{
			testName: "Empty string (escaped quotes)",
			from:     `s:0:\"\";`,
			to:       `s:0:\"\";`,
		},
		{
			testName: "Line break",
			from:     `s:0:"line\\nbreak";`,
			to:       `s:11:"line\\nbreak";`,
		},
		{
			testName: "Escaped URL",
			from:     `s:0:"https:\\/\\/automattic.com";`,
			to:       `s:24:"https:\\/\\/automattic.com";`,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			fixed := fix(test.from)
			if fixed != test.to {
				t.Error("Expected:", test.to, "Actual:", fixed)
			}
		})
	}
}

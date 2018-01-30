package main

import (
	"testing"
)

func BenchmarkFix(b *testing.B) {
	test := `s:0:\"https://automattic.com\";`
	for i := 0; i < b.N; i++ {
		fix(test)
	}
}

func BenchmarkSimpleReplace(b *testing.B) {
	line := "http://automattic.com"
	from := "http:"
	to := "https:"
	for i := 0; i < b.N; i++ {
		replaceAndFix(line, map[string]string{
			from: to,
		})
	}
}

func BenchmarkSerializedReplace(b *testing.B) {
	line := `s:0:\"http://automattic.com\";`
	from := "http://automattic.com"
	to := "https://automattic.com"
	for i := 0; i < b.N; i++ {
		replaceAndFix(line, map[string]string{
			from: to,
		})
	}
}

func TestReplace(t *testing.T) {
	var tests = []struct {
		testName string
		in       string
		out      string
		from     string
		to       string
	}{
		{
			testName: "http to https",

			from: "http://automattic.com",
			to:   "https://automattic.com",

			in:  `s:21:\"http://automattic.com\";`,
			out: `s:22:\"https://automattic.com\";`,
		},
		{
			testName: "URL in SQL",

			from: "http://automattic.com",
			to:   "https://automattic.com",

			in:  `('s:21:\"http://automattic.com\";'),('s:21:\"http://automattic.com\";')`,
			out: `('s:22:\"https://automattic.com\";'),('s:22:\"https://automattic.com\";')`,
		},
		{
			testName: "only fix updated strings",

			from: "http://automattic.com",
			to:   "https://automattic.com",

			in:  `('s:21:\"http://automattic.com\";'),('s:21:\"https://a8c.com\";')`,
			out: `('s:22:\"https://automattic.com\";'),('s:21:\"https://a8c.com\";')`,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			replaced := replaceAndFix(test.in, map[string]string{
				test.from: test.to,
			})

			if replaced != test.out {
				t.Error("Expected:", test.out, "Actual:", replaced)
			}
		})
	}
}

func TestMultiReplace(t *testing.T) {
	var tests = []struct {
		testName     string
		in           string
		out          string
		replacements map[string]string
	}{
		{
			testName: "simple test",
			in:       "http://automattic.com",
			out:      "https://automattic.org",
			replacements: map[string]string{
				"http:":          "https:",
				"automattic.com": "automattic.org",
			},
		},
		{
			testName: "overlapping",
			in:       "http://automattic.com",
			out:      "https://automattic.org",
			replacements: map[string]string{
				"http:":            "https:",
				"//automattic.com": "//automattic.org",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			replaced := replaceAndFix(test.in, test.replacements)

			if replaced != test.out {
				t.Error("Expected:", test.out, "Actual:", replaced)
			}
		})
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
			from:     `s:10:\"\";`,
			to:       `s:0:\"\";`,
		},
		{
			testName: "Empty string (corrected)",
			from:     `s:0:\"\";`,
			to:       `s:0:\"\";`,
		},
		{
			testName: "Empty string (escaped quotes)",
			from:     `s:0:\"\";`,
			to:       `s:0:\"\";`,
		},
		{
			testName: "Line break",
			from:     `s:0:\"line\\nbreak\";`,
			to:       `s:11:\"line\\nbreak\";`,
		},
		{
			testName: "Escaped URL",
			from:     `s:0:\"https:\\/\\/automattic.com\";`,
			to:       `s:24:\"https:\\/\\/automattic.com\";`,
		},
		{
			testName: "Correctly count multibyte characters",
			from:     `s:0:\"Does it work with emoji? 🙃\";`,
			to:       `s:29:\"Does it work with emoji? 🙃\";`,
		},
		{
			testName: "Many escaped characters (including escaped backslash)",
			from:     `s:0:\"\t\r\n \t\r\n \t\r\n \\ <a href=\"https://example.com\">Many\tescaped\tcharacters</a>\";`,
			to:       `s:71:\"\t\r\n \t\r\n \t\r\n \\ <a href=\"https://example.com\">Many\tescaped\tcharacters</a>\";`,
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

func TestInput(t *testing.T) {
	var tests = []struct {
		testName string
		in       string
		valid    bool
	}{
		{
			testName: "Simple domain name",
			in:       "automattic.com",
			valid:    true,
		},
		{
			testName: "Short string",
			in:       "s:",
			valid:    false,
		},
		{
			testName: "SQL string",
			in:       "),(",
			valid:    false,
		},
		{
			testName: "Serialization structure",
			in:       "a:4:",
			valid:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			valid := validInput(test.in, minInLength)
			if valid != test.valid {
				t.Error("Expected:", test.valid, "Actual:", valid)
			}
		})
	}
}

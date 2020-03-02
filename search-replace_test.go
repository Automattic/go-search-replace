package main

import (
	"bytes"
	"testing"
)

func BenchmarkFix(b *testing.B) {
	test := []byte(`s:0:\"https://automattic.com\";`)
	for i := 0; i < b.N; i++ {
		fix(test)
	}
}

func BenchmarkSimpleReplace(b *testing.B) {
	line := []byte("http://automattic.com")
	from := []byte("http:")
	to := []byte("https:")
	for i := 0; i < b.N; i++ {
		replaceAndFix(line, []*Replacement{
			&Replacement{
				From: from,
				To:   to,
			},
		})
	}
}

func BenchmarkSerializedReplace(b *testing.B) {
	line := []byte(`s:0:\"http://automattic.com\";`)
	from := []byte("http://automattic.com")
	to := []byte("https://automattic.com")
	for i := 0; i < b.N; i++ {
		replaceAndFix(line, []*Replacement{
			&Replacement{
				From: from,
				To:   to,
			},
		})
	}
}

func TestReplace(t *testing.T) {
	var tests = []struct {
		testName string
		in       []byte
		out      []byte
		from     []byte
		to       []byte
	}{
		{
			testName: "http to https",

			from: []byte("http://automattic.com"),
			to:   []byte("https://automattic.com"),

			in:  []byte(`s:21:\"http://automattic.com\";`),
			out: []byte(`s:22:\"https://automattic.com\";`),
		},
		{
			testName: "URL in SQL",

			from: []byte("http://automattic.com"),
			to:   []byte("https://automattic.com"),

			in:  []byte(`('s:21:\"http://automattic.com\";'),('s:21:\"http://automattic.com\";')`),
			out: []byte(`('s:22:\"https://automattic.com\";'),('s:22:\"https://automattic.com\";')`),
		},
		{
			testName: "only fix updated strings",

			from: []byte("http://automattic.com"),
			to:   []byte("https://automattic.com"),

			in:  []byte(`('s:21:\"http://automattic.com\";'),('s:21:\"https://a8c.com\";')`),
			out: []byte(`('s:22:\"https://automattic.com\";'),('s:21:\"https://a8c.com\";')`),
		},
		{
			testName: "emoji from",

			from: []byte("http://🖖.com"),
			to:   []byte("https://spock.com"),

			in:  []byte(`s:12:\"http://🖖.com\";`),
			out: []byte(`s:17:\"https://spock.com\";`),
		},
		{
			testName: "emoji to",

			from: []byte("https://spock.com"),
			to:   []byte("http://🖖.com"),

			in:  []byte(`s:17:\"https://spock.com\";`),
			out: []byte(`s:15:\"http://🖖.com\";`),
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			replaced := replaceAndFix(test.in, []*Replacement{
				&Replacement{
					From: test.from,
					To:   test.to,
				},
			})

			if !bytes.Equal(replaced, test.out) {
				t.Error("Expected:", string(test.out), "Actual:", string(replaced))
			}
		})
	}
}

func TestMultiReplace(t *testing.T) {
	var tests = []struct {
		testName     string
		in           []byte
		out          []byte
		replacements []*Replacement
	}{
		{
			testName: "simple test",
			in:       []byte("http://automattic.com"),
			out:      []byte("https://automattic.org"),
			replacements: []*Replacement{
				&Replacement{
					From: []byte("http:"),
					To:   []byte("https:"),
				},
				&Replacement{
					From: []byte("automattic.com"),
					To:   []byte("automattic.org"),
				},
			},
		},
		{
			testName: "overlapping",
			in:       []byte("http://automattic.com"),
			out:      []byte("https://automattic.org"),
			replacements: []*Replacement{
				&Replacement{
					From: []byte("http:"),
					To:   []byte("https:"),
				},
				&Replacement{
					From: []byte("//automattic.com"),
					To:   []byte("//automattic.org"),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			replaced := replaceAndFix(test.in, test.replacements)

			if !bytes.Equal(replaced, test.out) {
				t.Error("Expected:", string(test.out), "Actual:", string(replaced))
			}
		})
	}
}

func TestFix(t *testing.T) {
	var tests = []struct {
		testName string
		from     []byte
		to       []byte
	}{
		{
			testName: "Empty string",
			from:     []byte(`s:10:\"\";`),
			to:       []byte(`s:0:\"\";`),
		},
		{
			testName: "Empty string (corrected)",
			from:     []byte(`s:0:\"\";`),
			to:       []byte(`s:0:\"\";`),
		},
		{
			testName: "Empty string (escaped quotes)",
			from:     []byte(`s:0:\"\";`),
			to:       []byte(`s:0:\"\";`),
		},
		{
			testName: "Line break",
			from:     []byte(`s:0:\"line\\nbreak\";`),
			to:       []byte(`s:11:\"line\\nbreak\";`),
		},
		{
			testName: "Escaped URL",
			from:     []byte(`s:0:\"https:\\/\\/automattic.com\";`),
			to:       []byte(`s:24:\"https:\\/\\/automattic.com\";`),
		},
		{
			testName: "Many escaped characters (including escaped backslash)",
			from:     []byte(`s:0:\"\t\r\n \t\r\n \t\r\n \\ <a href=\"https://example.com\">Many\tescaped\tcharacters</a>\";`),
			to:       []byte(`s:71:\"\t\r\n \t\r\n \t\r\n \\ <a href=\"https://example.com\">Many\tescaped\tcharacters</a>\";`),
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			fixed := fix(test.from)
			if !bytes.Equal(fixed, test.to) {
				t.Error("Expected:", string(test.to), "Actual:", string(fixed))
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

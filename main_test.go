package main

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Automattic/go-search-replace/searchreplace"
)

var (
	_, b, _, _ = runtime.Caller(0)
	basePath   = filepath.Dir(b)
)

func doMainTest(t *testing.T, input string, expected string, mainArgs []string) {
	execArgs := append([]string{"run", basePath}, mainArgs...)
	cmd := exec.Command("go", execArgs...)

	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		t.Errorf("%v", err)
	}
	actual := out.String()

	if actual != expected {
		t.Errorf("%v does not match expected: %v", actual, expected)
	}
}

type failingReader struct {
	value        string
	err          error
	read         bool
	errWithValue bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, r.err
	}

	r.read = true
	n := copy(p, r.value)
	if r.errWithValue {
		return n, r.err
	}
	return n, nil
}

func testReplacements() []*searchreplace.Replacement {
	return []*searchreplace.Replacement{
		{
			From: []byte("http://uss-enterprise.com"),
			To:   []byte("https://ncc-1701-d.space"),
		},
	}
}

func TestRunReturnsNonZeroOnReadError(t *testing.T) {
	readError := errors.New("stdin read failed")
	stdin := &failingReader{
		value: "Check out: http://uss-enterprise.com/decks/10/sections/forward\n",
		err:   readError,
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run(stdin, &out, &errOut, testReplacements())

	if code != 1 {
		t.Errorf("Expected exit code 1, got %d", code)
	}

	expected := "Check out: https://ncc-1701-d.space/decks/10/sections/forward\n"
	if out.String() != expected {
		t.Errorf("%v does not match expected: %v", out.String(), expected)
	}

	if !strings.Contains(errOut.String(), readError.Error()) {
		t.Errorf("Expected stderr to contain %q, got %q", readError.Error(), errOut.String())
	}
}

func TestRunProcessesPartialLineReturnedWithReadError(t *testing.T) {
	readError := errors.New("stdin read failed")
	stdin := &failingReader{
		value:        "Check out: http://uss-enterprise.com/decks/10/sections/forward",
		err:          readError,
		errWithValue: true,
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run(stdin, &out, &errOut, testReplacements())

	if code != 1 {
		t.Errorf("Expected exit code 1, got %d", code)
	}

	expected := "Check out: https://ncc-1701-d.space/decks/10/sections/forward"
	if out.String() != expected {
		t.Errorf("%v does not match expected: %v", out.String(), expected)
	}

	if !strings.Contains(errOut.String(), readError.Error()) {
		t.Errorf("Expected stderr to contain %q, got %q", readError.Error(), errOut.String())
	}
}

func TestRunReturnsZeroAtEOF(t *testing.T) {
	var tests = []struct {
		testName string
		in       string
		out      string
	}{
		{
			testName: "with newline",
			in:       "Check out: http://uss-enterprise.com/decks/10/sections/forward\n",
			out:      "Check out: https://ncc-1701-d.space/decks/10/sections/forward\n",
		},
		{
			testName: "without newline",
			in:       "Check out: http://uss-enterprise.com/decks/10/sections/forward",
			out:      "Check out: https://ncc-1701-d.space/decks/10/sections/forward",
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			var out bytes.Buffer
			var errOut bytes.Buffer
			code := run(strings.NewReader(test.in), &out, &errOut, testReplacements())

			if code != 0 {
				t.Errorf("Expected exit code 0, got %d", code)
			}

			if out.String() != test.out {
				t.Errorf("%v does not match expected: %v", out.String(), test.out)
			}

			if errOut.String() != "" {
				t.Errorf("Expected empty stderr, got %q", errOut.String())
			}
		})
	}
}

func TestSimpleReplaceWithNewlineAtEOF(t *testing.T) {
	mainArgs := []string{
		"http://uss-enterprise.com",
		"https://ncc-1701-d.space",
	}

	input := "Space, the final frontier!\nCheck out: http://uss-enterprise.com/decks/10/sections/forward\n"
	expected := "Space, the final frontier!\nCheck out: https://ncc-1701-d.space/decks/10/sections/forward\n"
	doMainTest(t, input, expected, mainArgs)
}

func TestSimpleReplaceWithoutNewlineAtEOF(t *testing.T) {
	mainArgs := []string{
		"http://uss-enterprise.com",
		"https://ncc-1701-d.space",
	}
	input := "I tend bar, and I listen.\nhttp://uss-enterprise.com/personnel/guinan"
	expected := "I tend bar, and I listen.\nhttps://ncc-1701-d.space/personnel/guinan"
	doMainTest(t, input, expected, mainArgs)
}

func TestMultipleReplaceWithNewlineAtEOF(t *testing.T) {
	mainArgs := []string{
		"http://uss-enterprise.com",
		"https://ncc-1701-d.space",

		"sections",
		"areas",

		"https",
		"warp",
	}
	input := "Space, the final frontier!\nCheck out: http://uss-enterprise.com/decks/10/sections/forward\n"
	expected := "Space, the final frontier!\nCheck out: warp://ncc-1701-d.space/decks/10/areas/forward\n"
	doMainTest(t, input, expected, mainArgs)
}

func TestMultipleReplaceWithoutNewlineAtEOF(t *testing.T) {
	mainArgs := []string{
		"http://uss-enterprise.com",
		"https://ncc-1701-d.space",

		"sections",
		"areas",

		"https",
		"warp",
	}
	input := "Space, the final frontier!\nCheck out: http://uss-enterprise.com/decks/10/sections/forward"
	expected := "Space, the final frontier!\nCheck out: warp://ncc-1701-d.space/decks/10/areas/forward"
	doMainTest(t, input, expected, mainArgs)
}

func TestSerializedReplaceWithCss(t *testing.T) {
	mainArgs := []string{
		"https://uss-enterprise.com",
		"https://ncc-1701-d.space",
	}

	input := `a:2:{s:3:\"key\";s:5:\"value\";s:3:\"css\";s:208:\"body { color: #123456;\r\nborder-bottom: none; }\r\ndiv.bg { background: url('https://uss-enterprise.com/wp-content/uploads/main-bg.gif');\r\n  background-position: left center;\r\n    background-repeat: no-repeat; }\";}`
	expected := `a:2:{s:3:\"key\";s:5:\"value\";s:3:\"css\";s:206:\"body { color: #123456;\r\nborder-bottom: none; }\r\ndiv.bg { background: url('https://ncc-1701-d.space/wp-content/uploads/main-bg.gif');\r\n  background-position: left center;\r\n    background-repeat: no-repeat; }\";}`
	doMainTest(t, input, expected, mainArgs)
}

func TestSerializedReplaceWithCssAndUnrelatedSerializationMarker(t *testing.T) {
	mainArgs := []string{
		"https://uss-enterprise.com",
		"https://ncc-1701-d.space",
	}

	input := `a:2:{s:3:\"key\";s:5:\"value\";s:3:\"css\";s:239:\"body { color: #123456;\r\nborder-bottom: none; }\r\nbody:after{ content: \"▼\"; }\r\ndiv.bg { background: url('https://uss-enterprise.com/wp-content/uploads/main-bg.gif');\r\n  background-position: left center;\r\n    background-repeat: no-repeat; }\";}`
	expected := `a:2:{s:3:\"key\";s:5:\"value\";s:3:\"css\";s:237:\"body { color: #123456;\r\nborder-bottom: none; }\r\nbody:after{ content: \"▼\"; }\r\ndiv.bg { background: url('https://ncc-1701-d.space/wp-content/uploads/main-bg.gif');\r\n  background-position: left center;\r\n    background-repeat: no-repeat; }\";}`
	doMainTest(t, input, expected, mainArgs)
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

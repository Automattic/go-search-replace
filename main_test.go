package main

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/Automattic/go-search-replace/searchreplace"
)

var (
	_, b, _, _ = runtime.Caller(0)
	basePath   = filepath.Dir(b)
)

func doMainTest(t *testing.T, input string, expected string, mainArgs []string) {
	t.Helper()

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

func doMainFailureTest(t *testing.T, input string, mainArgs []string) (string, string) {
	t.Helper()

	execArgs := append([]string{"run", basePath}, mainArgs...)
	cmd := exec.Command("go", execArgs...)

	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err == nil {
		t.Fatal("Expected command to fail")
	}

	return out.String(), stderr.String()
}

func buildMainBinary(t *testing.T) string {
	t.Helper()

	binaryPath := filepath.Join(t.TempDir(), "go-search-replace")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", binaryPath, basePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build CLI: %v\n%s", err, output)
	}

	return binaryPath
}

func doMainFailureExitCodeTest(t *testing.T, binaryPath string, input string, mainArgs []string) (string, string, int) {
	t.Helper()

	cmd := exec.Command(binaryPath, mainArgs...)

	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatal("Expected command to fail")
	}

	exitError, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("Expected exit error, got: %v", err)
	}

	return out.String(), stderr.String(), exitError.ExitCode()
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

func TestValidInput(t *testing.T) {
	var tests = []struct {
		testName string
		in       string
		valid    bool
	}{
		{
			testName: "bare domain",
			in:       "automattic.com",
			valid:    true,
		},
		{
			testName: "domain with port",
			in:       "example.com:8080",
			valid:    true,
		},
		{
			testName: "domain with path",
			in:       "example.com/wp-content",
			valid:    true,
		},
		{
			testName: "localhost with path",
			in:       "localhost/wp-content",
			valid:    true,
		},
		{
			testName: "IPv4 with path",
			in:       "127.0.0.1/wp-content",
			valid:    true,
		},
		{
			testName: "http URL",
			in:       "http://uss-enterprise.com",
			valid:    true,
		},
		{
			testName: "https URL",
			in:       "https://ncc-1701-d.space",
			valid:    true,
		},
		{
			testName: "URL with path",
			in:       "http://old.example.com/wp-content",
			valid:    true,
		},
		{
			testName: "https token",
			in:       "https",
			valid:    true,
		},
		{
			testName: "replacement token",
			in:       "warp",
			valid:    true,
		},
		{
			testName: "path segment token from",
			in:       "sections",
			valid:    true,
		},
		{
			testName: "path segment token to",
			in:       "areas",
			valid:    true,
		},
		{
			testName: "http scheme token",
			in:       "http:",
			valid:    true,
		},
		{
			testName: "https scheme token",
			in:       "https:",
			valid:    true,
		},
		{
			testName: "too long argument",
			in:       strings.Repeat("a", maxArgumentLength+1),
			valid:    false,
		},
		{
			testName: "relative traversal path",
			in:       "../../etc/passwd",
			valid:    false,
		},
		{
			testName: "absolute filesystem path",
			in:       "/tmp/file",
			valid:    false,
		},
		{
			testName: "relative etc path",
			in:       "etc/passwd",
			valid:    false,
		},
		{
			testName: "relative home path",
			in:       "home/user/.ssh/id_rsa",
			valid:    false,
		},
		{
			testName: "relative wp content path",
			in:       "wp-content/uploads",
			valid:    false,
		},
		{
			testName: "dot slash path",
			in:       "./example.com",
			valid:    false,
		},
		{
			testName: "domain path traversal",
			in:       "example.com/../admin",
			valid:    false,
		},
		{
			testName: "URL path traversal",
			in:       "http://example.com/../admin",
			valid:    false,
		},
		{
			testName: "repeated empty path segments",
			in:       "http://example.com//admin",
			valid:    false,
		},
		{
			testName: "unsupported file scheme",
			in:       "file:///etc/passwd",
			valid:    false,
		},
		{
			testName: "javascript scheme",
			in:       "javascript:alert",
			valid:    false,
		},
		{
			testName: "missing URL host",
			in:       "http://",
			valid:    false,
		},
		{
			testName: "malformed URL host",
			in:       "http:///missing-host",
			valid:    false,
		},
		{
			testName: "hostname label starts with hyphen",
			in:       "http://-bad.example.com",
			valid:    false,
		},
		{
			testName: "hostname label ends with hyphen",
			in:       "http://bad-.example.com",
			valid:    false,
		},
		{
			testName: "missing scheme",
			in:       "://example.com",
			valid:    false,
		},
		{
			testName: "non-numeric port",
			in:       "example.com:abc",
			valid:    false,
		},
		{
			testName: "out of range port",
			in:       "example.com:99999",
			valid:    false,
		},
		{
			testName: "array serialization marker",
			in:       "a:4:",
			valid:    false,
		},
		{
			testName: "string serialization marker",
			in:       "s:1:",
			valid:    false,
		},
		{
			testName: "SQL string",
			in:       "),(",
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

func TestValidateReplacementArgs(t *testing.T) {
	tests := []struct {
		testName string
		args     []string
		wantErr  string
	}{
		{
			testName: "valid multiple pairs",
			args: []string{
				"http://uss-enterprise.com",
				"https://ncc-1701-d.space",
				"sections",
				"areas",
				"https",
				"warp",
			},
		},
		{
			testName: "valid replacement containing from",
			args:     []string{"http", "https"},
		},
		{
			testName: "valid ordered shrinking chain",
			args:     []string{"aaaa", "bbbb", "bbbb", "ccc"},
		},
		{
			testName: "valid ordered replacement chain with one expansion",
			args:     []string{"aaaa", "bbbbbbbb", "bbbb", "cccc"},
		},
		{
			testName: "no-op replacement",
			args:     []string{"automattic.com", "automattic.com"},
			wantErr:  "must be different",
		},
		{
			testName: "duplicate from",
			args:     []string{"automattic.com", "example.com", "automattic.com", "example.org"},
			wantErr:  "duplicate <from>",
		},
		{
			testName: "max length",
			args:     []string{strings.Repeat("a", maxArgumentLength+1), "example.com"},
			wantErr:  "maximum length",
		},
		{
			testName: "expansion factor",
			args:     []string{"abcd", strings.Repeat("a", len("abcd")*maxExpansionFactor+1)},
			wantErr:  "expands <from>",
		},
		{
			testName: "multiple expanding pairs",
			args:     []string{"aaaa", "bbbbbbbb", "bbbb", strings.Repeat("c", len("bbbb")*maxExpansionFactor)},
			wantErr:  "only one expanding replacement pair",
		},
		{
			testName: "multi-prior-output ordered expansion bypass",
			args: []string{
				"aaaa",
				"bbbbb",
				"cccc",
				"ddddd",
				"bbbbbddddd",
				strings.Repeat("e", len("bbbbbddddd")*maxExpansionFactor),
			},
			wantErr: "only one expanding replacement pair",
		},
		{
			testName: "multiple expanding pairs assembled by intervening replacement",
			args: []string{
				"aaaa",
				"bbbbcccc",
				"bbbb",
				"dddd",
				"ddddcccc",
				strings.Repeat("e", len("ddddcccc")*maxExpansionFactor),
			},
			wantErr: "only one expanding replacement pair",
		},
		{
			testName: "multiple expanding pairs assembled across replacement boundary",
			args: []string{
				"aaaa",
				"bbbbbbbb",
				"bbbbbbbbc",
				strings.Repeat("e", len("bbbbbbbbc")*maxExpansionFactor),
			},
			wantErr: "only one expanding replacement pair",
		},
		{
			testName: "multiple expanding pairs assembled across two-sided replacement boundary",
			args: []string{
				"xxxx",
				"bbbbbbbb",
				"abbbbbbbbc",
				strings.Repeat("e", len("abbbbbbbbc")*maxExpansionFactor),
			},
			wantErr: "only one expanding replacement pair",
		},
		{
			testName: "multiple expanding pairs assembled across aggregate replacement boundaries",
			args: []string{
				"aaaaaaaaaa",
				"bbbbbbcccccc",
				"ccccccddddddddbbbbbb",
				strings.Repeat("e", len("ccccccddddddddbbbbbb")*maxExpansionFactor),
			},
			wantErr: "only one expanding replacement pair",
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			err := validateReplacementArgs(test.args)
			if test.wantErr == "" {
				if err != nil {
					t.Fatal("Expected no error, got:", err)
				}
				return
			}

			if err == nil {
				t.Fatal("Expected error containing:", test.wantErr)
			}

			if !strings.Contains(err.Error(), test.wantErr) {
				t.Error("Expected error containing:", test.wantErr, "Actual:", err)
			}
		})
	}
}

func TestValidationExitCodes(t *testing.T) {
	binaryPath := buildMainBinary(t)

	tests := []struct {
		testName string
		args     []string
		wantCode int
		wantErr  string
	}{
		{
			testName: "odd replacement count",
			args:     []string{"automattic.com", "example.com", "orphaned"},
			wantCode: exitUsage,
			wantErr:  "All replacements must have a <from> and <to> value",
		},
		{
			testName: "invalid from",
			args:     []string{"bad", "example.com"},
			wantCode: exitInvalidFrom,
			wantErr:  "Invalid <from>",
		},
		{
			testName: "invalid to",
			args:     []string{"automattic.com", "x"},
			wantCode: exitInvalidTo,
			wantErr:  "Invalid <to>",
		},
		{
			testName: "multiple expanding pairs",
			args:     []string{"aaaa", "bbbbb", "cccc", "ddddd"},
			wantCode: exitInvalidTo,
			wantErr:  "only one expanding replacement pair",
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			stdout, stderr, exitCode := doMainFailureExitCodeTest(t, binaryPath, "", test.args)

			if stdout != "" {
				t.Error("Expected no stdout, got:", stdout)
			}

			if exitCode != test.wantCode {
				t.Error("Expected exit code:", test.wantCode, "Actual:", exitCode)
			}

			if !strings.Contains(stderr, test.wantErr) {
				t.Error("Expected stderr containing:", test.wantErr, "Actual:", stderr)
			}
		})
	}
}

func TestTooManyReplacementPairs(t *testing.T) {
	args := make([]string, 0, (maxReplacementPairs+1)*2)
	for i := 0; i <= maxReplacementPairs; i++ {
		args = append(args, "from"+strconv.Itoa(i), "to"+strconv.Itoa(i))
	}

	err := validateReplacementArgs(args)
	if err == nil {
		t.Fatal("Expected too many replacement pairs to fail")
	}

	if !strings.Contains(err.Error(), "Too many replacement pairs") {
		t.Error("Expected too many pairs error, got:", err)
	}
}

func TestInvalidArgsFailBeforeProcessing(t *testing.T) {
	stdout, stderr := doMainFailureTest(t, "http://uss-enterprise.com\n", []string{
		"http://",
		"https://ncc-1701-d.space",
	})

	if stdout != "" {
		t.Error("Expected no stdout, got:", stdout)
	}

	if !strings.Contains(stderr, "Invalid <from>") {
		t.Error("Expected invalid <from> error, got:", stderr)
	}
}

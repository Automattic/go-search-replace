package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

	input := `a:2:{s:3:\"key\";s:5:\"value\";s:3:\"css\";s:216:\"body { color: #123456;\r\nborder-bottom: none; }\r\ndiv.bg { background: url('https://uss-enterprise.com/wp-content/uploads/main-bg.gif');\r\n  background-position: left center;\r\n    background-repeat: no-repeat; }\";}`
	expected := `a:2:{s:3:\"key\";s:5:\"value\";s:3:\"css\";s:214:\"body { color: #123456;\r\nborder-bottom: none; }\r\ndiv.bg { background: url('https://ncc-1701-d.space/wp-content/uploads/main-bg.gif');\r\n  background-position: left center;\r\n    background-repeat: no-repeat; }\";}`
	doMainTest(t, input, expected, mainArgs)
}

func TestSerializedReplaceWithCssAndUnrelatedSerializationMarker(t *testing.T) {
	mainArgs := []string{
		"https://uss-enterprise.com",
		"https://ncc-1701-d.space",
	}

	input := `a:2:{s:3:\"key\";s:5:\"value\";s:3:\"css\";s:249:\"body { color: #123456;\r\nborder-bottom: none; }\r\nbody:after{ content: \"▼\"; }\r\ndiv.bg { background: url('https://uss-enterprise.com/wp-content/uploads/main-bg.gif');\r\n  background-position: left center;\r\n    background-repeat: no-repeat; }\";}`
	expected := `a:2:{s:3:\"key\";s:5:\"value\";s:3:\"css\";s:247:\"body { color: #123456;\r\nborder-bottom: none; }\r\nbody:after{ content: \"▼\"; }\r\ndiv.bg { background: url('https://ncc-1701-d.space/wp-content/uploads/main-bg.gif');\r\n  background-position: left center;\r\n    background-repeat: no-repeat; }\";}`
	doMainTest(t, input, expected, mainArgs)
}

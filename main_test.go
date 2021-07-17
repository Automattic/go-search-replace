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
	basepath   = filepath.Dir(b)
)

func doMainTest(t *testing.T, from []string, to []string, input string, expected string) {
	// TODO: Support multiple from-to pairs
	cmd := exec.Command("go", "run", basepath, from[0], to[0])

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
	from := []string{"uss-enterprise.com"}
	to := []string{"ncc-1701-d.space"}
	input := "Space, the final frontier!\nCheck out: https://uss-enterprise.com/decks/10/sections/forward\n"
	expected := "Space, the final frontier!\nCheck out: https://ncc-1701-d.space/decks/10/sections/forward\n"
	doMainTest(t, from, to, input, expected)
}

func TestSimpleReplaceWithoutNewlineAtEOF(t *testing.T) {
	from := []string{"uss-enterprise.com"}
	to := []string{"ncc-1701-d.space"}
	input := "I tend bar, and I listen.\nhttps://uss-enterprise.com/personnel/guinan"
	expected := "I tend bar, and I listen.\nhttps://ncc-1701-d.space/personnel/guinan"
	doMainTest(t, from, to, input, expected)
}

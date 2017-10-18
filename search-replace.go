package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	searchRe  = `s:\d+:\\?\".*?\\?\";`
	replaceRe = `(?:s:)(?:\d+:)(\\?\")(.*?)(\\?\";)`
	inputRe   = `^[A-Za-z0-9\.:/]+$`
)

var (
	search  = regexp.MustCompile(searchRe)
	replace = regexp.MustCompile(replaceRe)
	input   = regexp.MustCompile(inputRe)

	from string
	to   string

	flagFix bool
)

func init() {
	flag.BoolVar(&flagFix, "fix-only", false, "Fix all serialized strings")
	flag.Parse()

	if flagFix {
		return
	}

	if len(flag.Args()) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: search-replace <from> <to>")
		os.Exit(1)
	}

	from = flag.Arg(0)
	if !input.MatchString(from) {
		fmt.Fprintln(os.Stderr, "Invalid from URL")
		os.Exit(2)
	}

	to = flag.Arg(1)
	if !input.MatchString(to) {
		fmt.Fprintln(os.Stderr, "Invalid to URL")
		os.Exit(3)
	}
}

func main() {
	r := bufio.NewReaderSize(os.Stdin, 2*1024*1024)
	for {
		line, err := r.ReadString('\n')

		if err == io.EOF {
			break
		}

		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			break
		}

		if flagFix {
			line = search.ReplaceAllStringFunc(line, fix)
		} else {
			line = replaceAndFix(line, from, to)
		}

		fmt.Print(line)
	}
}

func replaceAndFix(line, from, to string) string {
	if !strings.Contains(line, from) {
		return line
	}

	// Find/replace from->to
	line = strings.Replace(line, from, to, -1)

	// Fix serialized string lengths
	line = search.ReplaceAllStringFunc(line, func(match string) string {
		// Skip fixing if we didn't replace anything
		if !strings.Contains(match, to) {
			return match
		}

		return fix(match)
	})

	return line
}

func fix(match string) string {
	parts := replace.FindStringSubmatch(match)

	// Get string length - number of escaped characters and avoid double counting escaped \
	length := len(parts[2]) - (strings.Count(parts[2], `\`) - strings.Count(parts[2], `\\`))
	return fmt.Sprintf("s:%d:%s%s%s", length, parts[1], parts[2], parts[3])
}

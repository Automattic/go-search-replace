package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
)

const (
	searchRe  = `s:\d+:\\?\".*?\\?\";`
	replaceRe = `(?:s:)(?:\d+:)(\\?\")(.*?)(\\?\";)`

	inputRe    = `^[A-Za-z0-9\-\.:/]{4,}$`
	badInputRe = `\w:\d+:`
)

var (
	search  = regexp.MustCompile(searchRe)
	replace = regexp.MustCompile(replaceRe)
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: search-replace <from> <to>")
		os.Exit(1)
		return
	}

	from := os.Args[1]
	if !validInput(from) {
		fmt.Fprintln(os.Stderr, "Invalid from URL")
		os.Exit(2)
		return
	}

	to := os.Args[2]
	if !validInput(to) {
		fmt.Fprintln(os.Stderr, "Invalid to URL")
		os.Exit(3)
		return
	}

	var wg sync.WaitGroup
	lines := make(chan chan string, 10)

	wg.Add(1)
	go func() {
		defer wg.Done()

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

			wg.Add(1)
			ch := make(chan string)
			lines <- ch

			go func(line string) {
				defer wg.Done()
				line = replaceAndFix(line, from, to)
				ch <- line
			}(line)
		}
	}()

	go func() {
		wg.Wait()
		close(lines)
	}()

	for line := range lines {
		fmt.Print(<-line)
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

func validInput(in string) bool {
	input := regexp.MustCompile(inputRe)
	if !input.MatchString(in) {
		return false
	}

	bad := regexp.MustCompile(badInputRe)
	if bad.MatchString(in) {
		return false
	}

	return true
}

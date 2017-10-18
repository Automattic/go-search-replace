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
	inputRe   = `^[A-Za-z0-9\.:/]+$`
)

var (
	search  = regexp.MustCompile(searchRe)
	replace = regexp.MustCompile(replaceRe)
	input   = regexp.MustCompile(inputRe)
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: search-replace <from> <to>")
		return
	}

	from := os.Args[1]
	if !input.MatchString(from) {
		fmt.Fprintln(os.Stderr, "Invalid from URL")
		return
	}

	to := os.Args[2]
	if !input.MatchString(to) {
		fmt.Fprintln(os.Stderr, "Invalid to URL")
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	lines := make(chan chan string, 10)

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

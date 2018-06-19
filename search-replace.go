package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
)

const (
	searchRe  = `s:\d+:\\\".*?\\\";`
	replaceRe = `s:\d+:\\\"(.*?)\\\";`

	badInputRe   = `\w:\d+:`
	inputRe      = `^[A-Za-z0-9_\-\.:/]+$`
	minInLength  = 4
	minOutLength = 2

	version = "0.0.6-dev"
)

var (
	search  = regexp.MustCompile(searchRe)
	replace = regexp.MustCompile(replaceRe)
)

func main() {
	versionFlag := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("go-search-replace version %s\n", version)
		os.Exit(0)
		return
	}

	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: search-replace <from> <to>")
		os.Exit(1)
		return
	}

	replacements := make(map[string]string)
	args := os.Args[1:]

	if len(args)%2 > 0 {
		fmt.Fprintln(os.Stderr, "All replacements must have a <from> and <to> value")
		os.Exit(1)
		return
	}

	var from, to string
	for i := 0; i < len(args)/2; i++ {
		from = args[i*2]
		if !validInput(from, minInLength) {
			fmt.Fprintln(os.Stderr, "Invalid <from> URL, minimum length is 4")
			os.Exit(2)
			return
		}

		to = args[(i*2)+1]
		if !validInput(to, minOutLength) {
			fmt.Fprintln(os.Stderr, "Invalid <to>, minimum length is 2")
			os.Exit(3)
			return
		}

		replacements[from] = to
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
				line = replaceAndFix(line, replacements)
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

func replaceAndFix(line string, replacements map[string]string) string {
	for from, to := range replacements {
		if !strings.Contains(line, from) {
			continue
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
	}

	return line
}

func fix(match string) string {
	parts := replace.FindStringSubmatch(match)

	if len(parts) != 2 {
		// This looks wrong, don't touch anything
		return match
	}

	// Get string length - number of escaped characters and avoid double counting escaped \
	length := len(parts[1]) - (strings.Count(parts[1], `\`) - strings.Count(parts[1], `\\`))
	return fmt.Sprintf("s:%d:\\\"%s\\\";", length, parts[1])
}

func validInput(in string, length int) bool {
	if len(in) < length {
		return false
	}

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

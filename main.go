package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"unsafe"

	"github.com/Automattic/go-search-replace/searchreplace"
)

const (
	badInputRe   = `\w:\d+:`
	inputRe      = `^[A-Za-z0-9_\-\.:/]+$`
	minInLength  = 4
	minOutLength = 2

	version = "0.0.9-dev"
)

var (
	input = regexp.MustCompile(inputRe)
	bad   = regexp.MustCompile(badInputRe)
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

	var replacements []*searchreplace.Replacement
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

		replacements = append(replacements, &searchreplace.Replacement{
			From: []byte(from),
			To:   []byte(to),
		})
	}

	var wg sync.WaitGroup
	lines := make(chan chan []byte, 10)

	wg.Add(1)
	go func() {
		defer wg.Done()

		r := bufio.NewReaderSize(os.Stdin, 2*1024*1024)
		for {
			line, err := r.ReadBytes('\n')

			if err != nil {
				if err == io.EOF {
					if 0 == len(line) {
						break
					}
				} else {
					fmt.Fprintln(os.Stderr, err.Error())
					break
				}
			}

			wg.Add(1)
			ch := make(chan []byte)
			lines <- ch

			go func(line *[]byte) {
				defer wg.Done()
				line = searchreplace.FixLine(line, replacements)
				ch <- *line
			}(&line)
		}
	}()

	go func() {
		wg.Wait()
		close(lines)
	}()

	for line := range lines {
		fmt.Print(unsafeGetString(<-line))
	}
}

func validInput(in string, length int) bool {
	if len(in) < length {
		return false
	}

	if !input.MatchString(in) {
		return false
	}

	if bad.MatchString(in) {
		return false
	}

	return true
}

func unsafeGetString(bs []byte) string {
	return *(*string)(unsafe.Pointer(&bs))
}

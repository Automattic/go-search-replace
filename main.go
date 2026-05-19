package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"

	"github.com/Automattic/go-search-replace/searchreplace"
)

const (
	badInputRe   = `\w:\d+:`
	inputRe      = `^[A-Za-z0-9_\-\.:/]+$`
	minInLength  = 4
	minOutLength = 2

	version = "0.0.11"
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

	if code := run(os.Stdin, os.Stdout, os.Stderr, replacements); code != 0 {
		os.Exit(code)
		return
	}
}

func run(input io.Reader, output io.Writer, errorOutput io.Writer, replacements []*searchreplace.Replacement) int {
	if err := process(input, output, errorOutput, replacements); err != nil {
		return 1
	}

	return 0
}

func process(input io.Reader, output io.Writer, errorOutput io.Writer, replacements []*searchreplace.Replacement) error {
	var wg sync.WaitGroup
	lines := make(chan chan []byte, 10)
	readErrors := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()

		bufferSize := 16 * 1024 * 1024 // 16 MB
		r := bufio.NewReaderSize(input, bufferSize)
		for {
			line, err := r.ReadBytes('\n')

			if len(line) > 0 {
				wg.Add(1)
				ch := make(chan []byte)
				lines <- ch

				go func(line *[]byte) {
					defer wg.Done()
					line = searchreplace.FixLine(line, replacements)
					ch <- *line
				}(&line)
			}

			if err != nil {
				if err != io.EOF {
					fmt.Fprintln(errorOutput, err.Error())
					readErrors <- err
				}
				break
			}
		}
	}()

	go func() {
		wg.Wait()
		close(lines)
		close(readErrors)
	}()

	for line := range lines {
		outLine := <-line
		written, err := output.Write(outLine)
		if err != nil {
			fmt.Fprintln(errorOutput, err.Error())
			return err
		}
		if written != len(outLine) {
			fmt.Fprintln(errorOutput, io.ErrShortWrite.Error())
			return io.ErrShortWrite
		}
	}

	if err, ok := <-readErrors; ok {
		return err
	}

	return nil
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

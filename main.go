package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/Automattic/go-search-replace/searchreplace"
)

const (
	badInputRe          = `\w:\d+:`
	inputRe             = `^[A-Za-z0-9_\-\.:/]+$`
	safePartRe          = `^[A-Za-z0-9_.-]+$`
	minInLength         = 4
	minOutLength        = 2
	maxArgumentLength   = 2048
	maxReplacementPairs = 64
	maxExpansionFactor  = 16
	exitUsage           = 1
	exitInvalidFrom     = 2
	exitInvalidTo       = 3

	version = "0.0.11"
)

var (
	input    = regexp.MustCompile(inputRe)
	bad      = regexp.MustCompile(badInputRe)
	safePart = regexp.MustCompile(safePartRe)
)

type replacementArgError struct {
	message  string
	exitCode int
}

type replacementPair struct {
	from   string
	to     string
	number int
}

func (err replacementArgError) Error() string {
	return err.message
}

func (err replacementArgError) ExitCode() int {
	return err.exitCode
}

func newReplacementArgError(exitCode int, format string, args ...interface{}) error {
	return replacementArgError{
		message:  fmt.Sprintf(format, args...),
		exitCode: exitCode,
	}
}

func replacementArgExitCode(err error) int {
	if errWithExitCode, ok := err.(interface{ ExitCode() int }); ok {
		return errWithExitCode.ExitCode()
	}

	return exitInvalidFrom
}

func main() {
	versionFlag := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("go-search-replace version %s\n", version)
		os.Exit(0)
		return
	}

	args := flag.Args()

	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: search-replace <from> <to> [<from> <to> ...]")
		os.Exit(exitUsage)
		return
	}

	if err := validateReplacementArgs(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(replacementArgExitCode(err))
		return
	}

	replacements := make([]*searchreplace.Replacement, 0, len(args)/2)
	for i := 0; i < len(args)/2; i++ {
		from := args[i*2]
		to := args[(i*2)+1]

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
		fmt.Fprint(output, unsafeGetString(<-line))
	}

	if err, ok := <-readErrors; ok {
		return err
	}

	return nil
}

func validInput(in string, length int) bool {
	return validateInput(in, length) == nil
}

func validateReplacementArgs(args []string) error {
	if len(args)%2 > 0 {
		return newReplacementArgError(exitUsage, "All replacements must have a <from> and <to> value")
	}

	if len(args)/2 > maxReplacementPairs {
		return newReplacementArgError(exitUsage, "Too many replacement pairs: maximum is %d", maxReplacementPairs)
	}

	seenFrom := map[string]bool{}
	expandingPairSeen := false
	pairs := make([]replacementPair, 0, len(args)/2)

	for i := 0; i < len(args)/2; i++ {
		pairNumber := i + 1
		from := args[i*2]
		to := args[(i*2)+1]

		if err := validateInput(from, minInLength); err != nil {
			return newReplacementArgError(exitInvalidFrom, "Invalid <from> at pair %d: %v", pairNumber, err)
		}

		if seenFrom[from] {
			return newReplacementArgError(exitInvalidFrom, "Invalid <from> at pair %d: duplicate <from> value", pairNumber)
		}
		seenFrom[from] = true

		if err := validateInput(to, minOutLength); err != nil {
			return newReplacementArgError(exitInvalidTo, "Invalid <to> at pair %d: %v", pairNumber, err)
		}

		if from == to {
			return newReplacementArgError(exitInvalidFrom, "Invalid replacement at pair %d: <from> and <to> must be different", pairNumber)
		}

		if len(to) > len(from)*maxExpansionFactor {
			return newReplacementArgError(exitInvalidTo, "Invalid <to> at pair %d: replacement expands <from> by more than %dx", pairNumber, maxExpansionFactor)
		}

		if len(to) > len(from) {
			if expandingPairSeen {
				return newReplacementArgError(exitInvalidTo, "Invalid <to> at pair %d: only one expanding replacement pair is allowed per invocation", pairNumber)
			}
			expandingPairSeen = true
		}

		pairs = append(pairs, replacementPair{
			from:   from,
			to:     to,
			number: pairNumber,
		})
	}

	if err := validateOrderedExpansionChains(pairs); err != nil {
		return err
	}

	return nil
}

func validateOrderedExpansionChains(pairs []replacementPair) error {
	for earlierIndex := 0; earlierIndex < len(pairs); earlierIndex++ {
		earlier := pairs[earlierIndex]
		current := earlier.to
		limit := len(earlier.from) * maxExpansionFactor

		for laterIndex := earlierIndex + 1; laterIndex < len(pairs); laterIndex++ {
			later := pairs[laterIndex]

			if len(later.to) > len(later.from) {
				if err := validateBoundaryExpansionChain(earlier, current, later); err != nil {
					return err
				}
			}

			updated, ok := replaceAllWithLimit(current, later.from, later.to, limit)
			if !ok {
				return newReplacementArgError(
					exitInvalidTo,
					"Invalid ordered expansion chain between pairs %d and %d: cumulative replacement expansion exceeds %dx",
					earlier.number,
					later.number,
					maxExpansionFactor,
				)
			}

			current = updated
		}
	}

	return nil
}

func replaceAllWithLimit(in string, from string, to string, limit int) (string, bool) {
	count := strings.Count(in, from)
	if count == 0 {
		return in, true
	}

	updatedLength := len(in) + count*(len(to)-len(from))
	if updatedLength > limit {
		return "", false
	}

	return strings.ReplaceAll(in, from, to), true
}

func validateBoundaryExpansionChain(earlier replacementPair, current string, later replacementPair) error {
	overlapCount := aggregateBoundaryOverlapCount(current, later.from)
	if overlapCount == 0 {
		return nil
	}

	matchCount := (overlapCount + len(later.from) - 1) / len(later.from)
	originalLength := len(earlier.from) + matchCount*len(later.from) - overlapCount
	updatedLength := len(current) - overlapCount + matchCount*len(later.to)

	if updatedLength > originalLength*maxExpansionFactor {
		return newBoundaryExpansionChainError(earlier, later)
	}

	return nil
}

func aggregateBoundaryOverlapCount(current string, laterFrom string) int {
	var bytesInLaterFrom [256]bool
	for index := 0; index < len(laterFrom); index++ {
		bytesInLaterFrom[laterFrom[index]] = true
	}

	count := 0
	for index := 0; index < len(current); index++ {
		if bytesInLaterFrom[current[index]] {
			count++
		}
	}

	return count
}

func newBoundaryExpansionChainError(earlier replacementPair, later replacementPair) error {
	return newReplacementArgError(
		exitInvalidTo,
		"Invalid ordered expansion chain between pairs %d and %d: later expanding <from> can be assembled across a replacement boundary",
		earlier.number,
		later.number,
	)
}

func validateInput(in string, length int) error {
	if len(in) < length {
		return fmt.Errorf("minimum length is %d", length)
	}

	if len(in) > maxArgumentLength {
		return fmt.Errorf("maximum length is %d bytes", maxArgumentLength)
	}

	if !input.MatchString(in) {
		return fmt.Errorf("contains unsupported characters")
	}

	if bad.MatchString(in) {
		return fmt.Errorf("looks like serialized data")
	}

	if strings.Contains(in, "://") {
		return validateFullURL(in)
	}

	return validateBareInput(in)
}

func validateFullURL(in string) error {
	parsed, err := url.Parse(in)
	if err != nil {
		return fmt.Errorf("malformed URL")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme")
	}

	if parsed.Host == "" {
		return fmt.Errorf("URL requires a host")
	}

	if parsed.User != nil {
		return fmt.Errorf("URL must not include userinfo")
	}

	if parsed.RawQuery != "" || parsed.ForceQuery {
		return fmt.Errorf("URL must not include a query")
	}

	if parsed.Fragment != "" {
		return fmt.Errorf("URL must not include a fragment")
	}

	if err := validateHostPort(parsed.Host); err != nil {
		return err
	}

	if err := validatePath(parsed.Path); err != nil {
		return err
	}

	return nil
}

func validateBareInput(in string) error {
	if in == "http:" || in == "https:" {
		return nil
	}

	if strings.HasPrefix(in, "/") || strings.HasPrefix(in, "./") || strings.HasPrefix(in, "../") {
		return fmt.Errorf("must not look like a filesystem path")
	}

	if strings.Contains(in, "/") {
		host, path, _ := strings.Cut(in, "/")
		if err := validateHostPort(host); err != nil {
			return err
		}

		return validatePath("/" + path)
	}

	if strings.Contains(in, ":") {
		return validateHostPort(in)
	}

	if !safePart.MatchString(in) {
		return fmt.Errorf("contains unsupported characters")
	}

	return nil
}

func validateHostPort(in string) error {
	host := in
	if strings.Contains(in, ":") {
		if strings.Count(in, ":") > 1 {
			return fmt.Errorf("malformed port")
		}

		portIndex := strings.LastIndex(in, ":")
		host = in[:portIndex]
		port := in[portIndex+1:]
		if err := validatePort(port); err != nil {
			return err
		}
	}

	return validateHost(host)
}

func validateHost(in string) error {
	if in == "" {
		return fmt.Errorf("host is required")
	}

	if len(in) > 253 {
		return fmt.Errorf("host exceeds maximum length")
	}

	if strings.EqualFold(in, "localhost") {
		return nil
	}

	if ip := net.ParseIP(in); ip != nil && ip.To4() != nil {
		return nil
	}

	labels := strings.Split(in, ".")
	if len(labels) < 2 {
		return fmt.Errorf("host must be a domain, localhost, or IPv4 address")
	}

	for _, label := range labels {
		if label == "" {
			return fmt.Errorf("host contains an empty label")
		}

		if len(label) > 63 {
			return fmt.Errorf("host label exceeds maximum length")
		}

		if !isAlphaNumeric(label[0]) || !isAlphaNumeric(label[len(label)-1]) {
			return fmt.Errorf("host labels must start and end with an alphanumeric character")
		}

		for i := 0; i < len(label); i++ {
			if !isAlphaNumeric(label[i]) && label[i] != '-' {
				return fmt.Errorf("host labels may only contain alphanumeric characters or hyphens")
			}
		}
	}

	return nil
}

func isAlphaNumeric(char byte) bool {
	return (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')
}

func validatePort(in string) error {
	if in == "" {
		return fmt.Errorf("port is required")
	}

	for _, char := range in {
		if char < '0' || char > '9' {
			return fmt.Errorf("port must be numeric")
		}
	}

	port, err := strconv.Atoi(in)
	if err != nil || port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535")
	}

	return nil
}

func validatePath(in string) error {
	if in == "" {
		return nil
	}

	if !strings.HasPrefix(in, "/") {
		return fmt.Errorf("path must start with a slash")
	}

	if strings.Contains(in, "//") {
		return fmt.Errorf("path must not contain empty segments")
	}

	trimmedPath := strings.Trim(in, "/")
	if trimmedPath == "" {
		return nil
	}

	for _, segment := range strings.Split(trimmedPath, "/") {
		if segment == "." || segment == ".." {
			return fmt.Errorf("path must not contain traversal segments")
		}

		if !safePart.MatchString(segment) {
			return fmt.Errorf("path contains unsupported characters")
		}
	}

	return nil
}

func unsafeGetString(bs []byte) string {
	return *(*string)(unsafe.Pointer(&bs))
}

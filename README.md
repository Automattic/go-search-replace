# Go Search Replace

Search & replace URLs in WordPress SQL files.

```
cat example-from.com.sql | search-replace example-from.com example-to.com > example-to.com.sql
```

## Overview

Migrating WordPress databases often requires replacing domain names. This is a
complex operation because WordPress stores PHP serialized data, which encodes
string lengths. The common method uses PHP to unserialize the data, do the
search/replace, and then re-serialize the data before writing it back to the
database. Here we replace strings in the SQL file and then fix the string
lengths.

## Considerations

Replacing strings in a SQL file can be dangerous. We have to be careful not to
modify the structure of the file in a way that would corrupt the file. For this
reason, we're limiting the search domain to roughly include characters that can
be used in domain names. Since the most common usage for search-replace is
changing domain names or switching http: to https:, this is an easy way to avoid
otherwise complex issues.

## Installation

### From Official Releases

To install on macOS:

```
wget https://github.com/Automattic/go-search-replace/releases/latest/download/go-search-replace_darwin_arm64.gz
gunzip go-search-replace_darwin_arm64.gz
chmod +x go-search-replace_darwin_arm64
mv go-search-replace_darwin_arm64 /usr/local/bin/go-search-replace
go-search-replace --version
```

### From Source

To install from source, this package requires [Go](https://golang.org/).

Note the changes you need to make to your PATH and that you have to either restart your terminal or `source` your shell rc file.

You need to install Gox which you can install with
`go install github.com/mitchellh/gox@latest`

Once that's installed you can install this tool with the following command:
`go install github.com/Automattic/go-search-replace@latest`

Go is set up by convention, not configuration so your files likely live in a directory like: /Users/user/go/src/github.com/Automattic/go-search-replace

Nagivage to that directory and run
`make`

`go-search-replace` will be ready for you to use. Once built you won't have to complete any of the above steps again.

### Container Usage

To build and run the container image locally:

```
docker build -t go-search-replace .
cat example-from.com.sql | docker run --rm -i --read-only go-search-replace example-from.com example-to.com > example-to.com.sql
```

The image runs as a numeric non-root user. The read-only filesystem setting is
enforced by the container runtime or orchestrator, not by the Dockerfile, so pass
`--read-only` to `docker run` or set the equivalent option in your deployment
configuration.
The Dockerfile healthcheck runs `go-search-replace --version` as a binary
self-check for this stdin/stdout CLI; it is not a workload health probe.

## Package Usage

To use it as a package in your Go project, you can install it with the following command:

```
go get github.com/Automattic/go-search-replace
```

Once you've installed the package, you can use the `searchreplace` package in your project. Here's an example:

```go
package main

import (
	"fmt"
	"github.com/Automattic/go-search-replace/searchreplace"
)

func main() {
	input := []byte(`s:3:\"foo\";`)

	result := searchreplace.FixLine(&input, []*searchreplace.Replacement{
		{
			From: []byte("foo"),
			To:   []byte("Hello, Gophers!"),
		},
	})

	fmt.Println(string(*result))
}
```

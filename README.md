# Go Search Replace

[![Build Status](https://travis-ci.org/Automattic/go-search-replace.svg?branch=master)](https://travis-ci.org/Automattic/go-search-replace)

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

This package requires [Go](https://golang.org/). An easy way to install Go on a Mac is with [Homebrew](https://medium.com/@jimkang/install-go-on-mac-with-homebrew-5fa421fc55f5).

Note the changes you need to make to your PATH and that you have to either restart your terminal or `source` your shell rc file.

You need to install Gox which you can install with
`go install github.com/mitchellh/gox@latest`

Once that's installed you can install this tool with the following command:
`go install github.com/Automattic/go-search-replace@latest`

Go is set up by convention, not configuration so your files likely live in a directory like: /Users/user/go/src/github.com/Automattic/go-search-replace

Nagivage to that directory and run
`make`

`go-search-replace` will be ready for you to use. Once built you won't have to complete any of the above steps again.

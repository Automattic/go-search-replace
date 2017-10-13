# Go Search Replace

[![Build Status](https://travis-ci.com/Automattic/go-search-replace.svg?token=xWx9qCRAJeRdHxEcWW83&branch=master)](https://travis-ci.com/Automattic/go-search-replace)

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

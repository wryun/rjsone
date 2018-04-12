# rjsone

rjsone (Render JSON-e) is a simple wrapper around the
[JSON-e templating language](https://taskcluster.github.io/json-e/).

    Usage: ./rjsone [options] [key:]contextfile [[key:]contextfile ...]
      -i int
            indentation level of JSON output; 0 means no pretty-printing (default 2)
      -t string
            file to use for template (default is -, which is stdin) (default "-")
      -y    output YAML rather than JSON (always reads YAML/JSON)

## Getting it

No builds yet. Just:

    go get github.com/wryun/rjsone

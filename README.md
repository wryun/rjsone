[![Build Status](https://travis-ci.org/wryun/rjsone.svg?branch=master)](https://travis-ci.org/wryun/rjsone)

`rjsone` (Render JSON-e) is a simple wrapper around the
[JSON-e templating language](https://taskcluster.github.io/json-e/).

It's a safe and easy way to have templates of moderate complexity
for configuration as code 'languages' like Kubernetes and CloudFormation.

    Usage: rjsone [options] [context ...]
      -d    performs a deep merge of contexts
      -i int
            indentation of JSON output; 0 means no pretty-printing (default 2)
      -o string
            output to a file (default is -, which is stdout) (default "-")
      -t string
            file to use for template (- is stdin) (default "-")
      -v    show information about processing on stderr
      -y    output YAML rather than JSON (always reads YAML/JSON)

Context is usually provided by a list of arguments. By default,
these are interpreted as files. Data is loaded as YAML/JSON by default
and merged into the main context. If the 'filename' begins with a `+`,
the rest of the argument is interpreted as a raw string rather than
reading the file. For example:

    rjsone -t template.yaml context.yaml '+{"foo": 1}'

When duplicate keys are found, later entries replace earlier at the
top level only unless the `-d` flag is passed to perform deep merging.

You can specify a particular context key to load a YAML/JSON file into
using `keyname:filename.yaml`. You can also use `keyname:..` to indicate
that subsequent entries without keys should be loaded as a list element
into that key. If you instead use `keyname:...`, metadata information is
loaded as well and each list element is an object containing `{filename,
basename, content}`.

When loading the context, the default input format is YAML but you can
also use JSON, plain text, and `kv` (key value pairs, space separated,
as used by bazel and many unix tools). To specify the format, rather
than using a `:` you use `:format:`. For example:

    :yaml:ctx.yaml :kv:ctx.kv :json:ctx.json mykey:text:ctx.txt

Note that you must specify a key name under which to load the plain text
file, since it cannot define keys (i.e. is a plain text string). Also,
although the default format is yaml, the default format with `::` is
text. So the following equivalencies hold:

    mykey::context.txt == mykey:text:context.txt
    context.yaml == :context.yaml == :yaml:context.yaml

A common pattern, therefore, is to provide plain text arguments to
the template:

    rjsone -t template.yaml env::+production context.yaml

For complex applications, single argument functions can be added by
prefixing the filename with a `-` (or a `--` for raw string input). For
example:

    b64decode::--'base64 -d'

This adds a base64 decode function to the context which accepts two
arguments as input, an array (command line arguments) and string (stdin),
and outputs a string. In your template, you would use this function by
like `b64decode([], 'Zm9vCg==')`. As with before, you can use format
specifiers (:- is yaml on both sides for the default behaviour, and
you can explicitly specify kv/json/text/yaml between both `::` and
`--`).

# Getting it

[Grab the latest binary](https://github.com/wryun/rjsone/releases) or
build it yourself:

    go get github.com/wryun/rjsone

# Rationale

I often want to template JSON/YAML for declarative infrastructure as code
things (e.g. Kubernetes, CloudFormation, ...), and JSON-e is one of the
few languages that is also valid YAML/JSON, unlike the common option of
hijacking languages designed for plain text (or HTML) templating. If your
template is valid YAML/JSON, your editor can help you out with syntax
highlighting, and after you apply the template you will always have
valid YAML/JSON.

I also want to be 'declarative configuration language' agnostic
(i.e. avoiding Kubernetes specific templating tools...).

Before I discovered JSON-e, I wrote
[o-stache](https://github.com/wryun/ostache/). There are a
list of other structured templating options there, the most
prominent of which is [Jsonnet](http://jsonnet.org/).

# Basic usage example

Please see the JSON-e documentation for how to really use it.

`template.yaml`
```yaml
a: ${foo}
b: ${bar}
c: ${foobar}
```

`context1.yaml`
```yaml
foo: something
```

`context2.yaml`
```yaml
bar: nothing
```

`named.yaml`
```yaml
everything
```

Use YAML files for context:

```sh
$ rjsone -y -t template.yaml context1.yaml context2.yaml foobar:named.yaml
a: something
b: nothing
c: everything
```

Use context on command line (:: rather than : means interpret as raw string
not as JSON/YAML, and + means treat it as a string rather than a filename):
```sh
$ rjsone -y -t template.yaml foo::+something bar::+nothing foobar::+everything
a: something
b: nothing
c: everything
```

*Warning*: if you need to construct anything that's not a pure string from templated
input, you'll probably need to understand JSON-e's '$eval' operator.

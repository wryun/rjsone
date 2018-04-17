`rjsone` (Render JSON-e) is a simple wrapper around the
[JSON-e templating language](https://taskcluster.github.io/json-e/).

It's a safe and easy way to have templates of moderate complexity
for configuration as code 'languages' like Kubernetes and CloudFormation.

Context is usually provided by a list of arguments. By default,
these are interpreted as files. However, if the 'filename' begins with
a '+', the rest of the argument is interpreted as a raw string.
Data is loaded as YAML/JSON by default and merged into the
main context. You can specify a particular key to load a JSON
file into using keyname:filename.yaml; if you specify two colons
(i.e. keyname::filename.yaml) it will load it as a raw string.
When duplicate keys are found, later entries replace earlier
at the top level only (no multi-level merging).

You can also use keyname:.. (or keyname::..) to indicate that subsequent
entries without keys should be loaded as a list element into that key. If you
instead use 'keyname:...', metadata information is loaded as well
(filename, basename, content).

    Usage: rjsone [options] [key:]contextfile [[key:]contextfile ...]
      -i int
            indentation level of JSON output; 0 means no pretty-printing (default 2)
      -t string
            file to use for template (default is -, which is stdin) (default "-")
      -y    output YAML rather than JSON (always reads YAML/JSON)

# Getting it

No builds yet. Just:

    go get github.com/wryun/rjsone

# Rationale

I often want to template JSON/YAML for declarative
infrastructure as code things (e.g. Kubernetes, CloudFormation, ...),
and JSON-e is one of the few languages that is also valid YAML/JSON,
unlike the common option of hijacking languages designed for plain text
(or HTML) templating. If your template is valid YAML/JSON, your editor can
help you out with syntax highlighting, and the after you apply the
template you will always have valid YAML/JSON.

I also want to be 'declarative configuration language' agnostic
(i.e. avoiding Kubernetes specific templating tools...).

Before I discovered JSON-e, I wrote [o-stache](https://github.com/wryun/ostache/). There are a list of other options there, the most prominent of which is
[Jsonnet](http://jsonnet.org/).

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

```sh
$ rjsone -y -t template.yaml context1.yaml context2.yaml foobar:named.yaml
a: something
b: nothing
c: everything
```

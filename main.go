package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/ghodss/yaml"
	jsone "github.com/taskcluster/json-e"
)

const description = `rjsone is a simple wrapper around the JSON-e templating language.

See: https://taskcluster.github.io/json-e/

Context is provided by a list of filename arguments. The files are loaded
as YAML/JSON by default and merged into the main context. You can specify
a particular key to load a JSON file into using keyname:filename.yaml;
if you specify two colons (i.e. keyname::filename.yaml) it will load
it as a raw string. If duplicate keys are found, later entries replace
earlier at the top level only (no multi-level merging).

You can also use keyname:.. (or keyname::..) to indicate that subsequent
files without keys should be loaded as a list element into that key. If you
instead use 'keyname:...', metadata information is loaded as well
(filename, basename, content).
`

type arguments struct {
	yaml         bool
	indentation  int
	templateFile string
	contexts     []string
}

func main() {
	var args arguments
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), description)
		fmt.Fprintf(flag.CommandLine.Output(), "\nUsage: %s [options] [[key:[:]]contextfile ...]\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), "\n")
	}
	flag.StringVar(&args.templateFile, "t", "-", "file to use for template (- is stdin)")
	flag.BoolVar(&args.yaml, "y", false, "output YAML rather than JSON (always reads YAML/JSON)")
	flag.IntVar(&args.indentation, "i", 2, "indentation of JSON output; 0 means no pretty-printing")
	flag.Parse()

	args.contexts = flag.Args()

	if err := run(args); err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Fatal error: %s\n", err)
		os.Exit(2)
	}
}

func run(args arguments) error {
	var template interface{}
	err := readYamlFile(args.templateFile, &template)
	if err != nil {
		return err
	}

	context, err := loadContext(args.contexts)
	if err != nil {
		return err
	}

	output, err := jsone.Render(template, context)
	if err != nil {
		return err
	}

	var byteOutput []byte
	if args.yaml {
		byteOutput, err = yaml.Marshal(output)
	} else if args.indentation == 0 {
		byteOutput, err = json.Marshal(output)
	} else {
		byteOutput, err = json.MarshalIndent(output, "", strings.Repeat(" ", args.indentation))
	}

	if err != nil {
		return err
	}

	os.Stdout.Write(byteOutput)
	if !args.yaml && args.indentation != 0 {
		// MarshalIndent, sadly, doesn't print a newline at the end. Which I think it should.
		os.Stdout.WriteString("\n")
	}
	return nil
}

func loadContext(contextOps []string) (map[string]interface{}, error) {
	context := make(map[string]interface{})

	var currentContextList struct {
		raw      bool
		key      string
		metadata bool
	}

	for _, contextOp := range contextOps {
		splitContextOp := strings.SplitN(contextOp, ":", 2)
		if len(splitContextOp) < 2 { // i.e. we just have a file to load
			filename := splitContextOp[0]
			if currentContextList.key == "" { // we're not in a list - just load it in!
				err := readYamlFile(filename, &context)
				if err != nil {
					return nil, err
				}
			} else { // ah, we're in a list; we should append it to the right key
				var partialContext interface{}
				var err error
				if currentContextList.raw {
					var rawBytes []byte
					rawBytes, err = ioutil.ReadFile(filename)
					partialContext = string(rawBytes)
				} else {
					err = readYamlFile(filename, &partialContext)
				}
				if err != nil {
					return nil, err
				}
				if currentContextList.metadata {
					partialContext = map[string]interface{}{
						"content":  partialContext,
						"filename": filename,
						"basename": path.Base(filename),
					}
				}
				context[currentContextList.key] = append(context[currentContextList.key].([]interface{}), partialContext)
			}
		} else { // we have a key
			key := splitContextOp[0]
			if key == "" {
				return nil, fmt.Errorf("must specify key before ':' in %q", contextOp)
			}
			raw := strings.HasPrefix(splitContextOp[1], ":")
			var filename string
			if raw {
				filename = splitContextOp[1][1:]
			} else {
				filename = splitContextOp[1]
			}
			if filename == "" {
				return nil, fmt.Errorf("must specify filename or ellipsis after ':' in %q", contextOp)
			}

			if filename == ".." || filename == "..." { // we have a list to follow - switch mode!
				if _, ok := context[key].([]interface{}); !ok {
					context[key] = make([]interface{}, 0)
				}
				currentContextList.key = key
				currentContextList.raw = raw
				currentContextList.metadata = filename == "..."
			} else { // otherwise, we end any existing list and set this directly
				currentContextList.key = ""
				var partialContext interface{}
				var err error
				if raw {
					var rawBytes []byte
					rawBytes, err = ioutil.ReadFile(filename)
					partialContext = string(rawBytes)
				} else {
					err = readYamlFile(filename, &partialContext)
				}
				if err != nil {
					return nil, err
				}
				context[key] = partialContext
			}
		}
	}

	return context, nil
}

func readYamlFile(filename string, o interface{}) error {
	var byteTemplate []byte
	var err error
	if filename == "-" {
		byteTemplate, err = ioutil.ReadAll(os.Stdin)
	} else {
		byteTemplate, err = ioutil.ReadFile(filename)
	}

	if err != nil {
		return err
	}

	return yaml.Unmarshal(byteTemplate, &o)
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/imdario/mergo"
	jsone "github.com/taskcluster/json-e"
	// Quick hack of ghodss YAML to expose a new method
	yaml_ghodss "github.com/wryun/yaml-1"
	yaml_v2 "gopkg.in/yaml.v2"
)

const description = `rjsone is a simple wrapper around the JSON-e templating language.

See: https://taskcluster.github.io/json-e/

Context is usually provided by a list of arguments. By default,
these are interpreted as files. Data is loaded as YAML/JSON by default
and merged into the main context. If the 'filename' begins with a +,
the rest of the argument is interpreted as a raw string rather than
reading the file. For example:

    rjsone -t template.yaml context.yaml '+{"foo": 1}'

When duplicate keys are found, later entries replace earlier at the
top level only unless the -d flag is passed to perform deep merging.

You can specify a particular context key to load a YAML/JSON file into
using keyname:filename.yaml. You can also use keyname:.. to indicate
that subsequent entries without keys should be loaded as a list element
into that key. If you instead use keyname:..., metadata information is
loaded as well and each list element is an object containing {filename,
basename, content}.

When loading the context, the default input format is YAML but you can
also use JSON, plain text, and kv (key value pairs, space separated,
as used by bazel and many unix tools). To specify the format, rather
than using a : you use :format:. For example:

    :yaml:ctx.yaml :kv:ctx.kv :json:ctx.json mykey:text:ctx.txt

Note that you must specify a key name under which to load the plain text
file, since it cannot define keys (i.e. is a plain text string). Also,
although the default format is yaml, the default format with :: is
text. So the following equivalencies hold:

    mykey::context.txt == mykey:text:context.txt
    context.yaml == :context.yaml == :yaml:context.yaml

A common pattern, therefore, is to provide plain text arguments to
the template:

    rjsone -t template.yaml env::+production context.yaml

For complex applications, single argument functions can be added by
prefixing the filename with a - (or a -- for raw string input). For
example:

    b64decode::--'base64 -d'

This adds a base64 decode function to the context which accepts two
arguments as input, an array (command line arguments) and string (stdin),
and outputs a string. In your template, you would use this function by
like b64decode([], 'Zm9vCg=='). As with before, you can use format
specifiers (:- is yaml on both sides for the default behaviour, and
you can explicitly specify kv/json/text/yaml between both :: and
--).
`

type arguments struct {
	yaml         bool
	indentation  int
	templateFile string
	verbose      bool
	deepMerge    bool
	outputFile   string
	contexts     []context
}

type content interface {
	load() (interface{}, error)
	metadata() map[string]interface{}
}

func main() {
	var args arguments
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), description)
		fmt.Fprintf(flag.CommandLine.Output(), "\nUsage: %s [options] [context ...]\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), "\n")
	}
	flag.StringVar(&args.templateFile, "t", "-", "file to use for template (- is stdin)")
	flag.BoolVar(&args.yaml, "y", false, "output YAML rather than JSON (always reads YAML/JSON)")
	flag.BoolVar(&args.verbose, "v", false, "show information about processing on stderr")
	flag.BoolVar(&args.deepMerge, "d", false, "performs a deep merge of contexts")
	flag.StringVar(&args.outputFile, "o", "-", "output to a file (default is -, which is stdout)")
	flag.IntVar(&args.indentation, "i", 2, "indentation of JSON output; 0 means no pretty-printing")
	flag.Parse()

	args.contexts = parseContexts(flag.Args())
	logger := log.New(os.Stderr, "", 0)

	if err := run(logger, args); err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Fatal error: %s\n", err)
		os.Exit(2)
	}
}

func run(l *log.Logger, args arguments) (finalError error) {
	closeWithError := func(c io.Closer) {
		if err := c.Close(); err != nil && finalError == nil {
			finalError = err
		}
	}

	context, err := loadContext(args.contexts, args.deepMerge)
	if err != nil {
		return err
	}

	if args.verbose {
		l.Println("Calculated context:")
		output, err := yaml_ghodss.Marshal(context)
		if err != nil {
			return err
		}
		l.Println(string(output))
	}

	var input io.ReadCloser
	if args.templateFile == "-" {
		input = os.Stdin
	} else {
		input, err = os.Open(args.templateFile)
		if err != nil {
			return err
		}
		defer closeWithError(input)
	}

	var out io.WriteCloser
	if args.outputFile == "-" {
		out = os.Stdout
	} else {
		out, err = os.Create(args.outputFile)
		if err != nil {
			return err
		}
		defer closeWithError(out)
	}

	var encoder *yaml_v2.Encoder
	if args.yaml {
		encoder = yaml_v2.NewEncoder(out)
		defer closeWithError(encoder)
	}

	decoder := yaml_v2.NewDecoder(input)
	for {
		// json-e wants types as output by json, so we have to reach
		// into the annoying ghodss/yaml code to do the type conversion.
		// We can't use it directly (trivially), because it doesn't have
		// multi-document support.
		var passthroughTemplate interface{}
		err := decoder.Decode(&passthroughTemplate)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var template interface{}
		err = yaml_ghodss.YAMLTypesToJSONTypes(passthroughTemplate, &template)
		if err != nil {
			return err
		}

		output, err := jsone.Render(template, context)
		if err != nil {
			return err
		}

		if args.yaml {
			err = encoder.Encode(output)
			if err != nil {
				return err
			}
		} else {
			var byteOutput []byte
			if args.indentation == 0 {
				byteOutput, err = json.Marshal(output)
			} else {
				byteOutput, err = json.MarshalIndent(output, "", strings.Repeat(" ", args.indentation))
				// MarshalIndent, sadly, doesn't add a newline at the end. Which I think it should.
				byteOutput = append(byteOutput, 0x0a)
			}

			if err != nil {
				return err
			}

			_, err = out.Write(byteOutput)
			if err != nil {
				return err
			}
		}
	}
}

func loadContext(contexts []context, deepMerge bool) (map[string]interface{}, error) {
	finalContext := make(map[string]interface{})

	for _, context := range contexts {
		untypedNewContext, err := context.eval()
		if err != nil {
			return nil, err
		}

		newContext, ok := untypedNewContext.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("context %s had no top level keys: %q", context.original, untypedNewContext)
		}

		if deepMerge {
			err = mergo.Merge(&finalContext, newContext, mergo.WithOverride)
			if err != nil {
				return nil, err
			}
		} else {
			for k, v := range newContext {
				finalContext[k] = v
			}
		}
	}

	return finalContext, nil
}

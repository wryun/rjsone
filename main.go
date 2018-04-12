package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/ghodss/yaml"
	jsone "github.com/taskcluster/json-e"
)

const description = `rjsone is a simple wrapper around the JSON-e templating language.

See: https://taskcluster.github.io/json-e/
`

type arguments struct {
	yaml              bool
	indentation       int
	templateFile      string
	contextFiles      []string
	namedContextFiles map[string]string
}

func main() {
	args := arguments{
		namedContextFiles: make(map[string]string),
	}
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), description)
		fmt.Fprintf(flag.CommandLine.Output(), "\nUsage: %s [options] [key:]contextfile [[key:]contextfile ...]\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), "\n")
	}
	flag.StringVar(&args.templateFile, "t", "-", "file to use for template (- is stdin)")
	flag.BoolVar(&args.yaml, "y", false, "output YAML rather than JSON (always reads YAML/JSON)")
	flag.IntVar(&args.indentation, "i", 2, "indentation of JSON output; 0 means no pretty-printing")
	flag.Parse()
	for _, context := range flag.Args() {
		splitContext := strings.SplitN(context, ":", 2)
		if len(splitContext) < 2 {
			args.contextFiles = append(args.contextFiles, splitContext[0])
		} else {
			args.namedContextFiles[splitContext[0]] = splitContext[1]
		}
	}

	if err := run(args); err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Fatal error: %s\n", err)
		os.Exit(2)
	}
}

func run(args arguments) error {
	template, err := loadTemplate(args.templateFile)
	if err != nil {
		return err
	}

	context, err := loadContext(args.contextFiles, args.namedContextFiles)

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

	os.Stdout.Write(byteOutput)
	if !args.yaml && args.indentation != 0 {
		// MarshalIndent, sadly, doesn't print a newline at the end. Which I think it should.
		os.Stdout.WriteString("\n")
	}
	return nil
}

func loadContext(contextFiles []string, namedContextFiles map[string]string) (map[string]interface{}, error) {
	context := make(map[string]interface{})

	for _, filename := range contextFiles {
		byteContext, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}

		err = yaml.Unmarshal(byteContext, &context)
		if err != nil {
			return nil, err
		}
	}

	for key, filename := range namedContextFiles {
		byteContext, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}

		var partialContext interface{}
		err = yaml.Unmarshal(byteContext, &partialContext)
		if err != nil {
			return nil, err
		}
		context[key] = partialContext
	}

	return context, nil
}

func loadTemplate(templateFile string) (interface{}, error) {
	var template interface{}
	var byteTemplate []byte
	var err error
	if templateFile == "-" {
		byteTemplate, err = ioutil.ReadAll(os.Stdin)
	} else {
		byteTemplate, err = ioutil.ReadFile(templateFile)
	}

	if err != nil {
		return nil, err
	}

	return template, yaml.Unmarshal(byteTemplate, &template)
}

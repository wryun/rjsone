package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/imdario/mergo"
	jsone "github.com/taskcluster/json-e"
	jsone_interpreter "github.com/taskcluster/json-e/interpreter"
	// Quick hack of ghodss YAML to expose a new method
	yaml_ghodss "github.com/wryun/yaml-1"
	yaml_v2 "gopkg.in/yaml.v2"
)

const description = `rjsone is a simple wrapper around the JSON-e templating language.

See: https://taskcluster.github.io/json-e/

Context is usually provided by a list of arguments. By default,
these are interpreted as files. Data is loaded as YAML/JSON by default
and merged into the main context.

You can specify a particular context key to load a YAML/JSON
file into using keyname:filename.yaml; if you specify two colons
(i.e. keyname::filename.yaml) it will load it as a raw string.
When duplicate keys are found, later entries replace earlier
at the top level only (no multi-level merging), unless the '-d' flag is passed to perform deep merging.
In this context, if the filename begins with a '+', the rest of the argument
is interpreted as a raw string.

You can also use keyname:.. (or keyname::..) to indicate that subsequent
entries without keys should be loaded as a list element into that key. If you
instead use 'keyname:...', metadata information is loaded as well
(filename, basename, content).

For complex applications, single argument functions can be added by prefixing
the filename with a '-' (or a '--' for raw string input). For example:

    b64decode::--'base64 -d'

This adds a base64 decode function to the context which accepts an array
(command line arguments) and string (stdin) as input and outputs a string.
For example, you could use this function like b64decode([], 'Zm9vCg==').
Conversely, if you use :-, your command must accept JSON as stdin and
output JSON or YAML.
`

type arguments struct {
	yaml         bool
	indentation  int
	templateFile string
	verbose      bool
	deepMerge    bool
	contexts     []context
}

type context interface {
	eval() (interface{}, error)
}

type commonContext struct {
	key string
	raw bool
}

func (cc *commonContext) cleanupResult(result interface{}) (interface{}, error) {
	var finalResult interface{}
	if cc.raw {
		switch r := result.(type) {
		case []byte:
			finalResult = string(r)
		default:
			// otherwise, let's just let JSON-e complain...
			finalResult = result
		}
	} else {
		var rawBytes []byte
		switch r := result.(type) {
		case string:
			rawBytes = []byte(r)
		case []byte:
			rawBytes = r
		default:
			return nil, errors.New("somehow got a non string/byte slice?")
		}
		if err := yaml_ghodss.Unmarshal(rawBytes, &finalResult); err != nil {
			return nil, err
		}
	}

	if cc.key != "" {
		return map[string]interface{}{cc.key: finalResult}, nil
	}

	return finalResult, nil
}

type fileContext struct {
	commonContext
	filename string
}

func (fc *fileContext) eval() (interface{}, error) {
	result, err := ioutil.ReadFile(fc.filename)
	if err != nil {
		return nil, err
	}

	return fc.cleanupResult(result)
}

type stdinContext struct {
	commonContext
	filename string
}

func (sc *stdinContext) eval() (interface{}, error) {
	result, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}

	return sc.cleanupResult(result)
}

type functionContext struct {
	commonContext
	function  string
	rawOutput bool
	rawInput  bool
}

func (fc *functionContext) eval() (interface{}, error) {
	var f interface{}
	commandArray := strings.Split(fc.function, " ")

	if fc.rawInput && fc.rawOutput {
		f = func(args []interface{}, stdin string) (string, error) {
			stringArgs, err := castToStrings(args)
			extendedCommandArray := append(commandArray, stringArgs...)
			command := exec.Command(extendedCommandArray[0], extendedCommandArray[1:]...)
			command.Stderr = os.Stderr
			command.Stdin = bytes.NewReader([]byte(stdin))
			stdoutBytes, err := command.Output()
			if err != nil {
				return "", err
			}
			return string(stdoutBytes), nil
		}
	} else if fc.rawInput {
		f = func(args []interface{}, stdin string) (interface{}, error) {
			stringArgs, err := castToStrings(args)
			extendedCommandArray := append(commandArray, stringArgs...)
			command := exec.Command(extendedCommandArray[0], extendedCommandArray[1:]...)
			command.Stderr = os.Stderr
			command.Stdin = bytes.NewReader([]byte(stdin))
			stdoutBytes, err := command.Output()
			if err != nil {
				return nil, err
			}

			var o interface{}
			err = yaml_ghodss.Unmarshal(stdoutBytes, &o)
			if err != nil {
				return nil, err
			}
			return o, nil
		}
	} else if fc.rawOutput {
		f = func(args []interface{}, stdin interface{}) (string, error) {
			jsonBytes, err := json.Marshal(stdin)
			if err != nil {
				return "", err
			}

			stringArgs, err := castToStrings(args)
			extendedCommandArray := append(commandArray, stringArgs...)
			command := exec.Command(extendedCommandArray[0], extendedCommandArray[1:]...)
			command.Stderr = os.Stderr
			command.Stdin = bytes.NewReader(jsonBytes)
			stdoutBytes, err := command.Output()
			return string(stdoutBytes), err
		}
	} else {
		f = func(args []interface{}, stdin interface{}) (interface{}, error) {
			jsonBytes, err := json.Marshal(stdin)
			if err != nil {
				return "", err
			}

			stringArgs, err := castToStrings(args)
			extendedCommandArray := append(commandArray, stringArgs...)
			command := exec.Command(extendedCommandArray[0], extendedCommandArray[1:]...)
			command.Stderr = os.Stderr
			command.Stdin = bytes.NewReader(jsonBytes)
			stdoutBytes, err := command.Output()
			if err != nil {
				return nil, err
			}

			var o interface{}
			err = yaml_ghodss.Unmarshal(stdoutBytes, &o)
			if err != nil {
				return nil, err
			}
			return stdin, nil
		}
	}

	return jsone_interpreter.WrapFunction(f), nil
}

type textContext struct {
	commonContext
	text string
}

func (tc *textContext) eval() (interface{}, error) {
	return tc.cleanupResult(tc.text)
}

type listContext struct {
	commonContext
	contexts []context
	metadata bool
}

func (lc *listContext) eval() (interface{}, error) {
	outputList := make([]interface{}, 0, len(lc.contexts))

	for _, context := range lc.contexts {
		result, err := context.eval()
		if err != nil {
			return nil, err
		}

		if !lc.metadata {
			outputList = append(outputList, result)
		}

		metadataResult := map[string]interface{}{
			"content": result,
		}
		if fc, ok := context.(*fileContext); ok {
			basename := path.Base(fc.filename)
			metadataResult["filename"] = fc.filename
			metadataResult["basename"] = basename
			metadataResult["name"] = strings.TrimSuffix(basename, filepath.Ext(basename))
		}
		outputList = append(outputList, metadataResult)
	}

	return lc.cleanupResult(outputList)
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
	flag.BoolVar(&args.verbose, "v", false, "show information about processing on stderr")
	flag.BoolVar(&args.deepMerge, "d", false, "performs a deep merge of contexts")
	flag.IntVar(&args.indentation, "i", 2, "indentation of JSON output; 0 means no pretty-printing")
	flag.Parse()

	args.contexts = parseContexts(flag.Args())
	logger := log.New(os.Stderr, "", 0)

	if err := run(logger, os.Stdout, args); err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Fatal error: %s\n", err)
		os.Exit(2)
	}
}

func parseContexts(rawContextInfo []string) []context {
	contexts := make([]context, 0)

	currentContexts := &contexts

	for _, contextOp := range rawContextInfo {
		key := ""
		var entry string
		if strings.HasPrefix(contextOp, "+") {
			entry = contextOp
		} else {
			splitContextOp := strings.SplitN(contextOp, ":", 2)
			if len(splitContextOp) < 2 {
				entry = splitContextOp[0]
			} else {
				key = splitContextOp[0]
				entry = splitContextOp[1]
				// If we have a new key, we should jump out of any list we're in
				currentContexts = &contexts
			}
		}

		context := parseContext(key, entry)
		*currentContexts = append(*currentContexts, context)
		if listContext, ok := context.(*listContext); ok {
			currentContexts = &listContext.contexts
		}
	}

	return contexts
}

func parseContext(key string, entry string) context {
	baseContext := commonContext{
		raw: strings.HasPrefix(entry, ":"),
		key: key,
	}

	if baseContext.raw {
		entry = entry[1:]
	}

	switch {
	case entry == "..":
		return &listContext{commonContext: baseContext, metadata: false}
	case entry == "...":
		return &listContext{commonContext: baseContext, metadata: true}
	case strings.HasPrefix(entry, "+"):
		return &textContext{commonContext: baseContext, text: entry[1:]}
	case entry == "-":
		return &stdinContext{commonContext: baseContext}
	case strings.HasPrefix(entry, "--"):
		return &functionContext{commonContext: commonContext{raw: true, key: key}, rawInput: baseContext.raw, rawOutput: true, function: entry[2:]}
	case strings.HasPrefix(entry, "-"):
		return &functionContext{commonContext: commonContext{raw: true, key: key}, rawInput: baseContext.raw, rawOutput: false, function: entry[1:]}
	default:
		return &fileContext{commonContext: baseContext, filename: entry}
	}
}

func run(l *log.Logger, out io.Writer, args arguments) error {
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

	var input io.Reader
	if args.templateFile == "-" {
		input = os.Stdin
	} else {
		input, err = os.Open(args.templateFile)
		if err != nil {
			return err
		}
	}

	var encoder *yaml_v2.Encoder
	if args.yaml {
		encoder = yaml_v2.NewEncoder(out)
		defer encoder.Close()
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
			return nil, errors.New("TODO cannot have unkeyed top level context")
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

func castToStrings(slice []interface{}) ([]string, error) {
	result := make([]string, len(slice))
	for i, v := range slice {
		if s, ok := v.(string); !ok {
			return nil, errors.New("function command line arguments must be strings (use stdin or $json)")
		} else {
			result[i] = s
		}
	}
	return result, nil
}

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	jsone_interpreter "github.com/taskcluster/json-e/interpreter"
	// Quick hack of ghodss YAML to expose a new method
	yaml_ghodss "github.com/wryun/yaml-1"
)

func parseContexts(rawContextInfo []string) []context {
	contexts := make([]context, 0)

	var lc *listContext

	for _, contextOp := range rawContextInfo {
		key := ""
		var entry string
		if strings.HasPrefix(contextOp, "+") {
			// if it starts with a '+', we know it's raw and we shouldn't
			// try to find keys in it (otherwise we can't easily pass raw
			// JSON/YAML as an argument)
			entry = contextOp
		} else {
			splitContextOp := strings.SplitN(contextOp, ":", 2)
			if len(splitContextOp) < 2 {
				entry = splitContextOp[0]
			} else {
				key = splitContextOp[0]
				entry = splitContextOp[1]
				// If we have a new key, we should jump out of any list we're in
				lc = nil
			}
		}

		context := parseContext(key, entry, lc)
		if newLc, ok := context.(*listContext); ok {
			lc = newLc
			contexts = append(contexts, context)
		} else if lc != nil {
			lc.contexts = append(lc.contexts, context)
		} else {
			contexts = append(contexts, context)
		}

	}

	return contexts
}

func parseContext(key string, entry string, lc *listContext) context {
	cc := commonContext{
		raw: strings.HasPrefix(entry, ":"),
		key: key,
	}

	if cc.raw {
		entry = entry[1:]
	}

	if lc != nil {
		// we inherit rawness from the listContext. Quirk of the syntax...
		// (NB - there will not be a colon, because if there were it would
		// have seen a key and aborted the list)
		cc.raw = lc.rawChildren
	}

	// TODO: this currently allows a bunch of stupid things
	// (e.g. embedded listContexts...). Should write a proper grammar.
	switch {
	case entry == "..":
		return &listContext{commonContext: commonContext{raw: true, key: key}, rawChildren: cc.raw, metadata: false}
	case entry == "...":
		return &listContext{commonContext: commonContext{raw: true, key: key}, rawChildren: cc.raw, metadata: true}
	case strings.HasPrefix(entry, "+"):
		return &textContext{commonContext: cc, text: entry[1:]}
	case entry == "-":
		return &stdinContext{commonContext: cc}
	case strings.HasPrefix(entry, "--"):
		return &functionContext{commonContext: commonContext{raw: true, key: key}, rawInput: cc.raw, rawOutput: true, function: entry[2:]}
	case strings.HasPrefix(entry, "-"):
		return &functionContext{commonContext: commonContext{raw: true, key: key}, rawInput: cc.raw, rawOutput: false, function: entry[1:]}
	default:
		return &fileContext{commonContext: cc, filename: entry}
	}
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
			return nil, fmt.Errorf("somehow got a non string/byte slice: %q", result)
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

	return fc.cleanupResult(jsone_interpreter.WrapFunction(f))
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
	contexts    []context
	metadata    bool
	rawChildren bool
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
			continue
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

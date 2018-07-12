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

	"github.com/imdario/mergo"

	jsone_interpreter "github.com/taskcluster/json-e/interpreter"
	// Quick hack of ghodss YAML to expose a new method
	yaml_ghodss "github.com/wryun/yaml-1"
)

func parseContexts(rawContexts []string) []context {
	contexts := make([]context, 0)

	var lc *listContent

	for _, rawContext := range rawContexts {
		key := ""
		var rawContent string
		if strings.HasPrefix(rawContext, "+") {
			// if it starts with a '+', we know it's raw and we shouldn't
			// try to find keys in it (otherwise we can't easily pass raw
			// JSON/YAML as an argument)
			rawContent = rawContext
		} else {
			splitContext := strings.SplitN(rawContext, ":", 2)
			if len(splitContext) < 2 {
				rawContent = splitContext[0]
			} else {
				key = splitContext[0]
				rawContent = ":" + splitContext[1]
			}
		}

		if key != "" {
			// If we have a new key, we should jump out of any list we're in
			lc = nil
		}

		parsedContext := context{rawContext, key, parseContent(rawContent, lc)}
		if newLc, ok := parsedContext.content.(*listContent); ok {
			lc = newLc
			contexts = append(contexts, parsedContext)
		} else if lc != nil {
			lc.contexts = append(lc.contexts, parsedContext)
		} else {
			contexts = append(contexts, parsedContext)
		}

	}

	return contexts
}

func parseContent(content string, lc *listContent) content {
	fmtPointer, data := parseFormat(content)

	var format inputFormat
	if fmtPointer == nil {
		if lc == nil {
			format = yamlFormat
		} else {
			// we inherit rawness from the listContent. Quirk of the syntax...
			// (NB - there will not be a colon, because if there were it would
			// have seen a key and aborted the list)
			format = lc.childFormat
		}
	} else if *fmtPointer == "" {
		format = textFormat
	} else {
		format = *fmtPointer
	}

	// TODO: this currently allows a bunch of stupid things
	// (e.g. embedded listContents...). Should write a proper grammar.
	switch {
	case data == "..":
		return &listContent{childFormat: format, showMetadata: false}
	case data == "...":
		return &listContent{childFormat: format, showMetadata: true}
	case strings.HasPrefix(data, "+"):
		return &textContent{format: format, text: data[1:]}
	case data == "-":
		return &stdinContent{}
	case strings.HasPrefix(data, "--"):
		return &functionContent{rawInput: format == textFormat, rawOutput: true, function: data[2:]}
	case strings.HasPrefix(data, "-"):
		return &functionContent{rawInput: format == textFormat, rawOutput: false, function: data[1:]}
	default:
		return &fileContent{format: format, filename: data}
	}
}

type inputFormat string

const (
	yamlFormat = inputFormat("yaml")
	jsonFormat = inputFormat("json")
	kvFormat   = inputFormat("kv")
	textFormat = inputFormat("text")
)

// parseFormat part of content (content = :format:data)
// nothing there -> nil
func parseFormat(content string) (*inputFormat /* format */, string /* data */) {
	if !strings.HasPrefix(content, ":") {
		// it's just a text string/filename - no format specifier
		return nil, content
	}

	content = content[1:]

	// Hack: if the first thing after the ':' is a '+', it must be
	// raw yaml/json (and we don't want to split on ':', as that might
	// be a valid char...)
	if strings.HasPrefix(content, "+") {
		f := yamlFormat
		return &f, content
	}

	splitContent := strings.SplitN(content, ":", 2)

	if len(splitContent) == 1 {
		return nil, splitContent[0]
	}

	// we don't validate here to avoid errors - will fail on eval.
	f := inputFormat(splitContent[0])
	return &f, splitContent[1]
}

type context struct {
	original string
	key      string

	content content
}

func (c *context) eval() (interface{}, error) {
	result, err := c.content.load()
	if err != nil {
		return nil, err
	}

	if c.key != "" {
		return map[string]interface{}{c.key: result}, nil
	}

	return result, nil
}

func loadBytes(format inputFormat, data []byte) (interface{}, error) {
	switch format {
	case jsonFormat:
		var result interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, err
		}
		return result, nil
	case yamlFormat:
		var result interface{}
		if err := yaml_ghodss.Unmarshal(data, &result); err != nil {
			return nil, err
		}
		return result, nil
	case textFormat:
		return string(data), nil
	case kvFormat:
		// TODO unicode?
		lines := strings.Split(string(data), "\n")
		result := make(map[string]interface{}, len(lines))
		for _, line := range lines {
			if line == "" {
				continue
			}

			splitLine := strings.SplitN(line, " ", 2)
			if len(splitLine) != 2 {
				return nil, fmt.Errorf("line not in kv format: %q", line)
			}
			result[splitLine[0]] = splitLine[1]
		}
		return result, nil
	default:
		return nil, fmt.Errorf("format %q not supported", format)
	}
}

type fileContent struct {
	format   inputFormat
	filename string
}

func (fc *fileContent) load() (interface{}, error) {
	resultBytes, err := ioutil.ReadFile(fc.filename)
	if err != nil {
		return nil, err
	}
	return loadBytes(fc.format, resultBytes)
}

func (fc *fileContent) metadata() map[string]interface{} {
	basename := path.Base(fc.filename)
	return map[string]interface{}{
		"filename": fc.filename,
		"basename": basename,
		"name":     strings.TrimSuffix(basename, filepath.Ext(basename)),
	}
}

type stdinContent struct {
	format inputFormat
}

func (sc *stdinContent) load() (interface{}, error) {
	resultBytes, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	return loadBytes(sc.format, resultBytes)
}

func (sc *stdinContent) metadata() map[string]interface{} {
	return map[string]interface{}{}
}

type functionContent struct {
	function  string
	rawOutput bool
	rawInput  bool
}

type textContent struct {
	format inputFormat
	text   string
}

func (tc *textContent) load() (interface{}, error) {
	return loadBytes(tc.format, []byte(tc.text))
}

func (tc *textContent) metadata() map[string]interface{} {
	return map[string]interface{}{}
}

func (fc *functionContent) load() (interface{}, error) {
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

func (fc *functionContent) metadata() map[string]interface{} {
	return map[string]interface{}{}
}

type listContent struct {
	contexts     []context
	showMetadata bool
	childFormat  inputFormat
}

func (lc *listContent) load() (interface{}, error) {
	outputList := make([]interface{}, 0, len(lc.contexts))

	for _, context := range lc.contexts {
		result, err := context.eval()
		if err != nil {
			return nil, err
		}

		if !lc.showMetadata {
			outputList = append(outputList, result)
			continue
		}

		metadataResult := map[string]interface{}{
			"content": result,
		}
		mergo.Merge(&metadataResult, context.content.metadata())
		outputList = append(outputList, metadataResult)
	}

	return outputList, nil
}

func (lc *listContent) metadata() map[string]interface{} {
	return map[string]interface{}{}
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

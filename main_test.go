package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strconv"
	"testing"
)

func newTempFile(s string) string {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		panic(err)
	}
	return f.Name()
}

var filenameSubstitution = regexp.MustCompile(`\$\d`)

func substituteFilename(arg string, filenames []string) string {
	return string(filenameSubstitution.ReplaceAllFunc([]byte(arg), func(input []byte) []byte {
		findex, err := strconv.Atoi(string(input[1:]))
		if err != nil {
			panic(err)
		}
		return []byte(filenames[findex-1])
	}))
}

func Test_run(t *testing.T) {
	l := log.New(ioutil.Discard, "", 0)
	tests := []struct {
		name        string
		wantSuccess bool
		files       []string // actually input file contents
		args        arguments
		wantOut     string
	}{
		{
			"full context from file", true,
			[]string{"${x}", `{"x": "foo"}`},
			arguments{templateFile: "$1", contexts: []string{"$2"}},
			`"foo"`,
		},
		{
			"merge context from multiple files", true,
			[]string{"${x}${y}", `{"x": "foo", "y": "bar"}`, `{"x": "foobar"}`},
			arguments{templateFile: "$1", contexts: []string{"$2", "$3"}},
			`"foobarbar"`,
		},
		{
			"raw text as argument", true,
			[]string{"${x}"},
			arguments{templateFile: "$1", contexts: []string{"x::+foo"}},
			`"foo"`,
		},
		{
			"raw text in file", true,
			[]string{"${x}", "foo"},
			arguments{templateFile: "$1", contexts: []string{"x::$2"}},
			`"foo"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			filenames := make([]string, len(tt.files))
			for i, fileContent := range tt.files {
				filenames[i] = newTempFile(fileContent)
				defer os.Remove(filenames[i])
			}
			tt.args.templateFile = substituteFilename(tt.args.templateFile, filenames)
			for i, context := range tt.args.contexts {
				tt.args.contexts[i] = substituteFilename(context, filenames)
			}

			if err := run(l, out, tt.args); (err == nil) != tt.wantSuccess {
				t.Errorf("run() error = %v", err)
				return
			}
			if gotOut := out.String(); gotOut != tt.wantOut {
				t.Errorf("run() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}

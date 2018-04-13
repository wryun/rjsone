#!/bin/sh

go run ../../main.go -y -t template.yaml context1.yaml context2.yaml foobar:named.yaml text::input.txt list:.. context1.yaml context2.yaml list::.. input.txt withfilename::... input.txt

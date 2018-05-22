#!/bin/sh

rjsone -y -t template.yaml b64decode::--'base64 -d' b64encode::--base64 rawparam::+hello context1.yaml context2.yaml foobar:named.yaml text::input.txt list:.. context1.yaml context2.yaml list::.. input.txt withfilename::... input.txt

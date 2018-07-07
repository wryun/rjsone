#!/bin/sh

rjsone -y -t template.yaml encode::--'tr a-zA-Z' decode::--'tr b-zaB-ZA' rawparam::+hello context1.yaml context2.yaml foobar:named.yaml text::input.txt list:.. context1.yaml context2.yaml list2::.. input.txt withfilename::... input.txt

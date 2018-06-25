#!/bin/sh

rjsone -d -y -t template.yaml root.yaml root:sub.yaml root_list::+"string override"

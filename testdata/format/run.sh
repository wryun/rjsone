#!/bin/sh

exec rjsone -y -t template.yaml :yaml:context.yaml :kv:context.kv :json:context.json bar:text:context.txt

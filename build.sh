#!/usr/bin/env bash
# ******************************************************
# DESC    : supervisord build script
# AUTHOR  : Alex Stocks
# VERSION : 1.0
# LICENCE : Apache License 2.0
# EMAIL   : alexstocks@foxmail.com
# MOD     : 2019-01-30 21:21
# FILE    : build.sh
# ******************************************************

# go build -ldflags "-X main.CheckDeadlock=true" -x -race -o supervisord
GOOS=linux go build -ldflags "-X main.CheckDeadlock=true" -o supervisord

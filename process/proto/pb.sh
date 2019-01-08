#!/usr/bin/env bash
# ******************************************************
# DESC    :
# AUTHOR  : Alex Stocks
# VERSION : 1.0
# LICENCE : Apache License 2.0
# EMAIL   : alexstocks@foxmail.com
# MOD     : 2018-11-28 16:05
# FILE    : pb.sh
# ******************************************************

# descriptor.proto
gopath=~/test/golang/lib/src/github.com/gogo/protobuf/protobuf
# If you are using any gogo.proto extensions you will need to specify the
# proto_path to include the descriptor.proto and gogo.proto.
# gogo.proto is located in github.com/gogo/protobuf/gogoproto
gogopath=~/test/golang/lib/src/

protoc -I=$gopath:$gogopath:./ --gogoslick_out=./ --govalidators_out=gogoimport=true:.  process_info.proto
protoc-go-inject-tag -input=./process_info.pb.go

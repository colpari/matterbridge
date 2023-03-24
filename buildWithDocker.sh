#!/bin/sh

sudo docker run -it -u $(id -u) -v $PWD:/src/ -w /src/ -e HOME=/src/builder-home/ golang:latest go build  -v

#!/bin/sh

sudo docker run -it -u $(id -u) -v $PWD:/src/ -w /src/ -e CGO_ENABLED=0 -e HOME=/src/builder-home/ golang:latest go "$@"


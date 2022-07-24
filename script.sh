#!/bin/bash

cd ./main

trap "rm ./main" EXIT

go build ./main.go
./main
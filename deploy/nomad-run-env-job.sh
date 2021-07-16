#!/bin/bash

# This script substitutes environment variables in the given Nomad job and runs it afterwards.

if [[ "$#" -ne 2 ]]; then
  echo "Usage: $0 path/to/infile path/to/outfile"
fi

envsubst "$(env | sed -e 's/=.*//' -e 's/^/\$/g')" < $1 > $2
nomad validate $2
# nomad plan returns 1 if allocations are created or destroyed which is what we want here
# https://www.nomadproject.io/docs/commands/job/plan#usage
nomad plan     $2 || [ $? == 1 ]
nomad run      $2

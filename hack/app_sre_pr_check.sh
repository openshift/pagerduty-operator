#!/bin/bash

# AppSRE team CD

set -exv

CURRENT_DIR=$(dirname "$0")
REPO_ROOT=$(git rev-parse --show-toplevel)
YAML_FILES=$(git ls-tree --full-tree -r --name-only HEAD | egrep '\.ya?ml$')

for YAML_FILE in $YAML_FILES
do 
	# `-e` will fail the script if one of these is bad
	python "$CURRENT_DIR"/validate_yaml.py $REPO_ROOT/$YAML_FILE
done 

BASE_IMG="pagerduty-operator"
IMG="${BASE_IMG}:latest"

make build

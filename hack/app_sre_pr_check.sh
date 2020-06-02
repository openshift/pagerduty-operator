#!/bin/bash

# AppSRE team CD

set -exv

CURRENT_DIR=$(dirname "$0")
YAML_DIRS=( "${CURRENT_DIR}/../deploy/crds" "${CURRENT_DIR}/../manifests" )

for DIR in $YAML_DIRS 
do 
	if [[ -d $DIR ]]; then
		python "$CURRENT_DIR"/validate_yaml.py $CRD_DIR

		if [ "$?" != "0" ]; then
		    exit 1
		fi

	else
		echo "WARNING: No yaml for validation in directiory $DIR"
	fi
done 

exit 0

BASE_IMG="pagerduty-operator"
IMG="${BASE_IMG}:latest"

IMG="$IMG" make build

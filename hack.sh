#!/usr/bin/bash
IMAGE="quay.io/whearn/pagerduty-operator:"
OLD_VERSION=$(grep "Version" version/version.go | awk '{ print $3 }' | sed 's/"//g')

IFS="." read -r -a array <<< "${OLD_VERSION}"
let array[2]=${array[2]}+1
function join_by { local IFS="$1"; shift; echo "$*"; }
NEW_VERSION=$(join_by . "${array[@]}")

sed -i "s/${OLD_VERSION}/${NEW_VERSION}/" version/version.go
sed -i "s/${OLD_VERSION}/${NEW_VERSION}/" manifests/05-operator.yaml

operator-sdk build ${IMAGE}${NEW_VERSION}
docker push ${IMAGE}${NEW_VERSION}

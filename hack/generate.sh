#!/bin/bash

# Commands need to be run from project root
cd "$( dirname "${BASH_SOURCE[0]}" )"/..

operator-sdk generate k8s
operator-sdk generate crds

# This can be removed once the operator no longer needs to be run on
# OpenShift v3.11
yq d -i deploy/crds/pagerduty.openshift.io_pagerdutyintegrations_crd.yaml \
   spec.validation.openAPIV3Schema.type

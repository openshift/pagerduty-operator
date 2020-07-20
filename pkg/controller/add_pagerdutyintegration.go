package controller

import (
	"github.com/openshift/pagerduty-operator/pkg/controller/pagerdutyintegration"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, pagerdutyintegration.Add)
}

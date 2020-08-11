package error_manager

import (
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	statusOk                 = Status{status: "ok"}
	StatusInternalError      = Status{status: "internal-error"}
	StatusKubecallError      = Status{status: "kubecall-error"}
	StatusPagerdutycallError = Status{status: "pagerdutycall-error"}

	status_list = []Status{
		statusOk,
		StatusInternalError,
		StatusKubecallError,
		StatusPagerdutycallError,
	}

	log = logf.Log.WithName("error_manager")
)

// errorString is a trivial implementation of error.
type Status struct {
	status string
}

func (s Status) Error() string {
	return s.status
}

func (s *Status) IsEqual(s2 error) bool {
	status_error, ok := s2.(Status)
	if !ok {
		return false
	} else {
		return s.status == status_error.status
	}
}

func ToString(status error) (status_string string) {
	defer func() {
		// in case we are getting an unmanaged error, aggregating them under internal errors
		if r := recover(); r != nil {
			status_string = statusOk.status
		}
	}()

	status_string = StatusInternalError.status

	if status == nil {
		status_string = statusOk.status
	}

	for _, i := range status_list {
		if i.IsEqual(status) {
			status_string = i.status
		}
	}

	return
}

func ToStatus(in_status error, status Status) error {
	_, ok := in_status.(Status)

	if ok {
		return in_status
	}

	if in_status == nil {
		return nil
	} else {
		log.Error(in_status, "")
		return status
	}
}

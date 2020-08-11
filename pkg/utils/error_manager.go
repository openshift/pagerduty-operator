package utils

const (
	status := [...]string {
		"ok",
		"internal-error",
		"kubecall-error",
		"pagerdutycall-error"
	} 
)

func ToString(status interface{}) {
	return status[0]
}
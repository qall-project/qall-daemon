package object

type ResourceAssignment struct {
	ResourceProvider string
	ResourceNames    []string
	TaskHash         string
	WorkerHash       string
}

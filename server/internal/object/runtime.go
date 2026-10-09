package object

type RuntimeStatus string

const (
	StatusPending RuntimeStatus = "PENDING"
	StatusRunning RuntimeStatus = "RUNNING"
	StatusDone    RuntimeStatus = "DONE"
	StatusError   RuntimeStatus = "ERROR"
	StatusUnknown RuntimeStatus = "UNKNOWN"
)

type TaskRuntimeArgs struct {
	Payload              *TaskPayload
	TaskRunId            string
	HostBlockRegistryDir string
	TaskHash             string
	ArtifactHash         string
	Token                string
	WorkerAddresses      []string
	Name                 string
	EnvironmentVariables map[string]string
}

type WorkerRuntimeArgs struct {
	Payload              *WorkerPayload
	HostBlockRegistryDir string
	TaskRunId            string
	WorkerRunId          string
	WorkerHash           string
	Token                string
	Port                 string
	Name                 string
	EnvironmentVariables map[string]string
}

type WorkerRun struct {
	Id          string
	ContainerId string
	TaskRunId   string
	Status      RuntimeStatus
	Address     string
}

type TaskRun struct {
	Id          string
	Status      RuntimeStatus
	ContainerId string
}

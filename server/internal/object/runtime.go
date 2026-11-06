package object

type RuntimeStatus string

const (
	StatusPending RuntimeStatus = "PENDING"
	StatusRunning RuntimeStatus = "RUNNING"
	StatusDone    RuntimeStatus = "DONE"
	StatusError   RuntimeStatus = "ERROR"
	StatusUnknown RuntimeStatus = "UNKNOWN"
)

type DataloaderSpec struct {
	Name        string
	Src         string
	Dst         string
	Credentials map[string]string
}

// DataloaderEntry is a run-wide dataloader declaration as received from the
// client: it pairs a per-task spec with the task it belongs to. The daemon
// indexes the whole run's entries per task_hash so each started container only
// gets its own specs mounted.
type DataloaderEntry struct {
	TaskHash string
	DataloaderSpec
}

type TaskRuntimeArgs struct {
	Payload              *TaskPayload
	TaskRunId            string
	WorkflowRunId        string
	HostBlockRegistryDir string
	TaskHash             string
	ArtifactHash         string
	Token                string
	WorkerAddresses      []string
	Name                 string
	EnvironmentVariables map[string]string
	Dataloaders          []DataloaderSpec
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

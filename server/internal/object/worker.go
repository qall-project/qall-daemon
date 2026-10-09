package object

type WorkerPayload struct {
	Type               string
	CodeFormat         string
	Code               string
	Requirements       []string
	Image              string
	RuntimeQallVersion string
}

type WorkerEntry struct {
	WorkerProvider string
	InputFormat    string
	WorkerHash     string
}

package object

type WorkerPayload struct {
	Type               string
	CodeFormat         string
	Code               string
	Requirements       []string
	Image              string
	RuntimeQallVersion string
}

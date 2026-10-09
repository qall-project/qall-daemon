package object

type TaskPayload struct {
	Type                   string
	CodeFormat             string
	Code                   string
	Requirements           []string
	QuantumRunInputFormats []string
	Image                  string
	RuntimeQallVersion     string
}

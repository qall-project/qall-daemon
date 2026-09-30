package object

type WorkerPayload struct {
	Type         string
	CodeFormat   string
	Code         string
	Requirements []string
	Image        string
}

type WorkerEntry struct {
	Provider    string
	InputFormat string
	Hash        string
}

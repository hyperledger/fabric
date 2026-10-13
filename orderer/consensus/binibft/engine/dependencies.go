package engine

// Assembler creates proposals.
type Assembler interface {
	// AssembleProposal creates a proposal which includes
	// the given requests (when permitting) and metadata.
	AssembleProposal(metadata []byte, requests [][]byte) Proposal
}

type RequestInspector interface {
	// RequestID returns info about the given request.
	RequestID(req []byte) RequestInfo
}

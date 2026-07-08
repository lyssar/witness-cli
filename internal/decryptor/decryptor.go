package decryptor

import "context"

// Request describes one decryption unit.
type Request struct {
	OperationalID string
	KeyPath       string
	SourcePath    string
	TargetPath    string
}

// Decryptor defines the stable decryption contract for manifest secret entries.
type Decryptor interface {
	Name() string
	DecryptFile(ctx context.Context, request Request) error
}

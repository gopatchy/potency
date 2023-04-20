package potency

import (
	"crypto/sha256"
	"hash"
	"io"
)

type bodyIntercept struct {
	source io.ReadCloser
	sha256 hash.Hash
}

func newBodyIntercept(source io.ReadCloser) *bodyIntercept {
	return &bodyIntercept{
		source: source,
		sha256: sha256.New(),
	}
}

func (bi *bodyIntercept) Read(p []byte) (int, error) {
	numBytes, err := bi.source.Read(p)
	bi.sha256.Write(p[:numBytes])

	return numBytes, err
}

func (bi *bodyIntercept) Close() error {
	return bi.source.Close()
}

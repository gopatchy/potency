package potency

import (
	"bytes"
	"net/http"
)

type responseWriterIntercept struct {
	dest       http.ResponseWriter
	buf        bytes.Buffer
	statusCode int
}

func newResponseWriterIntercept(dest http.ResponseWriter) *responseWriterIntercept {
	return &responseWriterIntercept{
		dest:       dest,
		buf:        bytes.Buffer{},
		statusCode: http.StatusOK,
	}
}

func (rwi *responseWriterIntercept) Header() http.Header {
	return rwi.dest.Header()
}

func (rwi *responseWriterIntercept) Write(data []byte) (int, error) {
	rwi.buf.Write(data)
	return rwi.dest.Write(data)
}

func (rwi *responseWriterIntercept) WriteHeader(statusCode int) {
	rwi.statusCode = statusCode
	rwi.dest.WriteHeader(statusCode)
}

package potency

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gopatchy/jsrest"
)

type Potency struct {
	handler http.Handler

	// TODO: Expire based on time; probably use age-based linked list and delete at write time
	cache   map[string]*savedResult
	cacheMu sync.RWMutex

	inProgress   map[string]bool
	inProgressMu sync.Mutex
}

type savedResult struct {
	Method        string      `json:"method"`
	URL           string      `json:"url"`
	RequestHeader http.Header `json:"requestHeader"`
	Sha256        string      `json:"sha256"`

	StatusCode     int         `json:"statusCode"`
	ResponseHeader http.Header `json:"responseHeader"`
	ResponseBody   []byte      `json:"responseBody"`
}

var (
	ErrConflict       = errors.New("conflict")
	ErrMismatch       = errors.New("idempotency mismatch")
	ErrBodyMismatch   = fmt.Errorf("request body mismatch: %w", ErrMismatch)
	ErrMethodMismatch = fmt.Errorf("HTTP method mismatch: %w", ErrMismatch)
	ErrURLMismatch    = fmt.Errorf("URL mismatch: %w", ErrMismatch)
	ErrHeaderMismatch = fmt.Errorf("Header mismatch: %w", ErrMismatch)
	ErrInvalidKey     = errors.New("invalid Idempotency-Key")

	criticalHeaders = []string{
		"Accept",
		"Authorization",
	}
)

func NewPotency(handler http.Handler) *Potency {
	return &Potency{
		handler:    handler,
		cache:      map[string]*savedResult{},
		inProgress: map[string]bool{},
	}
}

func (p *Potency) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	val := r.Header.Get("Idempotency-Key")
	if val == "" {
		p.handler.ServeHTTP(w, r)
		return
	}

	err := p.serveHTTP(w, r, val)
	if err != nil {
		jsrest.WriteError(w, err)
	}
}

func (p *Potency) serveHTTP(w http.ResponseWriter, r *http.Request, val string) error {
	if len(val) < 2 || !strings.HasPrefix(val, `"`) || !strings.HasSuffix(val, `"`) {
		return jsrest.Errorf(jsrest.ErrBadRequest, "%s (%w)", val, ErrInvalidKey)
	}

	key := val[1 : len(val)-1]

	saved := p.read(key)

	if saved != nil {
		if r.Method != saved.Method {
			return jsrest.Errorf(jsrest.ErrBadRequest, "%s (%w)", r.Method, ErrMethodMismatch)
		}

		if r.URL.String() != saved.URL {
			return jsrest.Errorf(jsrest.ErrBadRequest, "%s (%w)", r.URL.String(), ErrURLMismatch)
		}

		for _, h := range criticalHeaders {
			if saved.RequestHeader.Get(h) != r.Header.Get(h) {
				return jsrest.Errorf(jsrest.ErrBadRequest, "%s: %s (%w)", h, r.Header.Get(h), ErrHeaderMismatch)
			}
		}

		h := sha256.New()

		_, err := io.Copy(h, r.Body)
		if err != nil {
			return jsrest.Errorf(jsrest.ErrBadRequest, "hash request body failed (%w)", err)
		}

		hexed := hex.EncodeToString(h.Sum(nil))
		if hexed != saved.Sha256 {
			return jsrest.Errorf(jsrest.ErrBadRequest, "%s vs %s (%w)", hexed, saved.Sha256, ErrBodyMismatch)
		}

		for key, vals := range saved.ResponseHeader {
			w.Header().Set(key, vals[0])
		}

		w.WriteHeader(saved.StatusCode)
		_, _ = w.Write(saved.ResponseBody)

		return nil
	}

	// Store miss, proceed to normal execution with interception
	err := p.lockKey(key)
	if err != nil {
		return jsrest.Errorf(jsrest.ErrConflict, "%s", key)
	}

	defer p.unlockKey(key)

	requestHeader := http.Header{}
	for _, h := range criticalHeaders {
		requestHeader.Set(h, r.Header.Get(h))
	}

	bi := newBodyIntercept(r.Body)
	r.Body = bi

	rwi := newResponseWriterIntercept(w)
	w = rwi

	p.handler.ServeHTTP(w, r)

	save := &savedResult{
		Method:        r.Method,
		URL:           r.URL.String(),
		RequestHeader: requestHeader,
		Sha256:        hex.EncodeToString(bi.sha256.Sum(nil)),

		StatusCode:     rwi.statusCode,
		ResponseHeader: rwi.Header(),
		ResponseBody:   rwi.buf.Bytes(),
	}

	p.write(key, save)

	return nil
}

func (p *Potency) lockKey(key string) error {
	p.inProgressMu.Lock()
	defer p.inProgressMu.Unlock()

	if p.inProgress[key] {
		return ErrConflict
	}

	p.inProgress[key] = true

	return nil
}

func (p *Potency) unlockKey(key string) {
	p.inProgressMu.Lock()
	defer p.inProgressMu.Unlock()

	delete(p.inProgress, key)
}

func (p *Potency) read(key string) *savedResult {
	p.cacheMu.RLock()
	defer p.cacheMu.RUnlock()

	return p.cache[key]
}

func (p *Potency) write(key string, sr *savedResult) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()

	p.cache[key] = sr
}

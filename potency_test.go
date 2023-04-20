package potency_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/dchest/uniuri"
	"github.com/go-resty/resty/v2"
	"github.com/gopatchy/potency"
	"github.com/stretchr/testify/require"
)

func TestGET(t *testing.T) {
	t.Parallel()

	ts := newTestServer(t)
	defer ts.shutdown(t)

	key1 := uniuri.New()

	resp, err := ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key1)).
		Get("")
	require.NoError(t, err)
	require.False(t, resp.IsError())
	require.Equal(t, "bar", resp.Header().Get("X-Response"))

	resp1 := resp.String()

	resp, err = ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key1)).
		Get("")
	require.NoError(t, err)
	require.False(t, resp.IsError())
	require.Equal(t, "bar", resp.Header().Get("X-Response"))
	require.Equal(t, resp1, resp.String())

	key2 := uniuri.New()

	resp, err = ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key2)).
		Get("")
	require.NoError(t, err)
	require.False(t, resp.IsError())
	require.Equal(t, "bar", resp.Header().Get("X-Response"))

	resp2 := resp.String()

	require.NotEqual(t, resp2, resp1)

	resp, err = ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key1)).
		Get("x")
	require.NoError(t, err)
	require.True(t, resp.IsError())

	resp, err = ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key1)).
		Delete("")
	require.NoError(t, err)
	require.True(t, resp.IsError())

	resp, err = ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key1)).
		SetHeader("Authorization", "Bearer xyz").
		Get("")
	require.NoError(t, err)
	require.True(t, resp.IsError())

	resp, err = ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key1)).
		SetHeader("Accept", "text/xml").
		Get("")
	require.NoError(t, err)
	require.True(t, resp.IsError())

	resp, err = ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key1)).
		SetHeader("X-Test", "foo").
		Get("")
	require.NoError(t, err)
	require.False(t, resp.IsError())
	require.Equal(t, "bar", resp.Header().Get("X-Response"))
	require.Equal(t, resp1, resp.String())
}

func TestPOST(t *testing.T) {
	t.Parallel()

	ts := newTestServer(t)
	defer ts.shutdown(t)

	key1 := uniuri.New()

	resp, err := ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key1)).
		SetBody("test1").
		Post("")
	require.NoError(t, err)
	require.False(t, resp.IsError())

	resp1 := resp.String()

	resp, err = ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key1)).
		SetBody("test1").
		Post("")
	require.NoError(t, err)
	require.False(t, resp.IsError())
	require.Equal(t, resp1, resp.String())

	resp, err = ts.r().
		SetHeader("Idempotency-Key", fmt.Sprintf(`"%s"`, key1)).
		SetBody("test2").
		Post("")
	require.NoError(t, err)
	require.True(t, resp.IsError())
}

type testServer struct {
	dir string
	srv *http.Server
	rst *resty.Client
}

func newTestServer(t *testing.T) *testServer {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	mux := http.NewServeMux()
	p := potency.NewPotency(mux)

	listener, err := net.Listen("tcp", "[::]:0")
	require.NoError(t, err)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		w.Header().Add("X-Response", "bar")

		_, err = w.Write([]byte(uniuri.New()))
		require.NoError(t, err)
	})

	srv := &http.Server{
		Handler:           p,
		ReadHeaderTimeout: 1 * time.Second,
	}

	go func() {
		_ = srv.Serve(listener)
	}()

	baseURL := fmt.Sprintf("http://[::1]:%d/", listener.Addr().(*net.TCPAddr).Port)

	rst := resty.New().
		SetHeader("Content-Type", "application/json").
		SetBaseURL(baseURL)

	return &testServer{
		dir: dir,
		srv: srv,
		rst: rst,
	}
}

func (ts *testServer) r() *resty.Request {
	return ts.rst.R()
}

func (ts *testServer) shutdown(t *testing.T) {
	err := ts.srv.Shutdown(context.Background())
	require.NoError(t, err)

	os.RemoveAll(ts.dir)
}

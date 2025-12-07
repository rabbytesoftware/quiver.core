package mocks

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
)

type RoundTripFunc func(*http.Request) (*http.Response, error)

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func NewMockHTTPClient(roundTripper http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: roundTripper,
	}
}

func NewTimeoutClient() *http.Client {
	return NewMockHTTPClient(RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	}))
}

func NewNetworkErrorClient() *http.Client {
	return NewMockHTTPClient(RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network unreachable")
	}))
}

func NewCustomErrorClient(err error) *http.Client {
	return NewMockHTTPClient(RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, err
	}))
}

func NewStatusCodeClient(statusCode int, body string) *http.Client {
	return NewMockHTTPClient(RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: statusCode,
			Status:     http.StatusText(statusCode),
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}))
}

func NewBadContentLengthClient() *http.Client {
	return NewMockHTTPClient(RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Body:          io.NopCloser(strings.NewReader("test data")),
			Header:        make(http.Header),
			ContentLength: -1,
			Request:       req,
		}
		resp.Header.Set("Content-Length", "invalid")
		return resp, nil
	}))
}

func NewHeadersClient(headers map[string]string, body string) *http.Client {
	return NewMockHTTPClient(RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    req,
		}
		for k, v := range headers {
			resp.Header.Set(k, v)
		}
		return resp, nil
	}))
}

type ErrorReadCloser struct {
	ReadErr error
}

func (e *ErrorReadCloser) Read(p []byte) (n int, err error) {
	if e.ReadErr != nil {
		return 0, e.ReadErr
	}
	return 0, io.EOF
}

func (e *ErrorReadCloser) Close() error {
	return nil
}

func NewReadErrorClient(readErr error) *http.Client {
	return NewMockHTTPClient(RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       &ErrorReadCloser{ReadErr: readErr},
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}))
}

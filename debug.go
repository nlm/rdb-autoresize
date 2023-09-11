package main

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
)

type loggingTransport struct{}

func (s *loggingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	reqBytes, _ := httputil.DumpRequestOut(r, true)
	resp, err := http.DefaultTransport.RoundTrip(r)
	respBytes, _ := httputil.DumpResponse(resp, true)
	slog.Debug(
		"http request",
		slog.String("request", string(reqBytes)),
		slog.String("response", string(respBytes)),
	)
	return resp, err
}

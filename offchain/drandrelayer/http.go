package main

import (
	"net/http"
	"time"
)

type httpClientWithTimeout struct {
	TimeoutSec int
}

func (h httpClientWithTimeout) Client() *http.Client {
	t := h.TimeoutSec
	if t <= 0 {
		t = 10
	}
	return &http.Client{Timeout: time.Duration(t) * time.Second}
}

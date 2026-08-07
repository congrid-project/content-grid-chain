package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublisherVerifyRejectsInvalidWalletBeforeFetchingHomepage(t *testing.T) {
	srv := &server{}
	request := httptest.NewRequest(
		http.MethodPost,
		"/publishers/verify",
		strings.NewReader(`{"domain":"example.com","wallet":"not-a-wallet"}`),
	)
	response := httptest.NewRecorder()

	srv.handlePublisherVerify().ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "invalid wallet address")
}

func TestPublisherVerifyRejectsInvalidDomainBeforeFetchingHomepage(t *testing.T) {
	srv := &server{}
	request := httptest.NewRequest(
		http.MethodPost,
		"/publishers/verify",
		strings.NewReader(`{"domain":"localhost","wallet":"congrid1candidate"}`),
	)
	response := httptest.NewRecorder()

	srv.handlePublisherVerify().ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "invalid domain format")
}

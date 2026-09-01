package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublisherRegisterAcceptsCongridWalletPrefix(t *testing.T) {
	templates, err := buildPageTemplates(siteFS)
	require.NoError(t, err)

	form := url.Values{
		"domain": {"example.com"},
		"wallet": {"congrid1fglanlkvqtyznlw3flu88680zmctyug8qr03pj"},
	}
	request := httptest.NewRequest(http.MethodPost, "/publishers/register", strings.NewReader(form.Encode()))
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	srv := &server{templates: templates}
	srv.handlePublisherRegister("https://congrid.net").ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "Server-side registration requires a signing key name")
	require.NotContains(t, response.Body.String(), "Invalid wallet address")
}

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

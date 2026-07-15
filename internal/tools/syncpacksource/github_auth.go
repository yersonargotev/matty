package main

import "net/http"

const githubAPIOrigin = "https://api.github.com"

type githubAuthorizationTransport struct {
	token string
	base  http.RoundTripper
}

func newAuthenticatedGitHubHTTPClient(token string, base http.RoundTripper) *http.Client {
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{Transport: githubAuthorizationTransport{token: token, base: base}}
}

func (transport githubAuthorizationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.Header.Del("Authorization")
	if transport.token != "" && copy.URL.Scheme+"://"+copy.URL.Host == githubAPIOrigin {
		copy.Header.Set("Authorization", "Bearer "+transport.token)
	}
	return transport.base.RoundTrip(copy)
}

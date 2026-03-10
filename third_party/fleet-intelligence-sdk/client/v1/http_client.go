// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package v1

import (
	"crypto/tls"
	"net/http"
)

func createDefaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

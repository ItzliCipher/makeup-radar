//go:build !fixtures

package main

import "net/http"

// testTransport is only replaced in test builds.
func testTransport() http.RoundTripper { return nil }

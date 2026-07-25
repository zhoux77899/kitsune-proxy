// Package auth validates the local API key and applies upstream auth policy.
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

var ErrInvalidAPIKey = errors.New("invalid API key")

// Method identifies which supported inbound authentication headers were used.
type Method string

const (
	MethodBearer Method = "bearer"
	MethodAPIKey Method = "x-api-key"
	MethodBoth   Method = "both"
)

// Authenticate validates all present supported credentials.
func Authenticate(header http.Header, expected string) (Method, error) {
	authorizationValues := header.Values("Authorization")
	apiKeyValues := header.Values("X-Api-Key")
	if len(authorizationValues) > 1 || len(apiKeyValues) > 1 {
		return "", ErrInvalidAPIKey
	}

	hasBearer := len(authorizationValues) == 1
	hasAPIKey := len(apiKeyValues) == 1
	if !hasBearer && !hasAPIKey {
		return "", ErrInvalidAPIKey
	}

	if hasBearer {
		fields := strings.Fields(authorizationValues[0])
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || !constantTimeEqual(fields[1], expected) {
			return "", ErrInvalidAPIKey
		}
	}
	if hasAPIKey && !constantTimeEqual(apiKeyValues[0], expected) {
		return "", ErrInvalidAPIKey
	}

	switch {
	case hasBearer && hasAPIKey:
		return MethodBoth, nil
	case hasBearer:
		return MethodBearer, nil
	default:
		return MethodAPIKey, nil
	}
}

// Apply removes the local credential and applies the selected upstream policy.
func Apply(header http.Header, method Method, mode, upstreamAPIKey string) {
	header.Del("Authorization")
	header.Del("X-Api-Key")
	if mode == "none" {
		return
	}

	if method == MethodBearer || method == MethodBoth {
		header.Set("Authorization", "Bearer "+upstreamAPIKey)
	}
	if method == MethodAPIKey || method == MethodBoth {
		header.Set("X-Api-Key", upstreamAPIKey)
	}
}

func constantTimeEqual(left, right string) bool {
	leftHash := sha256.Sum256([]byte(left))
	rightHash := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}

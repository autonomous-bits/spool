package repository

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

var (
	// ErrInvalidListBudget reports a negative row or response-byte limit.
	ErrInvalidListBudget = errors.New("list query budget is invalid")
	// ErrResponseBudgetTooSmall reports a byte budget unable to represent a result.
	// The limit applies to the repository payload only; public adapters must reserve
	// space for their envelopes before invoking repository queries.
	ErrResponseBudgetTooSmall = errors.New("response budget cannot represent result")
)

type continuationToken struct {
	Fingerprint string `json:"fingerprint"`
	Offset      int    `json:"offset"`
}

const maxContinuationTokenBytes = 4096

func queryFingerprint(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func encodeContinuation(fingerprint string, offset int) string {
	data, _ := json.Marshal(continuationToken{Fingerprint: fingerprint, Offset: offset})
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeContinuation(token, fingerprint string) (int, error) {
	if token == "" {
		return 0, nil
	}
	if len(token) > maxContinuationTokenBytes {
		return 0, ErrInvalidContinuation
	}
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, ErrInvalidContinuation
	}
	var decoded continuationToken
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		decoded.Fingerprint != fingerprint || decoded.Offset <= 0 {
		return 0, ErrInvalidContinuation
	}
	return decoded.Offset, nil
}

func resultFits(result any, maxBytes int) bool {
	if maxBytes == 0 {
		return true
	}
	data, err := json.Marshal(result)
	return err == nil && len(data) <= maxBytes
}

package requestid

import (
	"crypto/rand"
	"encoding/base64"
)

type RequestID struct {
	str string
}

func NewRequestID() RequestID {
	b := make([]byte, 6)
	rand.Read(b)
	return RequestID{base64.RawURLEncoding.EncodeToString(b)}
}

func (id RequestID) String() string {
	return id.str
}

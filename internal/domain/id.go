package domain

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"
)

// ID identifies an entity. Values are UUIDv7 strings: they sort by creation
// time, which gives cheap keyset pagination (WHERE id < $cursor) and keeps
// B-tree inserts append-only in Postgres.
type ID string

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// NewID returns a fresh UUIDv7 (RFC 9562) built from the standard library.
func NewID() ID {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixMilli())<<16)
	if _, err := rand.Read(b[6:]); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x70 // version 7
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant

	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:], b[10:])
	return ID(out[:])
}

// ParseID validates the canonical textual form.
func ParseID(s string) (ID, error) {
	if !uuidPattern.MatchString(s) {
		return "", NewValidationError("id", CodeInvalidFmt, "must be a UUID")
	}
	return ID(s), nil
}

func (id ID) String() string { return string(id) }

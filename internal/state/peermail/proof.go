package peermail

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Proof is the MAC of a message under the secret of one pairing; empty when there
// is no secret. It covers every field the receiver acts on.
func Proof(secret []byte, m Message) string {
	if len(secret) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, secret)
	for _, part := range []string{m.ID, m.From.SessionID, m.To.SessionID, strconv.FormatInt(m.SentAt.UnixNano(), 10), strconv.Itoa(m.Hops), strconv.FormatUint(m.Seq, 10), cleanBody(m.Body)} {
		mac.Write([]byte(strconv.Itoa(len(part))))
		mac.Write([]byte{':'})
		mac.Write([]byte(part))
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// ProofValid reports whether m carries the proof its secret makes. Without a
// secret nothing is valid.
func ProofValid(secret []byte, m Message) bool {
	want := Proof(secret, m)
	if want == "" || m.Proof == "" {
		return false
	}
	return hmac.Equal([]byte(want), []byte(m.Proof))
}

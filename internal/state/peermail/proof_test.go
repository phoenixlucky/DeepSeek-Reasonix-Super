package peermail

import (
	"testing"
	"time"
)

func proofMessage() Message {
	return Message{ID: "m-1", From: Party{SessionID: "s-a"}, To: Party{SessionID: "s-b"}, SentAt: time.Unix(100, 7).UTC(), Body: "do the thing"}
}

func TestAProofHoldsOnlyForTheMessageAndTheSecretItWasMadeFor(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	m := proofMessage()
	m.Proof = Proof(secret, m)
	if !ProofValid(secret, m) {
		t.Fatal("a message does not verify under the secret that signed it")
	}
	for name, mutate := range map[string]func(*Message){
		"body":   func(m *Message) { m.Body += "!" },
		"sender": func(m *Message) { m.From.SessionID = "s-x" },
		"target": func(m *Message) { m.To.SessionID = "s-x" },
		"id":     func(m *Message) { m.ID = "m-2" },
		"time":   func(m *Message) { m.SentAt = m.SentAt.Add(time.Nanosecond) },
		"hops":   func(m *Message) { m.Hops++ },
		"seq":    func(m *Message) { m.Seq++ },
	} {
		forged := m
		mutate(&forged)
		if ProofValid(secret, forged) {
			t.Fatalf("a message with a changed %s still verifies", name)
		}
	}
	if ProofValid([]byte("another secret another secret!!!"), m) || ProofValid(nil, m) {
		t.Fatal("a proof verifies under a secret that did not make it")
	}
}

func TestAMessageWithNoProofVerifiesUnderNoSecret(t *testing.T) {
	m := proofMessage()
	if ProofValid([]byte("s"), m) || ProofValid(nil, m) {
		t.Fatal("an unsigned message verifies")
	}
	if Proof(nil, m) != "" {
		t.Fatal("a proof exists without a secret")
	}
}

func TestAProofSurvivesTheBodyCleaningTheSpoolDoes(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	m := proofMessage()
	m.Body = "line\x00 one‮"
	m.Proof = Proof(secret, m)
	cleaned := m
	cleaned.Body = cleanBody(m.Body)
	if !ProofValid(secret, cleaned) {
		t.Fatal("the proof does not cover the body the receiver reads")
	}
}

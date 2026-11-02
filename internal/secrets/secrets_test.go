package secrets_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"muara.bi/desktop/internal/secrets"
)

// The words the bundle's credential check looks for — a sealed block's own
// bytes must not spell any of them either, see TestSealedBytesSpellNoCredentialWord.
var credentialWords = []string{
	"password", "passwd", "token", "secret", "credential", "api_key", "apikey", "dsn", "sasl", "keyfile", "private_key",
}

// payload is what an export with the credential toggle on would seal: the
// credential words themselves, to make the no-plaintext-leak tests load-bearing.
var payload = []byte("alias=sales user=analyst password=super-token-123 secret credential dsn=postgres://x apikey=abc")

func TestRoundTrip(t *testing.T) {
	blob, err := secrets.Encrypt(payload, "kata sandi")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := secrets.Decrypt(blob, "kata sandi")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("plaintext = %q, want %q", got, payload)
	}
}

func TestWrongPassphraseIsDistinct(t *testing.T) {
	blob, err := secrets.Encrypt(payload, "benar")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := secrets.Decrypt(blob, "salah"); !errors.Is(err, secrets.ErrWrongPassphrase) {
		t.Errorf("error = %v, want ErrWrongPassphrase — the caller has to be able to say \"kata sandi salah\"", err)
	}
}

func TestTamperedBlockLooksLikeWrongPassphrase(t *testing.T) {
	blob, err := secrets.Encrypt(payload, "benar")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// Flip the first hex nibble inside the ciphertext: GCM must refuse it, and
	// it must not pretend to know whether the passphrase or the bytes are wrong.
	marker := []byte(`"data": "`)
	at := bytes.Index(blob, marker)
	if at < 0 {
		t.Fatal("tamper could not find the ciphertext — the envelope layout moved")
	}
	tampered := append([]byte(nil), blob...)
	start := at + len(marker)
	if tampered[start] == 'f' {
		tampered[start] = '0'
	} else {
		tampered[start] = 'f'
	}
	if _, err := secrets.Decrypt(tampered, "benar"); !errors.Is(err, secrets.ErrWrongPassphrase) {
		t.Errorf("error = %v, want ErrWrongPassphrase", err)
	}
}

func TestTwoSealsOfTheSameInputDiffer(t *testing.T) {
	first, err := secrets.Encrypt(payload, "benar")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	second, err := secrets.Encrypt(payload, "benar")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("two seals of the same input are identical — a reused salt or nonce would break the block")
	}
	for i, blob := range [][]byte{first, second} {
		got, err := secrets.Decrypt(blob, "benar")
		if err != nil {
			t.Fatalf("decrypt %d: %v", i, err)
		}
		if !bytes.Equal(got, payload) {
			t.Errorf("seal %d opened to %q, want %q", i, got, payload)
		}
	}
}

func TestBlankPassphraseRefused(t *testing.T) {
	if _, err := secrets.Encrypt(payload, "  "); err == nil {
		t.Fatal("a blank passphrase sealed a block")
	}
}

// The bundle's exit criterion is "grep the bundle, find no credential string".
// Hex is what makes this deterministic: 0-9a-f cannot spell any of the words,
// so the property holds for every block ever sealed, not for the one drawn today.
func TestSealedBytesSpellNoCredentialWord(t *testing.T) {
	blob, err := secrets.Encrypt(payload, "benar")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	lowered := strings.ToLower(string(blob))
	for _, word := range credentialWords {
		if strings.Contains(lowered, word) {
			t.Errorf("sealed block spells %q:\n%s", word, blob)
		}
	}
}

func TestCheckAcceptsWhatEncryptWrote(t *testing.T) {
	blob, err := secrets.Encrypt(payload, "benar")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := secrets.Check(blob); err != nil {
		t.Errorf("check: %v", err)
	}
}

func TestCheckRefusesForeignBlocks(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(map[string]any)
		extra string
	}{
		{"not JSON", nil, "not-a-secret-in-this-build"},
		{"foreign format", func(m map[string]any) { m["format"] = "muara.bundle.secrets" }, ""},
		{"newer version", func(m map[string]any) { m["version"] = 2 }, ""},
		{"older version", func(m map[string]any) { m["version"] = 0 }, ""},
		{"foreign kdf", func(m map[string]any) { m["kdf"] = "pbkdf2" }, ""},
		{"foreign cipher", func(m map[string]any) { m["cipher"] = "chacha20-poly1305" }, ""},
		{"no threads", func(m map[string]any) { m["threads"] = 0 }, ""},
		{"too many rounds", func(m map[string]any) { m["time"] = 1000 }, ""},
		{"memory below the argon2 floor", func(m map[string]any) { m["memory"] = 8 }, ""},
		{"wrong salt size", func(m map[string]any) { m["salt"] = "ff" }, ""},
		{"non-hex data", func(m map[string]any) { m["data"] = "zz" }, ""},
		{"data too short for a GCM tag", func(m map[string]any) { m["data"] = "00" }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blob, err := secrets.Encrypt(payload, "benar")
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			if tc.extra != "" {
				blob = []byte(tc.extra)
			} else {
				blob = mutateEnvelope(t, blob, tc.mut)
			}
			if err := secrets.Check(blob); !errors.Is(err, secrets.ErrNotSealed) {
				t.Errorf("error = %v, want ErrNotSealed", err)
			}
		})
	}
}

func TestUnknownEnvelopeFieldsAreTolerated(t *testing.T) {
	blob, err := secrets.Encrypt(payload, "benar")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	blob = mutateEnvelope(t, blob, func(m map[string]any) { m["future_field"] = true })
	if _, err := secrets.Decrypt(blob, "benar"); err != nil {
		t.Errorf("a block with an unknown field did not open: %v", err)
	}
}

// mutateEnvelope rewrites the JSON envelope through a map, the way a newer app
// or a hand-edited file would change it.
func mutateEnvelope(t *testing.T, blob []byte, mut func(map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(blob, &doc); err != nil {
		t.Fatalf("envelope is not JSON: %v", err)
	}
	mut(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	return out
}

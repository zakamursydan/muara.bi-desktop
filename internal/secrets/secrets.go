// Package secrets seals the credential block of a .muara bundle (BRD §6).
//
// The block is the only place a credential may travel in a bundle, and it only
// travels when the exporter chose the opt-in passphrase path — Save never writes
// it, SaveWithSecrets does. The shape is a small JSON envelope: the KDF
// parameters stay readable by hand from the file, and the ciphertext rides as
// hex. Hex is deliberate rather than an accident of taste: its alphabet (0-9a-f)
// cannot spell any of the words the bundle's "no credential string" check looks
// for, so that property holds by construction instead of by luck of the draw
// (base64 output can randomly contain "dsn" or "sasl").
//
// The KDF is argon2id with this app's parameters, and it is new code — the web
// product's cryptohelper is AES-256-GCM with a key from the environment, which
// is not a passphrase KDF at all.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	// formatV1 is the only value this app writes and reads in the envelope's
	// format field, mirroring the role of bundle.Format.
	formatV1 = "muara.bundle.sealed"

	// version is the envelope layout this app writes.
	version = 1

	kdfArgon2id = "argon2id"
	cipherGCM   = "aes-256-gcm"

	// The parameters this app seals with. They are written into every envelope,
	// so raising them later does not orphan a block sealed today.
	kdfTime    = 3
	kdfMemory  = 64 * 1024 // KiB handed to argon2id
	kdfThreads = 4

	keySize   = 32 // AES-256
	saltSize  = 16
	nonceSize = 12 // GCM's standard nonce size
)

var (
	// ErrNotSealed says the bytes are not a credential block this app can read:
	// not JSON, a foreign format or version, or parameters outside what the
	// decoder is willing to spend KDF work on.
	ErrNotSealed = errors.New("secrets: not a sealed credential block")

	// ErrWrongPassphrase is what a wrong passphrase and a damaged block both
	// look like. GCM cannot tell the two apart, and pretending it could would
	// send the user to the wrong door.
	ErrWrongPassphrase = errors.New("secrets: passphrase does not match")
)

// envelope is the JSON document inside the secrets.enc entry. Every binary
// field is hex; see the package comment for why that is load-bearing.
type envelope struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	KDF     string `json:"kdf"`
	Cipher  string `json:"cipher"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	Salt    string `json:"salt"`
	Nonce   string `json:"nonce"`
	Data    string `json:"data"`
}

// Encrypt seals plaintext with passphrase: a fresh random salt goes through
// argon2id into a 32-byte key, and AES-256-GCM does the sealing under a fresh
// random nonce. Two seals of the same input never produce the same bytes — a
// reused salt or nonce would break the very property the block exists for.
func Encrypt(plaintext []byte, passphrase string) ([]byte, error) {
	if strings.TrimSpace(passphrase) == "" {
		return nil, fmt.Errorf("secrets: a blank passphrase cannot protect a credential block")
	}
	salt := make([]byte, saltSize)
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("secrets: draw salt: %w", err)
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secrets: draw nonce: %w", err)
	}

	key := argon2.IDKey([]byte(passphrase), salt, kdfTime, kdfMemory, kdfThreads, keySize)
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	sealed := aead.Seal(nil, nonce, plaintext, nil)

	env := envelope{
		Format:  formatV1,
		Version: version,
		KDF:     kdfArgon2id,
		Cipher:  cipherGCM,
		Time:    kdfTime,
		Memory:  kdfMemory,
		Threads: kdfThreads,
		Salt:    hex.EncodeToString(salt),
		Nonce:   hex.EncodeToString(nonce),
		Data:    hex.EncodeToString(sealed),
	}
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("secrets: encode envelope: %w", err)
	}
	return append(data, '\n'), nil
}

// Decrypt opens a blob made by Encrypt. A wrong passphrase and a damaged block
// both come back as ErrWrongPassphrase.
func Decrypt(blob []byte, passphrase string) ([]byte, error) {
	env, err := parse(blob)
	if err != nil {
		return nil, err
	}
	salt, nonce, sealed, err := decodeFields(env)
	if err != nil {
		return nil, err
	}

	key := argon2.IDKey([]byte(passphrase), salt, env.Time, env.Memory, env.Threads, keySize)
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: either this is the wrong passphrase or the block was damaged",
			ErrWrongPassphrase)
	}
	return plain, nil
}

// Check says whether blob is a credential block this app could open, without
// spending the KDF work — that costs hundreds of milliseconds on purpose, and
// Open must be able to tell a readable bundle apart from a broken block cheaply.
func Check(blob []byte) error {
	env, err := parse(blob)
	if err != nil {
		return err
	}
	_, _, _, err = decodeFields(env)
	return err
}

func parse(blob []byte) (envelope, error) {
	var env envelope
	if err := json.Unmarshal(blob, &env); err != nil {
		return env, fmt.Errorf("%w: %v", ErrNotSealed, err)
	}
	// Unmarshalling tolerates unknown keys, in the same way an entity entry does:
	// a field this app has never heard of is not a reason to refuse the block.
	if env.Format != formatV1 {
		return env, fmt.Errorf("%w: format %q, this app reads %q", ErrNotSealed, env.Format, formatV1)
	}
	if env.Version != version {
		if env.Version > version {
			return env, fmt.Errorf("%w: version %d, this app understands up to %d — update the app to read it",
				ErrNotSealed, env.Version, version)
		}
		return env, fmt.Errorf("%w: version %d, this app reads version %d", ErrNotSealed, env.Version, version)
	}
	if env.KDF != kdfArgon2id {
		return env, fmt.Errorf("%w: kdf %q, this app reads %q", ErrNotSealed, env.KDF, kdfArgon2id)
	}
	if env.Cipher != cipherGCM {
		return env, fmt.Errorf("%w: cipher %q, this app reads %q", ErrNotSealed, env.Cipher, cipherGCM)
	}
	// The decoder is willing to spend only a bounded amount of KDF work on a
	// block this machine did not write; argon2id itself refuses less than 8 KiB
	// per thread.
	if env.Threads < 1 || env.Threads > 16 {
		return env, fmt.Errorf("%w: threads %d, this app reads 1-16", ErrNotSealed, env.Threads)
	}
	if env.Time < 1 || env.Time > 10 {
		return env, fmt.Errorf("%w: time %d, this app reads 1-10", ErrNotSealed, env.Time)
	}
	if env.Memory < 8*uint32(env.Threads) || env.Memory > 1<<20 {
		return env, fmt.Errorf("%w: memory %d KiB, this app reads %d-1048576",
			ErrNotSealed, env.Memory, 8*uint32(env.Threads))
	}
	return env, nil
}

// decodeFields turns the hex fields back into the lengths the cipher needs, so
// a block with the right words but the wrong sizes is refused before any KDF
// work rather than after it.
func decodeFields(env envelope) (salt, nonce, sealed []byte, err error) {
	if salt, err = decodeHex(env.Salt, saltSize); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: salt: %v", ErrNotSealed, err)
	}
	if nonce, err = decodeHex(env.Nonce, nonceSize); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: nonce: %v", ErrNotSealed, err)
	}
	if sealed, err = decodeHex(env.Data, -1); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: data: %v", ErrNotSealed, err)
	}
	// GCM's tag is 16 bytes; anything shorter cannot be opened, and saying so
	// here keeps the error a shape problem instead of a passphrase problem.
	if len(sealed) < 16 {
		return nil, nil, nil, fmt.Errorf("%w: data is %d bytes, too short to carry a GCM tag", ErrNotSealed, len(sealed))
	}
	return salt, nonce, sealed, nil
}

func decodeHex(field string, wantLen int) ([]byte, error) {
	out, err := hex.DecodeString(field)
	if err != nil {
		return nil, fmt.Errorf("not hex: %v", err)
	}
	if wantLen >= 0 && len(out) != wantLen {
		return nil, fmt.Errorf("%d bytes, want %d", len(out), wantLen)
	}
	return out, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: %w", err)
	}
	return cipher.NewGCM(block)
}

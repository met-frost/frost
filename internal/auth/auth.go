package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"gitlab.met.no/frost/frost/internal/common"
)

// General note about how tokens is currently used:
//
// Given that 1) communication is always over HTTPS (thus eliminating risk of
// eavesdropping), and 2) the key is distributed to the client in a secret way
// (typically in some manual copy&paste way), the client could have authorized itself
// by sending the key directly in a request header without decrypting/encrypting.
// However, by passing the encrypted token, we achieve an extra layer of security.
// Besides, we would then have the future option of sending useful/important information
// in the token instead of just a dummy string.

// --- BEGIN --- code based on
//     https://www.thepolyglotdeveloper.com/2018/02/ \
//         encrypt-decrypt-data-golang-application-crypto-packages/

func createHash(key string) string {
	hasher := md5.New()
	hasher.Write([]byte(key))
	return hex.EncodeToString(hasher.Sum(nil))
}

func encrypt(data []byte, passphrase string) ([]byte, error) {
	block, _ := aes.NewCipher([]byte(createHash(passphrase)))

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return []byte{}, fmt.Errorf("cipher.NewGCM() failed: %v", err)
	}

	nonce := make([]byte, gcm.NonceSize())

	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return []byte{}, fmt.Errorf("io.ReadFull() failed: %v", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	return ciphertext, nil
}

func decrypt(data []byte, passphrase string) ([]byte, error) {
	key := []byte(createHash(passphrase))

	block, err := aes.NewCipher(key)
	if err != nil {
		return []byte{}, fmt.Errorf("aes.NewCipher() failed: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return []byte{}, fmt.Errorf("cipher.NewGCM() failed: %v", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return []byte{}, fmt.Errorf(
			"data too small (expected at least %d bytes; got %d)", nonceSize, len(data))
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return []byte{}, fmt.Errorf("gcm.Open() failed: %v", err)
	}

	return plaintext, nil
}

// --- END --- code based on
//     https://www.thepolyglotdeveloper.com/2018/02/ \
//         encrypt-decrypt-data-golang-application-crypto-packages/

// ExtractToken extracts token from request header hdrName.
// Returns ("", ..., ..., nil) if no such header exists, otherwise returns
// (Base64-decoded token, Base64-encoded token, ..., nil) upon success,
// or (..., ..., HTTP status code, error) upon error.
func ExtractToken(request *http.Request, hdrName string) (string, []byte, int, error) {
	tokenB64Enc := request.Header.Get(hdrName)
	if tokenB64Enc == "" {
		return "", []byte{}, -1, nil
	}

	tokenB64Dec, err := base64.URLEncoding.DecodeString(tokenB64Enc)
	if err != nil {
		return "", []byte{}, http.StatusBadRequest, fmt.Errorf(
			"base64.URLEncoding.DecodeString() failed: %v (can happen if request "+
				"header %s is malformed/invalid)", err, hdrName)
	}

	return tokenB64Enc, tokenB64Dec, -1, nil
}

// authorized decides if a request is authorized with respect to an encryption key, an
// encrypted token, and a reference plaintext.
// key is the encryption key, hdrName is the name of the request header containing the encrypted
// candidate token, and refWord is used as the plaintext to be matched against the decrypted
// candidate token.
// Upon success, the function returns (bool, token, nil), otherwise (false, token, error).
// The returned token is the encrypted, Base64-encoded token, if available at that point,
// otherwise "".
func authorized(key, hdrName, refWord string, request *http.Request) (bool, string, error) {
	tokenB64Enc, tokenB64Dec, _, err := ExtractToken(request, "X-Frost-Writetoken")
	if err != nil {
		return false, "", fmt.Errorf("ExtractToken() failed: %v", err)
	}

	if tokenB64Enc == "" { // no header => unauthorized
		return false, "", nil
	}

	plainToken, err := decrypt(tokenB64Dec, key)
	if err != nil { // unauthorized
		return false, tokenB64Enc, fmt.Errorf(
			"decrypt() failed: %v (can happen if request header %s is absent or invalid)",
			err, hdrName)
	}

	if string(plainToken) == refWord {
		return true, tokenB64Enc, nil // decrypted token matches reference word => authorized
	}

	// decrypted token does not match reference word => unauthorized
	return false, tokenB64Enc, nil
}

// Reference word used for read authorization. Note: currently this word can be anything
// (even an empty string).
var readAuthRefWord = "dummy"

// ExemptedFromReadRestriction checks if a request is exempted from a read restriction by matching
// any X-Frost-Readtoken in the request header with readAuthRefWord and restrToken.
// If both match, the request is exempted.
// The request is also trivially exempted if the service was not initialized with a read key.
// Returns (bool, nil) upon success, otherwise (false, error).
func ExemptedFromReadRestriction(request *http.Request, restrToken string) (bool, error) {
	readKey := common.Getenv("READKEY", "")

	// STEP 1: consider the request trivially authorized if READKEY isn't set
	// (this can be practical in a development/test situation where security is usually
	// of no concern)
	if readKey == "" {
		return true, nil
	}

	// STEP 2: check if the request is authorized in general
	auth1, token, err := authorized(readKey, "X-Frost-Readtoken", readAuthRefWord, request)
	if (!auth1) || (err != nil) {
		return false, err
	}

	// STEP 3: final authorization ok iff the request token matches restrToken
	return token == restrToken, nil
}

// Reference word used for write authorization. Note: currently this word can be anything
// (even an empty string).
var writeAuthRefWord = "dummy"

// ValidateWriteToken checks if token 1) can be decrypted using writeKey as passphrase,
// and 2) matches a reference word. hdrName is the name of the request header from which the token
// was extracted.
// Returns nil upon success, otherwise error.
func ValidateWriteToken(token []byte, writeKey, hdrName string) error {
	plainToken, err := decrypt(token, writeKey)
	if err != nil {
		return fmt.Errorf(
			"decrypt() failed: %v (can happen if request header %s is absent or invalid)",
			err, hdrName)
	}

	if string(plainToken) != writeAuthRefWord {
		return fmt.Errorf("decrypted token doesn't match reference word")
	}

	return nil
}

// createToken generates a token to be used for authorization.
// The environment variable envKey is used as the encryption key and refWord is used
// as the plaintext to be encrypted.
// Returns (token, nil) upon success, otherwise ("", error).
func createToken(envKey, refWord string) (string, error) {
	key := common.Getenv(envKey, "")
	if key == "" {
		return "", fmt.Errorf("missing environment variable: %s", envKey)
	}
	token, err := encrypt([]byte(refWord), key)
	if err != nil {
		return "", fmt.Errorf("encrypt() failed: %v", err)
	}
	return base64.URLEncoding.EncodeToString(token), nil
}

// CreateReadToken generates a read token that can be used for being exempted from
// time series read access rules (defined in TSREADACCESS).
// Returns (read token, nil) upon success, otherwise ("", error).
func CreateReadToken() (string, error) {
	return createToken("READKEY", readAuthRefWord)
}

// CreateWriteToken generates a write token that can be used for authorizing write operations.
// Returns (write token, nil) upon success, otherwise ("", error).
func CreateWriteToken() (string, error) {
	return createToken("WRITEKEY", writeAuthRefWord)
}

// OAuthCreds ... (TODO: add documentation)
type OAuthCreds struct {
	AccessToken  string
	RefreshToken string
}

// GetRequestMetadata ... (TODO: add documentation)
func (creds *OAuthCreds) GetRequestMetadata(
	ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + creds.AccessToken,
	}, nil
}

// RequireTransportSecurity ... (TODO: add documentation)
func (creds *OAuthCreds) RequireTransportSecurity() bool {
	return true
}

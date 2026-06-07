package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/types"
)

// PackFile returns a full-length HMAC-SHA256 hex string that uniquely and
// securely identifies a file. The BOT_TOKEN is used as the HMAC secret so
// that only this server can produce valid hashes — an attacker who knows the
// message ID cannot construct a valid hash without also knowing the token.
//
// Both the bot (when generating share/stream links) and the HTTP stream route
// (when verifying the ?hash= query param) call this function, so upgrading it
// here automatically secures every link the bot produces and every request the
// server accepts — no other file needs to change.
func PackFile(fileName string, fileSize int64, mimeType string, fileID int64) string {
	// Build the same deterministic plaintext as before so existing logic is
	// easy to follow: concatenate all fields in a fixed order.
	h := &types.HashableFileStruct{
		FileName: fileName,
		FileSize: fileSize,
		MimeType: mimeType,
		FileID:   fileID,
	}
	plaintext := h.Pack() // MD5 hex of the fields — used as HMAC input

	// HMAC-SHA256 over that plaintext, keyed with BOT_TOKEN.
	mac := hmac.New(sha256.New, []byte(config.ValueOf.BotToken))
	mac.Write([]byte(plaintext))
	return hex.EncodeToString(mac.Sum(nil)) // 64-char hex string
}

// GetShortHash truncates the full 64-char HMAC-SHA256 hex to HashLength chars.
// HashLength defaults to 16 (was 6) giving 64 bits of security instead of 24.
func GetShortHash(fullHash string) string {
	return fullHash[:config.ValueOf.HashLength]
}

// CheckHash verifies that inputHash equals the short hash of expectedHash.
// Called identically by both the bot commands and the HTTP stream route.
func CheckHash(inputHash string, expectedHash string) bool {
	return inputHash == GetShortHash(expectedHash)
}

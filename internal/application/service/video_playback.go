package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// EncryptVideoPlaybackGrant keeps bearer credentials out of plaintext Redis.
// The key follows the same process secret used to validate access JWTs.
func EncryptVideoPlaybackGrant(plain []byte) ([]byte, error) {
	key := sha256.Sum256([]byte("weknora-video-playback:" + getJwtSecret()))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func DecryptVideoPlaybackGrant(encoded []byte) ([]byte, error) {
	key := sha256.Sum256([]byte("weknora-video-playback:" + getJwtSecret()))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(encoded) < gcm.NonceSize() {
		return nil, errors.New("invalid video playback grant")
	}
	return gcm.Open(nil, encoded[:gcm.NonceSize()], encoded[gcm.NonceSize():], nil)
}

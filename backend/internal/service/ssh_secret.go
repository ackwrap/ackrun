package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ackwrap/ackrun/internal/paths"
)

const sshSecretKeySize = 32

var ErrSSHSecretKeyUnavailable = errors.New("SSH secret key is unavailable")

type secretCipher struct {
	aead cipher.AEAD
}

type sshSecretCipher = secretCipher

func loadOrCreateSSHSecretCipher(p *paths.Paths, allowCreate bool) (*sshSecretCipher, error) {
	return loadOrCreateSecretCipher(p.SSHSecretKeyPath(), allowCreate, ErrSSHSecretKeyUnavailable)
}

func loadOrCreateSecretCipher(path string, allowCreate bool, unavailable error) (*secretCipher, error) {
	key, err := loadOrCreateSecretKey(path, allowCreate)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", unavailable, err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: initialize cipher", unavailable)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: initialize AEAD", unavailable)
	}
	return &secretCipher{aead: aead}, nil
}

func loadOrCreateSecretKey(path string, allowCreate bool) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != sshSecretKeySize {
			return nil, errors.New("secret key has invalid length")
		}
		if err := os.Chmod(path, 0600); err != nil {
			return nil, fmt.Errorf("protect secret key: %w", err)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	if !allowCreate {
		return nil, errors.New("secret key is missing while encrypted credentials exist")
	}
	key = make([]byte, sshSecretKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate secret key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return loadOrCreateSecretKey(path, allowCreate)
	}
	if err != nil {
		return nil, fmt.Errorf("create secret key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write secret key: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("sync secret key: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close secret key: %w", err)
	}
	return key, nil
}

func (c *secretCipher) encrypt(context, field string, plaintext []byte) ([]byte, []byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	sealed := c.aead.Seal(nil, nonce, plaintext, []byte(context+":"+field+":1"))
	return sealed, nonce, nil
}

func (c *secretCipher) decrypt(context, field string, ciphertext, nonce []byte) ([]byte, error) {
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(context+":"+field+":1"))
	if err != nil {
		return nil, errors.New("decrypt credential failed")
	}
	return plaintext, nil
}

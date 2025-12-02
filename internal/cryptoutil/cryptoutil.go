// Package cryptoutil предоставляет функции для генерации, сохранения, загрузки и использования RSA-ключей,
// а также гибридное шифрование данных с помощью алгоритмов AES и RSA.
package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/levinOo/go-metrics-project/internal/config"
)

// EnsureKeypair проверяет наличие ключевой пары RSA по пути, заданному в cfg.CryptoKeyPath.
// Если ключи отсутствуют, генерирует пару и сохраняет в файлы private.pem и public.pem.
func EnsureKeypair(cfg config.Config) error {
	if cfg.CryptoKeyPath == "" {
		return nil
	}

	dir := filepath.Dir(cfg.CryptoKeyPath)

	privateKeyPath := filepath.Join(dir, "private.pem")
	publicKeyPath := filepath.Join(dir, "public.pem")

	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		if err := GenerateAndSaveKeypair(privateKeyPath, publicKeyPath); err != nil {
			return fmt.Errorf("failed to generate keypair: %w", err)
		}
	}

	return nil
}

// GenerateAndSaveKeypair генерирует новую пару RSA-ключей (2048 бит) и сохраняет их
// в файлы по заданному пути для приватного и публичного ключа в формате PEM.
// Приватный ключ сохраняется как "RSA PRIVATE KEY" (PKCS#1), публичный — "PUBLIC KEY".
func GenerateAndSaveKeypair(privateKeyPath, publicKeyPath string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})
	if err := os.WriteFile(privateKeyPath, privateKeyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write private key to %s: %w", privateKeyPath, err)
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})
	if err := os.WriteFile(publicKeyPath, publicKeyPEM, 0644); err != nil {
		return fmt.Errorf("failed to write public key to %s: %w", publicKeyPath, err)
	}

	return nil
}

// LoadPrivateKey загружает RSA-приватный ключ из PEM-файла (PKCS#1 или PKCS#8).
// Возвращает *rsa.PrivateKey, либо ошибку, если не удаётся декодировать или распознать ключ.
func LoadPrivateKey(privateKeyPath string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block for private key")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	rsaKey, ok := priv.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return rsaKey, nil
}

// LoadPublicKey загружает RSA-публичный ключ из PEM-файла (PKIX).
// Возвращает *rsa.PublicKey, либо ошибку, если файл не найден или содержит неподдерживаемый формат.
func LoadPublicKey(publicKeyPath string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block for public key")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rsaPub, nil
}

// EncryptDataHybrid выполняет гибридное шифрование данных:
// генерирует случайный AES-ключ (32 байта), шифрует данные с помощью AES-GCM,
// а затем шифрует сам AES-ключ с помощью переданного RSA-публичного ключа.
// Возвращает соединённый результат: зашифрованный AES-ключ + зашифрованные данные.
func EncryptDataHybrid(publicKey *rsa.PublicKey, data []byte) ([]byte, error) {
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %w", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	encryptedData := gcm.Seal(nonce, nonce, data, nil)

	hash := sha256.New()
	encryptedKey, err := rsa.EncryptOAEP(hash, rand.Reader, publicKey, aesKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt AES key: %w", err)
	}

	result := append(encryptedKey, encryptedData...)
	return result, nil
}

// DecryptDataHybrid расшифровывает данные, зашифрованные методом EncryptDataHybrid.
// Принимает RSA-приватный ключ и буфер (зашифрованный AES-ключ + AES-GCM данные).
// Расшифровывает AES-ключ, затем полностью расшифровывает данные.
func DecryptDataHybrid(privateKey *rsa.PrivateKey, data []byte) ([]byte, error) {
	keySize := privateKey.Size()
	if len(data) < keySize {
		return nil, fmt.Errorf("data too short")
	}

	encryptedKey := data[:keySize]
	encryptedData := data[keySize:]

	hash := sha256.New()
	aesKey, err := rsa.DecryptOAEP(hash, rand.Reader, privateKey, encryptedKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt  %w", err)
	}

	return plaintext, nil
}

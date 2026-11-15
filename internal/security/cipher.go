// Package security 提供本地凭据加密能力。
//
// 密钥保存在数据目录下的 secrets.key 文件中（32 字节），使用 AES-256-GCM
// 对 SSH 密码、AI API Key 等敏感信息进行加密。密钥文件首次使用时自动生成，
// 并通过“临时文件 + rename”的方式原子写入。
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// KeySize 是 AES-256 密钥长度（字节）。
const KeySize = 32

// Cipher 使用 AES-256-GCM 对字符串进行加解密。
//
// Cipher 可被多个 goroutine 并发使用（crypto/cipher.AEAD 的 Seal/Open
// 在密钥不变的情况下是并发安全的）。
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher 从 keyPath 加载密钥并构造 Cipher。
//
// 若 keyPath 不存在，则生成 32 字节随机密钥并以 0600 权限原子写入；
// 若已存在，则读取并校验其长度必须为 32 字节。错误信息不会包含密钥内容。
func NewCipher(keyPath string) (*Cipher, error) {
	if keyPath == "" {
		return nil, errors.New("密钥文件路径不能为空")
	}

	key, err := loadKeyFile(keyPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("读取密钥文件失败: %w", err)
		}
		// 首次运行：生成并原子写入密钥文件。
		generated := make([]byte, KeySize)
		if _, err := io.ReadFull(rand.Reader, generated); err != nil {
			return nil, fmt.Errorf("生成随机密钥失败: %w", err)
		}
		if err := writeKeyFileAtomic(keyPath, generated); err != nil {
			return nil, fmt.Errorf("写入密钥文件失败: %w", err)
		}
		// 回读磁盘上的最终内容，避免并发首次初始化时使用未落盘的密钥。
		key, err = loadKeyFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("校验新密钥文件失败: %w", err)
		}
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("初始化AES加密器失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("初始化GCM模式失败: %w", err)
	}
	return &Cipher{aead: gcm}, nil
}

// Encrypt 加密明文，返回 base64(nonce|ciphertext)。
// 明文为空时返回空字符串，便于“空密码表示未设置”的语义。
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if c == nil || c.aead == nil {
		return "", errors.New("加密器未初始化")
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}

	// Seal 返回 nonce 之后的密文（含认证标签）。
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt 解密 Encrypt 产生的字符串。空字符串返回空字符串。
func (c *Cipher) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if c == nil || c.aead == nil {
		return "", errors.New("加密器未初始化")
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// 兼容未填充的 base64 输入。
		data, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return "", errors.New("密文格式无效")
		}
	}

	nonceSize := c.aead.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("密文长度无效")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// 不返回底层错误细节，避免任何侧信道信息。
		return "", errors.New("解密失败：密文已损坏或密钥不匹配")
	}
	return string(plaintext), nil
}

// loadKeyFile 读取并校验密钥文件。
func loadKeyFile(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("密钥文件损坏：长度应为 %d 字节", KeySize)
	}
	// 尽力收紧权限；Windows 上 chmod 语义有限，失败不影响使用。
	_ = os.Chmod(path, 0o600)
	return key, nil
}

// writeKeyFileAtomic 将密钥原子写入目标路径：同目录临时文件 + rename。
func writeKeyFileAtomic(path string, key []byte) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("创建密钥目录失败: %w", err)
		}
	}

	f, err := os.CreateTemp(dir, ".secrets-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时密钥文件失败: %w", err)
	}
	tmpName := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmpName)
	}

	if err := f.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("设置密钥文件权限失败: %w", err)
	}
	if _, err := f.Write(key); err != nil {
		cleanup()
		return fmt.Errorf("写入密钥数据失败: %w", err)
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("同步密钥文件失败: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("关闭临时密钥文件失败: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("替换密钥文件失败: %w", err)
	}
	return nil
}

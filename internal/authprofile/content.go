package authprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/pelletier/go-toml/v2"
)

var reservedNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ValidateReservedName 校验中心预留的 model_provider 名称是否可安全用作目录名。
func ValidateReservedName(name string) error {
	if name == "" || name == "." || name == ".." {
		return errors.New("reserved name is invalid")
	}
	if !reservedNamePattern.MatchString(name) {
		return errors.New("reserved name contains an invalid character")
	}
	return nil
}

// ComputeContentHash 用规范化后的 auth.json 与 config-auth.toml 计算内容哈希。
// 服务端与客户端必须使用同一算法，避免仅因键顺序或空白差异误判为冲突。
func ComputeContentHash(authJSON, configTOML []byte) (string, error) {
	var auth map[string]any
	if err := json.Unmarshal(authJSON, &auth); err != nil {
		return "", err
	}
	canonicalAuth, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	var config map[string]any
	if err := toml.Unmarshal(configTOML, &config); err != nil {
		return "", err
	}
	canonicalConfig, err := toml.Marshal(config)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	digest.Write(canonicalAuth)
	digest.Write([]byte("\n"))
	digest.Write(canonicalConfig)
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

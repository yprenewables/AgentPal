package central

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// ParseJWTSubject 只解析 JWT payload 的 sub，不校验签名。
// 设计前提是 Central Host 只运行在可信内网。
func ParseJWTSubject(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid jwt")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("invalid jwt payload")
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", errors.New("invalid jwt payload")
	}
	subject, _ := payload["sub"].(string)
	if err := validateSubject(subject); err != nil {
		return "", err
	}
	return subject, nil
}

func validateSubject(subject string) error {
	if subject == "" {
		return errors.New("jwt sub is required")
	}
	if len(subject) > 256 {
		return errors.New("jwt sub is too long")
	}
	if subject != strings.TrimSpace(subject) {
		return errors.New("jwt sub has surrounding whitespace")
	}
	if subject == "." || subject == ".." || strings.Contains(subject, "..") {
		return errors.New("jwt sub is invalid")
	}
	for _, r := range subject {
		if r == '/' || r == '\\' || r == ':' || r < 0x20 || r == 0x7f {
			return errors.New("jwt sub contains an invalid character")
		}
	}
	return nil
}

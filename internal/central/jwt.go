package central

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// ParseJWTClaims 只解析 JWT payload，不校验签名。
// 设计前提是 Central Host 只运行在可信内网。
func ParseJWTClaims(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid jwt")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid jwt payload")
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, errors.New("invalid jwt payload")
	}
	return payload, nil
}

// ParseJWTSubject 解析 JWT payload 的 sub 并校验其可安全用作路径。
func ParseJWTSubject(token string) (string, error) {
	payload, err := ParseJWTClaims(token)
	if err != nil {
		return "", err
	}
	subject, _ := payload["sub"].(string)
	if err := validateSubject(subject); err != nil {
		return "", err
	}
	return subject, nil
}

// requestBearer 从 Authorization 头或 access_token Cookie 读取令牌。
func requestBearer(r *http.Request) string {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		if cookie, err := r.Cookie("access_token"); err == nil {
			token = cookie.Value
		}
	}
	return token
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

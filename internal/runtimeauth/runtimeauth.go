package runtimeauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const SettingKey = "agent_runtime_secret"

type SettingStore interface {
	GetSetting(key string) (string, bool, error)
	SetSetting(key, value string) error
}

type Payload struct {
	Type         string   `json:"typ"`
	WorkspaceID  string   `json:"workspaceId"`
	Project      string   `json:"project"`
	Agent        string   `json:"agent"`
	RunID        string   `json:"runId,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Exp          int64    `json:"exp"`
	Iat          int64    `json:"iat"`
}

type Principal struct {
	WorkspaceID  string   `json:"workspaceId"`
	Project      string   `json:"project"`
	Agent        string   `json:"agent"`
	RunID        string   `json:"runId,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Exp          int64    `json:"exp"`
	Iat          int64    `json:"iat"`
}

func EnsureSecret(store SettingStore) string {
	if store == nil {
		return GenerateSecret()
	}
	if value, ok, err := store.GetSetting(SettingKey); err == nil && ok && value != "" {
		return value
	}
	secret := GenerateSecret()
	_ = store.SetSetting(SettingKey, secret)
	return secret
}

func GenerateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

func Issue(secret string, payload Payload, ttl time.Duration) string {
	now := time.Now().UTC()
	payload.Type = "agent_runtime"
	payload.Iat = now.Unix()
	payload.Exp = now.Add(ttl).Unix()
	header := encode([]byte(`{"alg":"HS256","typ":"JWT"}`))
	raw, _ := json.Marshal(payload)
	body := encode(raw)
	sig := sign(secret, header+"."+body)
	return header + "." + body + "." + sig
}

func Validate(secret, token string) (Principal, bool) {
	parts := strings.SplitN(strings.TrimSpace(token), ".", 3)
	if len(parts) != 3 {
		return Principal{}, false
	}
	if !hmac.Equal([]byte(parts[2]), []byte(sign(secret, parts[0]+"."+parts[1]))) {
		return Principal{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Principal{}, false
	}
	var payload Payload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Principal{}, false
	}
	if payload.Type != "agent_runtime" || payload.WorkspaceID == "" || payload.Project == "" || payload.Agent == "" {
		return Principal{}, false
	}
	if time.Now().Unix() > payload.Exp {
		return Principal{}, false
	}
	return Principal{
		WorkspaceID:  payload.WorkspaceID,
		Project:      payload.Project,
		Agent:        payload.Agent,
		RunID:        payload.RunID,
		Capabilities: payload.Capabilities,
		Exp:          payload.Exp,
		Iat:          payload.Iat,
	}, true
}

func sign(secret, msg string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return encode(mac.Sum(nil))
}

func encode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

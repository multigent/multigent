package imbridge

import (
	"context"
	"fmt"
	"strings"

	larkbridge "github.com/multigent/multigent/internal/imbridge/lark"
)

type ProviderInfo struct {
	ID    string
	Label string
}

type SetupBeginResponse = larkbridge.BeginResponse
type SetupPollResponse = larkbridge.PollResponse

type Provider interface {
	Info() ProviderInfo
	OpenBaseURL() string
	BeginSetup(ctx context.Context) (SetupBeginResponse, error)
	PollSetup(ctx context.Context, deviceCode, baseURL string) (SetupPollResponse, error)
}

var registry = []Provider{
	larkFamilyProvider{id: larkbridge.ProviderFeishu, label: "Feishu"},
	larkFamilyProvider{id: larkbridge.ProviderLark, label: "Lark"},
}

func Providers() []ProviderInfo {
	out := make([]ProviderInfo, 0, len(registry))
	for _, provider := range registry {
		out = append(out, provider.Info())
	}
	return out
}

func LookupProvider(id string) (Provider, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, provider := range registry {
		if provider.Info().ID == id {
			return provider, true
		}
	}
	return nil, false
}

type larkFamilyProvider struct {
	id    string
	label string
}

func (p larkFamilyProvider) Info() ProviderInfo {
	return ProviderInfo{ID: p.id, Label: p.label}
}

func (p larkFamilyProvider) OpenBaseURL() string {
	if p.id == larkbridge.ProviderLark {
		return "https://open.larksuite.com"
	}
	return "https://open.feishu.cn"
}

func (p larkFamilyProvider) BeginSetup(ctx context.Context) (SetupBeginResponse, error) {
	client := larkbridge.RegistrationClient{}
	return client.Begin(ctx, p.id)
}

func (p larkFamilyProvider) PollSetup(ctx context.Context, deviceCode, baseURL string) (SetupPollResponse, error) {
	client := larkbridge.RegistrationClient{}
	return client.Poll(ctx, p.id, deviceCode, baseURL)
}

func MustOpenBaseURL(id string) (string, error) {
	provider, ok := LookupProvider(id)
	if !ok {
		return "", fmt.Errorf("unsupported IM provider %q", id)
	}
	return provider.OpenBaseURL(), nil
}

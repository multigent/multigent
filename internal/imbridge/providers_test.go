package imbridge

import "testing"

func TestProviderRegistryExposesFeishuAndLark(t *testing.T) {
	providers := Providers()
	if len(providers) != 2 {
		t.Fatalf("providers len=%d", len(providers))
	}
	feishu, ok := LookupProvider("FEISHU")
	if !ok {
		t.Fatalf("feishu provider not found")
	}
	if feishu.Info().ID != "feishu" || feishu.OpenBaseURL() != "https://open.feishu.cn" {
		t.Fatalf("unexpected feishu provider: %#v %s", feishu.Info(), feishu.OpenBaseURL())
	}
	lark, ok := LookupProvider("lark")
	if !ok {
		t.Fatalf("lark provider not found")
	}
	if lark.Info().ID != "lark" || lark.OpenBaseURL() != "https://open.larksuite.com" {
		t.Fatalf("unexpected lark provider: %#v %s", lark.Info(), lark.OpenBaseURL())
	}
	if _, ok := LookupProvider("slack"); ok {
		t.Fatalf("unknown provider should not resolve")
	}
}

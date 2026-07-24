package api

import (
	"context"
	"log"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/imbridge"
	larkbridge "github.com/multigent/multigent/internal/imbridge/lark"
)

func (s *Server) refreshAgentIMBridges() {
	if s.controlDB == nil {
		return
	}
	bindings, err := s.controlDB.ListAgentChannelBindings(controldb.AgentChannelBindingFilter{Status: "connected"})
	if err != nil {
		log.Printf("[im] list channel bindings failed: %v", err)
		return
	}
	next := make(map[string]imBridgeConfig)
	for _, binding := range bindings {
		provider, ok := imbridge.LookupProvider(binding.Provider)
		if !ok {
			continue
		}
		if provider.Info().ID != larkbridge.ProviderFeishu && provider.Info().ID != larkbridge.ProviderLark {
			continue
		}
		secret, ok, err := s.controlDB.ConnectionSecret(binding.ConnectionID)
		if err != nil {
			log.Printf("[im:%s] load channel secret failed for %s/%s: %v", binding.Provider, binding.ProjectID, binding.AgentID, err)
			continue
		}
		if !ok {
			continue
		}
		values, err := openConnectionSecret(secret)
		if err != nil {
			log.Printf("[im:%s] open channel secret failed for %s/%s: %v", binding.Provider, binding.ProjectID, binding.AgentID, err)
			continue
		}
		cfg := imBridgeConfig{
			provider:  provider,
			baseURL:   strings.TrimSpace(values["baseUrl"]),
			appID:     strings.TrimSpace(values["appId"]),
			appSecret: strings.TrimSpace(values["appSecret"]),
		}
		if cfg.baseURL == "" {
			cfg.baseURL = provider.OpenBaseURL()
		}
		if cfg.appID == "" || cfg.appSecret == "" {
			continue
		}
		next[cfg.key()] = cfg
	}

	s.agentIMMu.Lock()
	defer s.agentIMMu.Unlock()
	for key, cancel := range s.agentIMCancel {
		if _, ok := next[key]; !ok {
			cancel()
			delete(s.agentIMCancel, key)
		}
	}
	for key, cfg := range next {
		if _, ok := s.agentIMCancel[key]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.agentIMCancel[key] = cancel
		go s.runAgentIMBridge(ctx, key, cfg)
	}
}

func (s *Server) stopAgentIMBridges() {
	s.agentIMMu.Lock()
	defer s.agentIMMu.Unlock()
	for key, cancel := range s.agentIMCancel {
		log.Printf("[im] stopping bridge %s", key)
		cancel()
		delete(s.agentIMCancel, key)
	}
}

type imBridgeConfig struct {
	provider  imbridge.Provider
	baseURL   string
	appID     string
	appSecret string
}

func (c imBridgeConfig) key() string {
	return c.provider.Info().ID + ":" + c.appID
}

func (s *Server) runAgentIMBridge(ctx context.Context, key string, cfg imBridgeConfig) {
	providerID := cfg.provider.Info().ID
	handler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(eventCtx context.Context, event *larkim.P2MessageReceiveV1) error {
			message, ok := larkSDKMessageToIncoming(event)
			if !ok {
				return nil
			}
			result, err := s.acceptIMMessage(cfg.provider, cfg.appID, "", message)
			if err != nil {
				log.Printf("[im:%s] websocket event failed app=%s message=%s: %v", providerID, cfg.appID, message.MessageID, err)
				return nil
			}
			if ignored, _ := result["ignored"].(bool); ignored {
				log.Printf("[im:%s] websocket message ignored app=%s message=%s reason=%v", providerID, cfg.appID, message.MessageID, result["reason"])
			}
			_ = eventCtx
			return nil
		}).
		OnP2MessageReadV1(func(context.Context, *larkim.P2MessageReadV1) error {
			return nil
		})
	opts := []larkws.ClientOption{
		larkws.WithEventHandler(handler),
		larkws.WithLogLevel(larkcore.LogLevelWarn),
		larkws.WithLogger(agentIMLogger{}),
	}
	if cfg.baseURL != "" {
		opts = append(opts, larkws.WithDomain(cfg.baseURL))
	}
	client := larkws.NewClient(cfg.appID, cfg.appSecret, opts...)
	log.Printf("[im:%s] starting websocket bridge key=%s base=%s", providerID, key, cfg.baseURL)
	if err := client.Start(ctx); err != nil && ctx.Err() == nil {
		log.Printf("[im:%s] websocket bridge stopped key=%s: %v", providerID, key, err)
	}
}

func larkSDKMessageToIncoming(event *larkim.P2MessageReceiveV1) (imbridge.IncomingMessage, bool) {
	if event == nil || event.Event == nil || event.Event.Message == nil || event.Event.Sender == nil {
		return imbridge.IncomingMessage{}, false
	}
	msg := event.Event.Message
	out := imbridge.IncomingMessage{
		MessageID:    stringPtrValue(msg.MessageId),
		ChatID:       stringPtrValue(msg.ChatId),
		ChatType:     stringPtrValue(msg.ChatType),
		RootID:       stringPtrValue(msg.RootId),
		ParentID:     stringPtrValue(msg.ParentId),
		SenderOpenID: larkSDKUserID(event.Event.Sender.SenderId),
		RawContent:   stringPtrValue(msg.Content),
	}
	out.Text = larkbridge.ExtractText(larkbridge.EventMessage{
		MessageID:   out.MessageID,
		RootID:      out.RootID,
		ParentID:    out.ParentID,
		ChatID:      out.ChatID,
		ChatType:    out.ChatType,
		MessageType: stringPtrValue(msg.MessageType),
		Content:     out.RawContent,
	})
	if strings.TrimSpace(out.MessageID) == "" {
		out.MessageID = "im-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return out, true
}

func larkSDKUserID(id *larkim.UserId) string {
	if id == nil {
		return ""
	}
	if strings.TrimSpace(stringPtrValue(id.OpenId)) != "" {
		return strings.TrimSpace(stringPtrValue(id.OpenId))
	}
	if strings.TrimSpace(stringPtrValue(id.UserId)) != "" {
		return strings.TrimSpace(stringPtrValue(id.UserId))
	}
	return strings.TrimSpace(stringPtrValue(id.UnionId))
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

type agentIMLogger struct{}

func (agentIMLogger) Debug(context.Context, ...interface{}) {}
func (agentIMLogger) Info(context.Context, ...interface{})  {}
func (agentIMLogger) Warn(_ context.Context, args ...interface{}) {
	log.Print(append([]interface{}{"[im:ws] "}, args...)...)
}
func (agentIMLogger) Error(_ context.Context, args ...interface{}) {
	log.Print(append([]interface{}{"[im:ws] "}, args...)...)
}

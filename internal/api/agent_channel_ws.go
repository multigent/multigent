package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
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
			secrets:   values,
		}
		if cfg.baseURL == "" {
			cfg.baseURL = provider.OpenBaseURL()
		}
		if !cfg.isRunnable() {
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
	secrets   map[string]string
}

func (c imBridgeConfig) key() string {
	return c.provider.Info().ID + ":" + c.appID
}

func (c imBridgeConfig) isRunnable() bool {
	switch c.provider.Info().ID {
	case larkbridge.ProviderFeishu, larkbridge.ProviderLark:
		return c.appID != "" && c.appSecret != ""
	case "telegram", "discord":
		return strings.TrimSpace(c.secrets["botToken"]) != ""
	case "slack":
		return false
	default:
		return false
	}
}

func (s *Server) runAgentIMBridge(ctx context.Context, key string, cfg imBridgeConfig) {
	providerID := cfg.provider.Info().ID
	switch providerID {
	case "telegram":
		s.runTelegramIMBridge(ctx, key, cfg)
		return
	case "discord":
		s.runDiscordIMBridge(ctx, key, cfg)
		return
	}
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

func (s *Server) runTelegramIMBridge(ctx context.Context, key string, cfg imBridgeConfig) {
	token := strings.TrimSpace(cfg.secrets["botToken"])
	baseURL := strings.TrimRight(firstNonEmptyIMBridge(cfg.baseURL, "https://api.telegram.org"), "/")
	if token == "" {
		return
	}
	log.Printf("[im:telegram] starting polling bridge key=%s", key)
	offset := int64(0)
	_, _ = httpJSONForBridge(ctx, http.MethodGet, baseURL+"/bot"+token+"/getUpdates?offset=-1&timeout=0", "", nil)
	for ctx.Err() == nil {
		endpoint := fmt.Sprintf("%s/bot%s/getUpdates?timeout=30", baseURL, token)
		if offset > 0 {
			endpoint += "&offset=" + strconv.FormatInt(offset, 10)
		}
		raw, err := httpJSONForBridge(ctx, http.MethodGet, endpoint, "", nil)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("[im:telegram] polling failed key=%s: %v", key, err)
				sleepContext(ctx, 5*time.Second)
			}
			continue
		}
		var resp struct {
			OK     bool              `json:"ok"`
			Result []json.RawMessage `json:"result"`
		}
		if json.Unmarshal(raw, &resp) != nil || !resp.OK {
			sleepContext(ctx, 3*time.Second)
			continue
		}
		for _, item := range resp.Result {
			var update struct {
				UpdateID int64 `json:"update_id"`
			}
			if json.Unmarshal(item, &update) == nil && update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			parsed, err := cfg.provider.ParseEvent(item)
			if err != nil || !parsed.IsMessage {
				continue
			}
			result, err := s.acceptIMMessage(cfg.provider, cfg.appID, "", parsed.Message)
			if err != nil {
				log.Printf("[im:telegram] event failed key=%s message=%s: %v", key, parsed.Message.MessageID, err)
				continue
			}
			if ignored, _ := result["ignored"].(bool); ignored {
				log.Printf("[im:telegram] message ignored key=%s message=%s reason=%v", key, parsed.Message.MessageID, result["reason"])
			}
		}
	}
}

func (s *Server) runDiscordIMBridge(ctx context.Context, key string, cfg imBridgeConfig) {
	token := strings.TrimSpace(cfg.secrets["botToken"])
	if token == "" {
		return
	}
	backoff := 3 * time.Second
	for ctx.Err() == nil {
		if err := s.runDiscordGatewayOnce(ctx, key, cfg, token); err != nil && ctx.Err() == nil {
			log.Printf("[im:discord] gateway stopped key=%s: %v", key, err)
			sleepContext(ctx, backoff)
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = 3 * time.Second
	}
}

func (s *Server) runDiscordGatewayOnce(ctx context.Context, key string, cfg imBridgeConfig, token string) error {
	raw, err := httpJSONForBridge(ctx, http.MethodGet, "https://discord.com/api/v10/gateway/bot", "Bot "+token, nil)
	if err != nil {
		return err
	}
	var gateway struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &gateway); err != nil {
		return err
	}
	if gateway.URL == "" {
		return fmt.Errorf("discord gateway URL is empty")
	}
	wsURL := gateway.URL + "/?v=10&encoding=json"
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Printf("[im:discord] starting gateway bridge key=%s", key)
	var hello struct {
		Op int `json:"op"`
		D  struct {
			HeartbeatInterval int `json:"heartbeat_interval"`
		} `json:"d"`
	}
	if err := conn.ReadJSON(&hello); err != nil {
		return err
	}
	heartbeatInterval := time.Duration(hello.D.HeartbeatInterval) * time.Millisecond
	if heartbeatInterval <= 0 {
		heartbeatInterval = 45 * time.Second
	}
	identify := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":   token,
			"intents": 1 | 512 | 4096 | 32768,
			"properties": map[string]string{
				"os":      "linux",
				"browser": "multigent",
				"device":  "multigent",
			},
		},
	}
	if err := conn.WriteJSON(identify); err != nil {
		return err
	}
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	done := make(chan error, 1)
	seqCh := make(chan int64, 8)
	go func() {
		for {
			var packet struct {
				Op int             `json:"op"`
				T  string          `json:"t"`
				S  *int64          `json:"s"`
				D  json.RawMessage `json:"d"`
			}
			if err := conn.ReadJSON(&packet); err != nil {
				done <- err
				return
			}
			if packet.S != nil {
				select {
				case seqCh <- *packet.S:
				default:
				}
			}
			if packet.T != "MESSAGE_CREATE" {
				continue
			}
			message, ok := discordGatewayMessageToIncoming(packet.D, cfg.appID)
			if !ok {
				continue
			}
			result, err := s.acceptIMMessage(cfg.provider, cfg.appID, "", message)
			if err != nil {
				log.Printf("[im:discord] event failed key=%s message=%s: %v", key, message.MessageID, err)
				continue
			}
			if ignored, _ := result["ignored"].(bool); ignored {
				log.Printf("[im:discord] message ignored key=%s message=%s reason=%v", key, message.MessageID, result["reason"])
			}
		}
	}()
	var lastSeq *int64
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-done:
			return err
		case seq := <-seqCh:
			lastSeq = &seq
		case <-ticker.C:
			if err := conn.WriteJSON(map[string]any{"op": 1, "d": lastSeq}); err != nil {
				return err
			}
		}
	}
}

func discordGatewayMessageToIncoming(raw json.RawMessage, botID string) (imbridge.IncomingMessage, bool) {
	var msg struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
		GuildID   string `json:"guild_id"`
		Content   string `json:"content"`
		Author    struct {
			ID  string `json:"id"`
			Bot bool   `json:"bot"`
		} `json:"author"`
		ReferencedMessage *struct {
			ID string `json:"id"`
		} `json:"referenced_message"`
	}
	if json.Unmarshal(raw, &msg) != nil || msg.ID == "" || msg.Author.ID == "" || msg.Author.Bot || msg.Author.ID == botID {
		return imbridge.IncomingMessage{}, false
	}
	chatType := "guild"
	if msg.GuildID == "" {
		chatType = "dm"
	}
	parent := ""
	if msg.ReferencedMessage != nil {
		parent = msg.ReferencedMessage.ID
	}
	text := stripDiscordMention(msg.Content, botID)
	if strings.TrimSpace(text) == "" {
		return imbridge.IncomingMessage{}, false
	}
	return imbridge.IncomingMessage{
		MessageID:    msg.ID,
		ChatID:       msg.ChannelID,
		ChatType:     chatType,
		ParentID:     parent,
		SenderOpenID: msg.Author.ID,
		Text:         text,
		RawContent:   msg.Content,
	}, true
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

func firstNonEmptyIMBridge(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stripDiscordMention(text, botID string) string {
	text = strings.ReplaceAll(text, "<@"+botID+">", "")
	text = strings.ReplaceAll(text, "<@!"+botID+">", "")
	return strings.TrimSpace(text)
}

func httpJSONForBridge(ctx context.Context, method, endpoint, auth string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(auth) != "" {
		req.Header.Set("Authorization", strings.TrimSpace(auth))
	}
	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s http %d: %s", method, endpoint, resp.StatusCode, string(raw))
	}
	return raw, nil
}

func sleepContext(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
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

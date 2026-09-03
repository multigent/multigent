package lark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OpenAPIClient struct {
	BaseURL     string
	AppID       string
	AppSecret   string
	AccessToken string
	HTTPClient  *http.Client
}

type InteractiveCard struct {
	InteractionID string
	Title         string
	Body          string
	RawJSON       json.RawMessage
	Fields        []InteractiveCardField
	Actions       []InteractiveCardAction
	Links         []InteractiveCardLink
}

type InteractiveCardField struct {
	Label string
	Value string
}

type InteractiveCardAction struct {
	ID           string
	Label        string
	Style        string
	RequiresText bool
}

type InteractiveCardLink struct {
	Label string
	URL   string
}

type ProgressCardEntry struct {
	Kind    string
	Title   string
	Content string
	Status  string
}

type ProgressCard struct {
	Title     string
	State     string
	Reasoning []ProgressCardEntry
	Tools     []ProgressCardEntry
	Final     string
}

type OutgoingAttachment struct {
	Kind     string
	FileName string
	MIME     string
	Data     []byte
}

type DownloadedResource struct {
	FileName string
	MIME     string
	Data     []byte
}

type UserProfile struct {
	OpenID          string
	UserID          string
	UnionID         string
	Name            string
	Email           string
	EnterpriseEmail string
}

func (c OpenAPIClient) ReplyText(ctx context.Context, messageID, text string) error {
	messageID = strings.TrimSpace(messageID)
	text = strings.TrimSpace(text)
	if messageID == "" {
		return fmt.Errorf("message id is required")
	}
	if text == "" {
		text = "(empty response)"
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{
		"msg_type": "text",
		"content":  mustJSON(map[string]string{"text": text}),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.openBaseURL(), "/")+"/open-apis/im/v1/messages/"+messageID+"/reply", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("reply message http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("reply message failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) AddReaction(ctx context.Context, messageID, emoji string) (string, error) {
	messageID = strings.TrimSpace(messageID)
	emoji = strings.TrimSpace(emoji)
	if messageID == "" {
		return "", fmt.Errorf("message id is required")
	}
	if emoji == "" {
		emoji = "OK"
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]any{
		"reaction_type": map[string]string{"emoji_type": emoji},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.openBaseURL(), "/")+"/open-apis/im/v1/messages/"+messageID+"/reactions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("add reaction http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ReactionID string `json:"reaction_id"`
			Reaction   struct {
				ReactionID string `json:"reaction_id"`
			} `json:"reaction"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &parsed) == nil {
		if parsed.Code != 0 {
			return "", fmt.Errorf("add reaction failed: code=%d msg=%s", parsed.Code, parsed.Msg)
		}
		if parsed.Data.ReactionID != "" {
			return parsed.Data.ReactionID, nil
		}
		return parsed.Data.Reaction.ReactionID, nil
	}
	return "", nil
}

func (c OpenAPIClient) RemoveReaction(ctx context.Context, messageID, reactionID string) error {
	messageID = strings.TrimSpace(messageID)
	reactionID = strings.TrimSpace(reactionID)
	if messageID == "" || reactionID == "" {
		return nil
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, strings.TrimRight(c.openBaseURL(), "/")+"/open-apis/im/v1/messages/"+messageID+"/reactions/"+reactionID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("remove reaction http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("remove reaction failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) ReplyMarkdown(ctx context.Context, messageID, title, markdown string) error {
	messageID = strings.TrimSpace(messageID)
	title = strings.TrimSpace(title)
	markdown = strings.TrimSpace(markdown)
	if messageID == "" {
		return fmt.Errorf("message id is required")
	}
	if markdown == "" {
		markdown = "(empty message)"
	}
	return c.replyRaw(ctx, messageID, "post", buildMarkdownPostBody(title, markdown), "reply markdown message")
}

func (c OpenAPIClient) ReplyProgressCard(ctx context.Context, messageID string, card ProgressCard) (string, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return "", fmt.Errorf("message id is required")
	}
	return c.replyRawWithMessageID(ctx, messageID, "interactive", mustJSON(buildProgressCardBody(card)), "reply progress card")
}

func (c OpenAPIClient) PatchProgressCard(ctx context.Context, messageID string, card ProgressCard) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("message id is required")
	}
	return c.patchRaw(ctx, messageID, mustJSON(buildProgressCardBody(card)), "patch progress card")
}

func (c OpenAPIClient) ReplyInteractiveCard(ctx context.Context, messageID string, card InteractiveCard) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("message id is required")
	}
	if strings.TrimSpace(card.Title) == "" {
		card.Title = "Agent"
	}
	if len(bytes.TrimSpace(card.RawJSON)) == 0 && strings.TrimSpace(card.Body) == "" {
		card.Body = "请选择一个操作。"
	}
	if len(bytes.TrimSpace(card.RawJSON)) == 0 && strings.TrimSpace(card.InteractionID) == "" {
		return fmt.Errorf("interaction id is required")
	}
	return c.replyRaw(ctx, messageID, "interactive", mustJSON(buildInteractiveCardBody(card, nil)), "reply interactive card")
}

func (c OpenAPIClient) ReplyAttachment(ctx context.Context, messageID string, attachment OutgoingAttachment) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("message id is required")
	}
	msgType, content, err := c.prepareAttachmentMessage(ctx, attachment)
	if err != nil {
		return err
	}
	return c.replyRaw(ctx, messageID, msgType, content, "reply attachment")
}

func (c OpenAPIClient) replyRaw(ctx context.Context, messageID, msgType, content, op string) error {
	_, err := c.replyRawWithMessageID(ctx, messageID, msgType, content, op)
	return err
}

func (c OpenAPIClient) replyRawWithMessageID(ctx context.Context, messageID, msgType, content, op string) (string, error) {
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{
		"msg_type": msgType,
		"content":  content,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.openBaseURL(), "/")+"/open-apis/im/v1/messages/"+messageID+"/reply", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s http %d: %s", op, resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
			Message   struct {
				MessageID string `json:"message_id"`
			} `json:"message"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return "", fmt.Errorf("%s failed: code=%d msg=%s", op, parsed.Code, parsed.Msg)
	}
	if parsed.Data.MessageID != "" {
		return parsed.Data.MessageID, nil
	}
	return parsed.Data.Message.MessageID, nil
}

func (c OpenAPIClient) patchRaw(ctx context.Context, messageID, content, op string) error {
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"content": content})
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, strings.TrimRight(c.openBaseURL(), "/")+"/open-apis/im/v1/messages/"+messageID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s http %d: %s", op, resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("%s failed: code=%d msg=%s", op, parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) SendAttachment(ctx context.Context, receiveIDType, receiveID string, attachment OutgoingAttachment) error {
	receiveIDType = strings.TrimSpace(receiveIDType)
	receiveID = strings.TrimSpace(receiveID)
	if receiveIDType == "" {
		receiveIDType = "open_id"
	}
	if receiveID == "" {
		return fmt.Errorf("receive id is required")
	}
	msgType, content, err := c.prepareAttachmentMessage(ctx, attachment)
	if err != nil {
		return err
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{
		"receive_id": receiveID,
		"msg_type":   msgType,
		"content":    content,
	})
	u := strings.TrimRight(c.openBaseURL(), "/") + "/open-apis/im/v1/messages?receive_id_type=" + receiveIDType
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send attachment message http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("send attachment message failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) prepareAttachmentMessage(ctx context.Context, attachment OutgoingAttachment) (string, string, error) {
	if len(attachment.Data) == 0 {
		return "", "", fmt.Errorf("attachment data is required")
	}
	kind := strings.ToLower(strings.TrimSpace(attachment.Kind))
	if kind == "" {
		kind = inferAttachmentKind(attachment.FileName, attachment.MIME)
	}
	if kind == "image" {
		imageKey, err := c.uploadImage(ctx, attachment.Data)
		if err != nil {
			return "", "", err
		}
		return "image", mustJSON(map[string]string{"image_key": imageKey}), nil
	}
	fileName := strings.TrimSpace(attachment.FileName)
	if fileName == "" {
		fileName = "attachment"
	}
	fileType := detectLarkFileType(attachment.MIME, fileName)
	fileKey, err := c.uploadFile(ctx, fileType, fileName, attachment.Data)
	if err != nil {
		return "", "", err
	}
	msgType := larkFileMessageType(fileType)
	switch msgType {
	case "audio", "media":
		return msgType, mustJSON(map[string]string{"file_key": fileKey}), nil
	default:
		return "file", mustJSON(map[string]string{"file_key": fileKey}), nil
	}
}

func (c OpenAPIClient) uploadImage(ctx context.Context, data []byte) (string, error) {
	raw, err := c.postMultipart(ctx, "/open-apis/im/v1/images", map[string]string{"image_type": "message"}, "image", "image", data)
	if err != nil {
		return "", fmt.Errorf("upload image: %w", err)
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ImageKey string `json:"image_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("upload image response: %w", err)
	}
	if parsed.Code != 0 {
		return "", fmt.Errorf("upload image failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	if strings.TrimSpace(parsed.Data.ImageKey) == "" {
		return "", fmt.Errorf("upload image: no image_key returned")
	}
	return parsed.Data.ImageKey, nil
}

func (c OpenAPIClient) uploadFile(ctx context.Context, fileType, fileName string, data []byte) (string, error) {
	raw, err := c.postMultipart(ctx, "/open-apis/im/v1/files", map[string]string{
		"file_type": fileType,
		"file_name": fileName,
	}, "file", fileName, data)
	if err != nil {
		return "", fmt.Errorf("upload file: %w", err)
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			FileKey string `json:"file_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("upload file response: %w", err)
	}
	if parsed.Code != 0 {
		return "", fmt.Errorf("upload file failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	if strings.TrimSpace(parsed.Data.FileKey) == "" {
		return "", fmt.Errorf("upload file: no file_key returned")
	}
	return parsed.Data.FileKey, nil
}

func (c OpenAPIClient) DownloadMessageResource(ctx context.Context, messageID, fileKey, resourceType string) (DownloadedResource, error) {
	messageID = strings.TrimSpace(messageID)
	fileKey = strings.TrimSpace(fileKey)
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	if messageID == "" {
		return DownloadedResource{}, fmt.Errorf("message id is required")
	}
	if fileKey == "" {
		return DownloadedResource{}, fmt.Errorf("file key is required")
	}
	if resourceType == "" {
		resourceType = "file"
	}
	if resourceType != "image" {
		resourceType = "file"
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return DownloadedResource{}, err
	}
	u := strings.TrimRight(c.openBaseURL(), "/") + "/open-apis/im/v1/messages/" + url.PathEscape(messageID) + "/resources/" + url.PathEscape(fileKey) + "?type=" + url.QueryEscape(resourceType)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return DownloadedResource{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return DownloadedResource{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return DownloadedResource{}, fmt.Errorf("download message resource http %d: %s", resp.StatusCode, string(raw))
	}
	const maxDownloadBytes = 50 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return DownloadedResource{}, err
	}
	if len(data) > maxDownloadBytes {
		return DownloadedResource{}, fmt.Errorf("download message resource too large")
	}
	fileName := fileKey
	if disposition := strings.TrimSpace(resp.Header.Get("Content-Disposition")); disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			if candidate := strings.TrimSpace(params["filename"]); candidate != "" {
				fileName = candidate
			}
		}
	}
	return DownloadedResource{
		FileName: fileName,
		MIME:     strings.TrimSpace(resp.Header.Get("Content-Type")),
		Data:     data,
	}, nil
}

// ExpandMergeForwardMessage resolves a forwarded message into its child
// messages. Lark emits merge_forward as a container; the downloadable
// resources belong to the outer container message, while the file/image keys
// are present only in the fetched child messages.
func (c OpenAPIClient) ExpandMergeForwardMessage(ctx context.Context, messageID string) (string, []MessageAttachment, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return "", nil, fmt.Errorf("message id is required")
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return "", nil, err
	}
	u := strings.TrimRight(c.openBaseURL(), "/") + "/open-apis/im/v1/messages/" + url.PathEscape(messageID) + "?user_id_type=open_id&card_msg_content_type=raw_card_content&with_sender_name=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("get forwarded message http %d: %s", resp.StatusCode, string(raw))
	}
	var envelope struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", nil, fmt.Errorf("decode forwarded message response: %w", err)
	}
	if envelope.Code != 0 {
		return "", nil, fmt.Errorf("get forwarded message failed: code=%d msg=%s", envelope.Code, envelope.Msg)
	}
	itemsRaw, _ := envelope.Data["items"].([]any)
	items := make([]map[string]any, 0, len(itemsRaw))
	for _, item := range itemsRaw {
		if parsed, ok := item.(map[string]any); ok {
			items = append(items, parsed)
		}
	}
	return ExpandMergeForwardItems(items, messageID)
}

func (c OpenAPIClient) postMultipart(ctx context.Context, path string, fields map[string]string, fileField, fileName string, data []byte) ([]byte, error) {
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.openBaseURL(), "/")+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(raw))
	}
	return raw, nil
}

func (c OpenAPIClient) SendText(ctx context.Context, receiveIDType, receiveID, text string) error {
	receiveIDType = strings.TrimSpace(receiveIDType)
	receiveID = strings.TrimSpace(receiveID)
	text = strings.TrimSpace(text)
	if receiveIDType == "" {
		receiveIDType = "open_id"
	}
	if receiveID == "" {
		return fmt.Errorf("receive id is required")
	}
	if text == "" {
		text = "(empty message)"
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{
		"receive_id": receiveID,
		"msg_type":   "text",
		"content":    mustJSON(map[string]string{"text": text}),
	})
	u := strings.TrimRight(c.openBaseURL(), "/") + "/open-apis/im/v1/messages?receive_id_type=" + receiveIDType
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send message http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("send message failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) SendMarkdown(ctx context.Context, receiveIDType, receiveID, title, markdown string) error {
	receiveIDType = strings.TrimSpace(receiveIDType)
	receiveID = strings.TrimSpace(receiveID)
	title = strings.TrimSpace(title)
	markdown = strings.TrimSpace(markdown)
	if receiveIDType == "" {
		receiveIDType = "open_id"
	}
	if receiveID == "" {
		return fmt.Errorf("receive id is required")
	}
	if markdown == "" {
		markdown = "(empty message)"
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{
		"receive_id": receiveID,
		"msg_type":   "post",
		"content":    buildMarkdownPostBody(title, markdown),
	})
	u := strings.TrimRight(c.openBaseURL(), "/") + "/open-apis/im/v1/messages?receive_id_type=" + receiveIDType
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send markdown message http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("send markdown message failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) GetUser(ctx context.Context, userID, userIDType string) (UserProfile, error) {
	userID = strings.TrimSpace(userID)
	userIDType = strings.TrimSpace(userIDType)
	if userID == "" {
		return UserProfile{}, fmt.Errorf("user id is required")
	}
	if userIDType == "" {
		userIDType = "open_id"
	}
	token := strings.TrimSpace(c.AccessToken)
	if token == "" {
		var err error
		token, err = c.tenantAccessToken(ctx)
		if err != nil {
			return UserProfile{}, err
		}
	}
	u := strings.TrimRight(c.openBaseURL(), "/") + "/open-apis/contact/v3/users/" + userID + "?user_id_type=" + userIDType
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return UserProfile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return UserProfile{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return UserProfile{}, fmt.Errorf("get user http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			User struct {
				OpenID          string `json:"open_id"`
				UserID          string `json:"user_id"`
				UnionID         string `json:"union_id"`
				Name            string `json:"name"`
				Email           string `json:"email"`
				EnterpriseEmail string `json:"enterprise_email"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return UserProfile{}, err
	}
	if parsed.Code != 0 {
		return UserProfile{}, fmt.Errorf("get user failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	user := parsed.Data.User
	return UserProfile{
		OpenID:          user.OpenID,
		UserID:          user.UserID,
		UnionID:         user.UnionID,
		Name:            user.Name,
		Email:           user.Email,
		EnterpriseEmail: user.EnterpriseEmail,
	}, nil
}

func buildMarkdownCardBody(title, markdown string) map[string]any {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Agent"
	}
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		markdown = "(empty message)"
	}
	return map[string]any{
		"schema": "2.0",
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"title": map[string]string{"tag": "plain_text", "content": title},
		},
		"body": map[string]any{
			"elements": []map[string]any{{
				"tag":  "div",
				"text": map[string]string{"tag": "lark_md", "content": markdown},
			}},
		},
	}
}

func buildMarkdownPostBody(title, markdown string) string {
	title = strings.TrimSpace(title)
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		markdown = "(empty message)"
	}
	body := map[string]any{
		"zh_cn": map[string]any{
			"content": [][]map[string]any{{
				{"tag": "md", "text": markdown},
			}},
		},
	}
	if title != "" {
		body["zh_cn"].(map[string]any)["title"] = title
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

func inferAttachmentKind(fileName, mimeType string) string {
	name := strings.ToLower(fileName)
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") || strings.HasSuffix(name, ".gif") || strings.HasSuffix(name, ".webp") || strings.HasSuffix(name, ".bmp"):
		return "image"
	default:
		return "file"
	}
}

func detectLarkFileType(mimeType, fileName string) string {
	name := strings.ToLower(fileName)
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case mimeType == "application/pdf" || strings.HasSuffix(name, ".pdf"):
		return "pdf"
	case strings.HasSuffix(name, ".doc") || strings.HasSuffix(name, ".docx"):
		return "doc"
	case strings.HasSuffix(name, ".xls") || strings.HasSuffix(name, ".xlsx") || strings.HasSuffix(name, ".csv"):
		return "xls"
	case strings.HasSuffix(name, ".ppt") || strings.HasSuffix(name, ".pptx"):
		return "ppt"
	case strings.HasPrefix(mimeType, "video/") || strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".mov") || strings.HasSuffix(name, ".m4v") || strings.HasSuffix(name, ".webm") || strings.HasSuffix(name, ".mkv"):
		return "mp4"
	case mimeType == "audio/ogg" || mimeType == "audio/opus" || mimeType == "application/ogg" || strings.HasSuffix(name, ".ogg") || strings.HasSuffix(name, ".opus"):
		return "opus"
	default:
		return "stream"
	}
}

func larkFileMessageType(fileType string) string {
	switch fileType {
	case "opus":
		return "audio"
	case "mp4":
		return "media"
	default:
		return "file"
	}
}

func (c OpenAPIClient) SendInteractiveCard(ctx context.Context, receiveIDType, receiveID string, card InteractiveCard) error {
	receiveIDType = strings.TrimSpace(receiveIDType)
	receiveID = strings.TrimSpace(receiveID)
	if receiveIDType == "" {
		receiveIDType = "open_id"
	}
	if receiveID == "" {
		return fmt.Errorf("receive id is required")
	}
	if strings.TrimSpace(card.Title) == "" {
		card.Title = "Agent"
	}
	if len(bytes.TrimSpace(card.RawJSON)) == 0 && strings.TrimSpace(card.Body) == "" {
		card.Body = "请选择一个操作。"
	}
	if len(bytes.TrimSpace(card.RawJSON)) == 0 && strings.TrimSpace(card.InteractionID) == "" {
		return fmt.Errorf("interaction id is required")
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	cardBody := buildInteractiveCardBody(card, nil)
	body, _ := json.Marshal(map[string]string{
		"receive_id": receiveID,
		"msg_type":   "interactive",
		"content":    mustJSON(cardBody),
	})
	u := strings.TrimRight(c.openBaseURL(), "/") + "/open-apis/im/v1/messages?receive_id_type=" + receiveIDType
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send interactive card http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("send interactive card failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func (c OpenAPIClient) UpdateInteractiveCard(ctx context.Context, updateToken, operatorOpenID string, card InteractiveCard) error {
	updateToken = strings.TrimSpace(updateToken)
	if updateToken == "" {
		return fmt.Errorf("card update token is required")
	}
	operatorOpenID = strings.TrimSpace(operatorOpenID)
	if strings.TrimSpace(card.Title) == "" {
		card.Title = "Agent"
	}
	if strings.TrimSpace(card.Body) == "" {
		card.Body = "操作已提交。"
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	var openIDs []string
	if operatorOpenID != "" {
		openIDs = []string{operatorOpenID}
	}
	body, _ := json.Marshal(map[string]any{
		"token": updateToken,
		"card":  buildInteractiveCardBody(card, openIDs),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.openBaseURL(), "/")+"/open-apis/interactive/v1/card/update", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("update interactive card http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("update interactive card failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

func buildInteractiveCardBody(card InteractiveCard, openIDs []string) map[string]any {
	if len(bytes.TrimSpace(card.RawJSON)) > 0 {
		var raw map[string]any
		if err := json.Unmarshal(card.RawJSON, &raw); err == nil && raw != nil {
			if len(openIDs) > 0 {
				raw["open_ids"] = openIDs
			}
			return raw
		}
	}
	elements := make([]map[string]any, 0, 4+len(card.Fields)+len(card.Links))
	appendHR := func() {
		if len(elements) == 0 {
			return
		}
		if tag, _ := elements[len(elements)-1]["tag"].(string); tag == "hr" {
			return
		}
		elements = append(elements, map[string]any{"tag": "hr"})
	}
	body := strings.TrimSpace(card.Body)
	if body != "" {
		for _, chunk := range splitCardMarkdown(body, 900) {
			elements = append(elements, map[string]any{"tag": "markdown", "content": chunk})
		}
	}
	if len(card.Fields) > 0 {
		appendHR()
	}
	for _, field := range card.Fields {
		label := strings.TrimSpace(field.Label)
		value := strings.TrimSpace(field.Value)
		if label == "" && value == "" {
			continue
		}
		if label == "" {
			elements = append(elements, map[string]any{"tag": "markdown", "content": value})
			continue
		}
		content := "**" + label + "**"
		if value != "" {
			content += "\n" + value
		}
		elements = append(elements, map[string]any{"tag": "markdown", "content": content})
	}
	for _, link := range card.Links {
		if strings.TrimSpace(link.Label) == "" || strings.TrimSpace(link.URL) == "" {
			continue
		}
		elements = append(elements, map[string]any{
			"tag":  "button",
			"text": map[string]string{"tag": "plain_text", "content": strings.TrimSpace(link.Label)},
			"type": "default",
			"behaviors": []map[string]any{{
				"type":        "open_url",
				"default_url": map[string]string{"url": strings.TrimSpace(link.URL)},
			}},
		})
	}
	actions := make([]map[string]any, 0, len(card.Actions))
	needsText := false
	for _, action := range card.Actions {
		id := strings.TrimSpace(action.ID)
		label := strings.TrimSpace(action.Label)
		if id == "" || label == "" {
			continue
		}
		if action.RequiresText {
			needsText = true
		}
		style := normalizeButtonStyle(action.Style)
		actions = append(actions, map[string]any{
			"tag":  "button",
			"text": map[string]string{"tag": "plain_text", "content": label},
			"type": style,
			"behaviors": []map[string]any{{
				"type": "callback",
				"value": map[string]string{
					"interaction_id": strings.TrimSpace(card.InteractionID),
					"action_id":      id,
					"action_label":   label,
				},
			}},
		})
	}
	if needsText || len(actions) > 0 {
		appendHR()
	}
	if needsText {
		elements = append(elements, map[string]any{
			"tag":         "input",
			"name":        "comment",
			"placeholder": map[string]string{"tag": "plain_text", "content": "补充说明或理由（可选）"},
		})
	}
	if len(actions) > 0 {
		elements = append(elements, actions...)
	}
	cardBody := map[string]any{
		"schema": "2.0",
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"template": cardHeaderTemplate(card.Title),
			"title":    map[string]string{"tag": "plain_text", "content": strings.TrimSpace(card.Title)},
		},
		"body": map[string]any{"elements": elements},
	}
	if len(openIDs) > 0 {
		cardBody["open_ids"] = openIDs
	}
	return cardBody
}

func buildProgressCardBody(card ProgressCard) map[string]any {
	title := strings.TrimSpace(card.Title)
	if title == "" {
		title = "Agent"
	}
	state := strings.ToLower(strings.TrimSpace(card.State))
	template := "blue"
	switch state {
	case "completed", "done", "success":
		template = "green"
	case "failed", "error":
		template = "red"
	case "running", "":
		template = "blue"
	default:
		template = "grey"
	}
	running := state == "running" || state == ""
	elements := make([]map[string]any, 0, 4)
	showReasoningPlaceholder := running || len(card.Reasoning) > 0
	if showReasoningPlaceholder {
		elements = append(elements, progressPanel("Reasoning", len(card.Reasoning), shouldExpandReasoningPanel(state, card.Reasoning), progressEntryElements(card.Reasoning, "Thinking...")))
	}
	if len(card.Tools) > 0 {
		elements = append(elements, progressPanel("Tools", len(card.Tools), running, progressEntryElements(card.Tools, "No tool calls yet")))
	}
	if running {
		elements = append(elements, map[string]any{
			"tag": "div",
			"text": map[string]any{
				"tag":        "plain_text",
				"content":    "Running...",
				"text_size":  "notation",
				"text_color": "grey",
			},
		})
	} else {
		footer := "Progress card stopped. Full response is in the next message."
		if strings.TrimSpace(card.Final) == "" {
			footer = "Progress card stopped."
		}
		elements = append(elements, map[string]any{"tag": "hr"})
		elements = append(elements, map[string]any{
			"tag": "div",
			"text": map[string]any{
				"tag":        "plain_text",
				"content":    footer,
				"text_size":  "notation",
				"text_color": "grey",
			},
		})
	}
	return map[string]any{
		"schema": "2.0",
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"template": template,
			"title":    map[string]string{"tag": "plain_text", "content": title},
		},
		"body": map[string]any{
			"elements": elements,
		},
	}
}

func shouldExpandReasoningPanel(state string, entries []ProgressCardEntry) bool {
	if state != "" && state != "running" {
		return false
	}
	const maxExpandedChars = 900
	total := 0
	for _, entry := range entries {
		total += len([]rune(entry.Title)) + len([]rune(entry.Content))
		if total > maxExpandedChars {
			return false
		}
	}
	return true
}

func progressPanel(label string, count int, expanded bool, elements []map[string]any) map[string]any {
	title := label
	if count > 0 {
		title = fmt.Sprintf("%s (%d)", label, count)
	}
	return map[string]any{
		"tag":              "collapsible_panel",
		"expanded":         expanded,
		"background_color": "grey",
		"header": map[string]any{
			"title": map[string]string{"tag": "plain_text", "content": title},
		},
		"elements": elements,
	}
}

func progressEntryElements(entries []ProgressCardEntry, empty string) []map[string]any {
	if len(entries) == 0 {
		return []map[string]any{{
			"tag": "div",
			"text": map[string]any{
				"tag":        "plain_text",
				"content":    empty,
				"text_size":  "notation",
				"text_color": "grey",
			},
		}}
	}
	const maxEntries = 8
	visible := entries
	hidden := 0
	if len(entries) > maxEntries {
		hidden = len(entries) - maxEntries
		visible = entries[hidden:]
	}
	out := make([]map[string]any, 0, len(visible)+1)
	if hidden > 0 {
		out = append(out, map[string]any{
			"tag": "div",
			"text": map[string]any{
				"tag":        "plain_text",
				"content":    fmt.Sprintf("... %d earlier steps hidden", hidden),
				"text_size":  "notation",
				"text_color": "grey",
			},
		})
	}
	for _, entry := range visible {
		content := strings.TrimSpace(entry.Title)
		if body := strings.TrimSpace(entry.Content); body != "" {
			if content != "" {
				content += "\n"
			}
			content += body
		}
		if status := strings.TrimSpace(entry.Status); status != "" {
			if content != "" {
				content += "\n"
			}
			content += "status: " + status
		}
		if content == "" {
			continue
		}
		content = trimProgressEntryContent(content)
		out = append(out, map[string]any{
			"tag": "div",
			"text": map[string]any{
				"tag":        "plain_text",
				"content":    content,
				"text_size":  "notation",
				"text_color": progressTextColor(entry),
			},
		})
	}
	if len(out) == 0 {
		return progressEntryElements(nil, empty)
	}
	return out
}

func trimProgressEntryContent(content string) string {
	content = strings.TrimSpace(content)
	const maxRunes = 600
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + "\n..."
}

func progressTextColor(entry ProgressCardEntry) string {
	if strings.EqualFold(strings.TrimSpace(entry.Kind), "thinking") {
		return "grey"
	}
	if strings.EqualFold(strings.TrimSpace(entry.Status), "failed") || strings.EqualFold(strings.TrimSpace(entry.Status), "error") {
		return "red"
	}
	return "default"
}

func cardHeaderTemplate(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	switch {
	case strings.Contains(title, "失败"), strings.Contains(title, "错误"), strings.Contains(title, "打回"), strings.Contains(title, "failed"), strings.Contains(title, "rejected"):
		return "red"
	case strings.Contains(title, "完成"), strings.Contains(title, "已提交"), strings.Contains(title, "通过"), strings.Contains(title, "completed"), strings.Contains(title, "approved"):
		return "green"
	case strings.Contains(title, "处理中"), strings.Contains(title, "正在"), strings.Contains(title, "running"):
		return "blue"
	default:
		return "blue"
	}
}

func splitCardMarkdown(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 900
	}
	var chunks []string
	var current strings.Builder
	currentRunes := 0
	flush := func() {
		if strings.TrimSpace(current.String()) == "" {
			current.Reset()
			currentRunes = 0
			return
		}
		chunks = append(chunks, strings.TrimSpace(current.String()))
		current.Reset()
		currentRunes = 0
	}
	for _, line := range strings.Split(text, "\n") {
		lineRunes := len([]rune(line)) + 1
		if currentRunes > 0 && currentRunes+lineRunes > maxRunes {
			flush()
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(line)
		currentRunes += lineRunes
	}
	flush()
	return chunks
}

func normalizeButtonStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "primary", "green":
		return "primary"
	case "danger", "red":
		return "danger"
	default:
		return "default"
	}
}

func (c OpenAPIClient) tenantAccessToken(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"app_id":     strings.TrimSpace(c.AppID),
		"app_secret": strings.TrimSpace(c.AppSecret),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.openBaseURL(), "/")+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("tenant token http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Code != 0 || parsed.TenantAccessToken == "" {
		return "", fmt.Errorf("tenant token failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return parsed.TenantAccessToken, nil
}

func (c OpenAPIClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (c OpenAPIClient) openBaseURL() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	}
	return "https://open.feishu.cn"
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

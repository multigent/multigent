package api

import (
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/imbridge"
)

type runtimeAttentionAttachmentPayload struct {
	Attachments  []imbridge.IncomingAttachment `json:"attachments"`
	SenderOpenID string                        `json:"senderOpenId,omitempty"`
}

func (s *Server) handleRuntimeAttentionAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeAgentFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeRuntimeAgentTokenRequired, "runtime agent token required")
		return
	}
	if !runtimeHasCapability(principal, "attention.use") {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime token lacks attention.use capability")
		return
	}
	workerID := s.runtimePrincipalAgentWorkerID(principal)
	if strings.TrimSpace(workerID) == "" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime agent is not linked to an agent worker")
		return
	}
	signalID := strings.TrimSpace(r.PathValue("id"))
	if signalID == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "attention signal id is required")
		return
	}
	index, err := strconv.Atoi(strings.TrimSpace(r.PathValue("index")))
	if err != nil || index <= 0 {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "attachment index must be a positive integer")
		return
	}
	signal, found, err := s.controlDB.AttentionSignalByID(principal.WorkspaceID, signalID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "attention signal not found")
		return
	}
	if strings.TrimSpace(signal.AgentWorkerID) != workerID {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "attention signal belongs to another agent")
		return
	}
	if signal.SourceKind != "im_message" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "attention signal is not an IM message")
		return
	}
	refs := rawJSONToMap(signal.RefsJSON)
	var payload runtimeAttentionAttachmentPayload
	_ = json.Unmarshal([]byte(signal.PayloadJSON), &payload)
	if index > len(payload.Attachments) {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "attachment not found")
		return
	}
	attachment := payload.Attachments[index-1]
	if strings.TrimSpace(attachment.ID) == "" && strings.TrimSpace(attachment.URL) != "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "this attention entry is a link, not a binary attachment; use its URL")
		return
	}
	bindingID := strings.TrimSpace(stringFromAny(refs["bindingId"]))
	if bindingID == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "attention signal has no channel binding")
		return
	}
	binding, found, err := s.controlDB.AgentChannelBindingByID(bindingID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !found || binding.WorkspaceID != principal.WorkspaceID || strings.TrimSpace(binding.AgentWorkerID) != workerID {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "attention channel binding belongs to another agent")
		return
	}
	log.Printf("[attention-attachment] download requested signal=%s worker=%s provider=%s index=%d type=%s attachment=%s message=%s",
		shortSensitiveHash(signalID), shortSensitiveHash(workerID), strings.TrimSpace(binding.Provider), index,
		strings.TrimSpace(attachment.Type), shortSensitiveHash(attachment.ID), shortSensitiveHash(stringFromAny(refs["messageId"])))
	download, err := s.downloadRuntimeAttentionAttachment(r, binding, refs, payload, attachment)
	if err != nil {
		log.Printf("[attention-attachment] download failed signal=%s worker=%s provider=%s index=%d type=%s attachment=%s: %v",
			shortSensitiveHash(signalID), shortSensitiveHash(workerID), strings.TrimSpace(binding.Provider), index,
			strings.TrimSpace(attachment.Type), shortSensitiveHash(attachment.ID), err)
		s.serverError(w, err)
		return
	}
	log.Printf("[attention-attachment] download succeeded signal=%s worker=%s provider=%s index=%d type=%s attachment=%s bytes=%d mime=%s filename=%q",
		shortSensitiveHash(signalID), shortSensitiveHash(workerID), strings.TrimSpace(binding.Provider), index,
		strings.TrimSpace(attachment.Type), shortSensitiveHash(attachment.ID), len(download.Data), strings.TrimSpace(download.MIME),
		safeAttachmentDownloadName(firstNonEmpty(download.FileName, attachment.Name)))
	fileName := safeAttachmentDownloadName(firstNonEmpty(download.FileName, attachment.Name, attachment.ID, fmt.Sprintf("attachment-%d", index)))
	contentType := strings.TrimSpace(firstNonEmpty(download.MIME, attachment.MIME, http.DetectContentType(download.Data)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(download.Data)))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
	w.Header().Set("X-Multigent-Attachment-Name", url.QueryEscape(fileName))
	w.Header().Set("X-Multigent-Attachment-Type", strings.TrimSpace(attachment.Type))
	w.Header().Set("X-Multigent-Attachment-ID", strings.TrimSpace(attachment.ID))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(download.Data)
}

func (s *Server) downloadRuntimeAttentionAttachment(r *http.Request, binding controldb.AgentChannelBinding, refs map[string]any, payload runtimeAttentionAttachmentPayload, attachment imbridge.IncomingAttachment) (imbridge.IncomingAttachmentDownload, error) {
	provider, ok := imbridge.LookupProvider(binding.Provider)
	if !ok {
		return imbridge.IncomingAttachmentDownload{}, fmt.Errorf("channel provider %q is not supported", binding.Provider)
	}
	downloader, ok := provider.(imbridge.AttachmentDownloader)
	if !ok {
		return imbridge.IncomingAttachmentDownload{}, fmt.Errorf("channel provider %q does not support attachment download", binding.Provider)
	}
	secret, found, err := s.controlDB.ConnectionSecret(binding.ConnectionID)
	if err != nil {
		return imbridge.IncomingAttachmentDownload{}, err
	}
	if !found {
		return imbridge.IncomingAttachmentDownload{}, fmt.Errorf("channel connection secret not found")
	}
	secrets, err := openConnectionSecret(secret)
	if err != nil {
		return imbridge.IncomingAttachmentDownload{}, err
	}
	message := imbridge.IncomingMessage{
		MessageID:    strings.TrimSpace(stringFromAny(refs["messageId"])),
		ChatID:       firstNonEmpty(stringFromAny(refs["chatId"]), stringFromAny(refs["externalChatId"])),
		ChatType:     strings.TrimSpace(stringFromAny(refs["chatType"])),
		SenderOpenID: strings.TrimSpace(payload.SenderOpenID),
	}
	return downloader.DownloadAttachment(r.Context(), secrets, message, attachment)
}

func safeAttachmentDownloadName(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\x00", ""))
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	return name
}

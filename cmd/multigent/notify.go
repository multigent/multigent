package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type notifySendRequest struct {
	To            string `json:"to"`
	Channel       string `json:"channel,omitempty"`
	Subject       string `json:"subject,omitempty"`
	Body          string `json:"body"`
	TaskID        string `json:"taskId,omitempty"`
	Urgency       string `json:"urgency,omitempty"`
	MessageFormat string `json:"messageFormat,omitempty"`
}

func newNotifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Notify humans through agent collaboration channels",
		Long: `Notify humans through the collaboration channels bound to the current
agent. This command is intended for agent runtime use: Multigent sends the
external message server-side and keeps an internal inbox copy for audit.`,
	}
	cmd.AddCommand(newNotifySendCmd())
	return cmd
}

func newNotifySendCmd() *cobra.Command {
	var req notifySendRequest
	var format string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a human notification from the current runtime agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			req.To = strings.TrimSpace(req.To)
			if req.To == "" {
				req.To = "human"
			}
			req.Body = strings.TrimSpace(req.Body)
			if req.Body == "" {
				return fmt.Errorf("body is required")
			}
			body, err := json.Marshal(req)
			if err != nil {
				return err
			}
			respBody, status, err := runtimePostJSON("/api/v1/runtime/notify", nil, body)
			if err != nil {
				return err
			}
			if resolveFormat(format) == "table" && status >= 200 && status < 300 {
				if err := printNotifySendTable(respBody); err != nil {
					return err
				}
			} else if _, err := os.Stdout.Write(append(bytes.TrimSpace(respBody), '\n')); err != nil {
				return err
			}
			if status < 200 || status >= 300 {
				return fmt.Errorf("runtime notify returned HTTP %d", status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&req.To, "to", "human", "recipient: human, owner, user:<username>, or chat:<group-name>")
	cmd.Flags().StringVar(&req.Channel, "channel", "auto", "channel provider: auto, feishu, lark, slack, telegram, discord")
	cmd.Flags().StringVar(&req.Subject, "subject", "", "notification subject")
	cmd.Flags().StringVar(&req.Body, "body", "", "notification body")
	cmd.Flags().StringVar(&req.TaskID, "task", "", "related task id")
	cmd.Flags().StringVar(&req.Urgency, "urgency", "", "urgency label: normal, review, blocking")
	cmd.Flags().StringVar(&req.MessageFormat, "message-format", "text", "message content format: text or markdown")
	cmd.Flags().StringVar(&format, "format", "", "output format: json or table")
	return cmd
}

func printNotifySendTable(body []byte) error {
	var resp struct {
		MessageID     string `json:"messageId"`
		Provider      string `json:"provider"`
		ChannelID     string `json:"channelId"`
		InternalSent  bool   `json:"internalSent"`
		ExternalSent  bool   `json:"externalSent"`
		ExternalError string `json:"externalError"`
		MessageFormat string `json:"messageFormat"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "MESSAGE_ID\tINTERNAL_SENT\tEXTERNAL_SENT\tFORMAT\tPROVIDER\tCHANNEL_ID\tEXTERNAL_ERROR\n")
	fmt.Fprintf(os.Stdout, "%s\t%t\t%t\t%s\t%s\t%s\t%s\n", resp.MessageID, resp.InternalSent, resp.ExternalSent, resp.MessageFormat, resp.Provider, resp.ChannelID, resp.ExternalError)
	return nil
}

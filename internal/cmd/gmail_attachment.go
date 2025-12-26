package cmd

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
	"google.golang.org/api/gmail/v1"
)

func newGmailAttachmentCmd(flags *rootFlags) *cobra.Command {
	var outPath string
	var name string

	cmd := &cobra.Command{
		Use:   "attachment <messageId> <attachmentId>",
		Short: "Download a single attachment",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := ui.FromContext(cmd.Context())
			account, err := requireAccount(flags)
			if err != nil {
				return err
			}
			messageID := strings.TrimSpace(args[0])
			attachmentID := strings.TrimSpace(args[1])
			if messageID == "" || attachmentID == "" {
				return usage("messageId/attachmentId required")
			}

			svc, err := newGmailService(cmd.Context(), account)
			if err != nil {
				return err
			}

			if strings.TrimSpace(outPath) == "" {
				dir, dirErr := config.EnsureGmailAttachmentsDir()
				if dirErr != nil {
					return dirErr
				}
				filename := strings.TrimSpace(name)
				if filename == "" {
					filename = "attachment.bin"
				}
				safeFilename := filepath.Base(filename)
				if safeFilename == "" || safeFilename == "." || safeFilename == ".." {
					safeFilename = "attachment.bin"
				}
				shortID := attachmentID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				destPath := filepath.Join(dir, fmt.Sprintf("%s_%s_%s", messageID, shortID, safeFilename))
				path, cached, bytes, dlErr := downloadAttachmentToPath(cmd, svc, messageID, attachmentID, destPath, -1)
				if dlErr != nil {
					return dlErr
				}
				if outfmt.IsJSON(cmd.Context()) {
					return outfmt.WriteJSON(os.Stdout, map[string]any{"path": path, "cached": cached, "bytes": bytes})
				}
				u.Out().Printf("path\t%s", path)
				u.Out().Printf("cached\t%t", cached)
				u.Out().Printf("bytes\t%d", bytes)
				return nil
			}

			path, cached, bytes, err := downloadAttachmentToPath(cmd, svc, messageID, attachmentID, outPath, -1)
			if err != nil {
				return err
			}
			if outfmt.IsJSON(cmd.Context()) {
				return outfmt.WriteJSON(os.Stdout, map[string]any{"path": path, "cached": cached, "bytes": bytes})
			}
			u.Out().Printf("path\t%s", path)
			u.Out().Printf("cached\t%t", cached)
			u.Out().Printf("bytes\t%d", bytes)
			return nil
		},
	}

	cmd.Flags().StringVar(&outPath, "out", "", "Write to a specific path (default: gogcli config dir)")
	cmd.Flags().StringVar(&name, "name", "", "Filename (only used when --out is empty)")
	return cmd
}

func downloadAttachmentToPath(
	cmd *cobra.Command,
	svc *gmail.Service,
	messageID string,
	attachmentID string,
	outPath string,
	expectedSize int64,
) (string, bool, int64, error) {
	if strings.TrimSpace(outPath) == "" {
		return "", false, 0, errors.New("missing outPath")
	}

	if expectedSize > 0 {
		if st, err := os.Stat(outPath); err == nil && st.Size() == expectedSize {
			return outPath, true, st.Size(), nil
		}
	} else if expectedSize == -1 {
		if st, err := os.Stat(outPath); err == nil && st.Size() > 0 {
			return outPath, true, st.Size(), nil
		}
	}

	body, err := svc.Users.Messages.Attachments.Get("me", messageID, attachmentID).Context(cmd.Context()).Do()
	if err != nil {
		return "", false, 0, err
	}
	if body == nil || body.Data == "" {
		return "", false, 0, errors.New("empty attachment data")
	}
	data, err := base64.RawURLEncoding.DecodeString(body.Data)
	if err != nil {
		// Gmail can return padded base64url; accept both.
		data, err = base64.URLEncoding.DecodeString(body.Data)
		if err != nil {
			return "", false, 0, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", false, 0, err
	}
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		return "", false, 0, err
	}
	return outPath, false, int64(len(data)), nil
}

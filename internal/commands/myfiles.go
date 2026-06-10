package commands

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/database"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	"github.com/dustin/go-humanize"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

const myFilesPerPage = 1

func (m *command) LoadMyFiles(d dispatcher.Dispatcher) {
	log := m.log.Named("myfiles")
	defer log.Sugar().Info("Loaded")
	d.AddHandler(handlers.NewCommand("myfiles", m.myFiles))
}

func (m *command) myFiles(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()

	if !database.IsEnabled() {
		ctx.Reply(u, ext.ReplyTextString("❌ Database not configured."), nil)
		return dispatcher.EndGroups
	}

	if database.GetDB().IsUserBanned(context.Background(), chatId) {
		ctx.Reply(u, ext.ReplyTextStyledTextArray([]styling.StyledTextOption{
			styling.Italic("Sᴏʀʀʏ Sɪʀ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ ᴛᴏ ᴜsᴇ ᴍᴇ.\n\n"),
			styling.TextURL("Cᴏɴᴛᴀᴄᴛ Dᴇᴠᴇʟᴏᴘᴇʀ", getUpdatesURL()),
			styling.Bold(" Tʜᴇʏ Wɪʟʟ Hᴇʟᴘ Yᴏᴜ"),
		}), nil)
		return dispatcher.EndGroups
	}

	return m.sendFileCard(ctx, u, chatId, 0, false, 0)
}

func (m *command) sendFileCard(ctx *ext.Context, u *ext.Update, userID int64, page int, isEdit bool, msgEditID int) error {
	db := database.GetDB()
	files, total, err := db.GetUserFiles(context.Background(), userID, page, myFilesPerPage)
	if err != nil || total == 0 || len(files) == 0 {
		msg := []styling.StyledTextOption{
			styling.Bold("Yᴏᴜ ʜᴀᴠᴇ ɴᴏ ꜰɪʟᴇs ʏᴇᴛ.\n\n"),
			styling.Plain("Send me any media to generate a stream link."),
		}
		if isEdit {
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:    &tg.InputPeerUser{UserID: userID},
				ID:      msgEditID,
				Message: "Yᴏᴜ ʜᴀᴠᴇ ɴᴏ ꜰɪʟᴇs ʏᴇᴛ.\n\nSend me any media to generate a stream link.",
			})
		} else {
			ctx.Reply(u, ext.ReplyTextStyledTextArray(msg), nil)
		}
		return dispatcher.EndGroups
	}

	totalPages := int(math.Ceil(float64(total) / float64(myFilesPerPage)))
	f := files[0]

	streamLink := f.Link
	downloadLink := streamLink + "&d=true"
	botUsername := ctx.Self.Username
	shareLink := fmt.Sprintf("https://t.me/%s?start=file_%d_%s", botUsername, f.MessageID, f.Hash)

	isVideo := strings.Contains(f.MimeType, "video")
	isMedia := isVideo ||
		strings.Contains(f.MimeType, "audio") ||
		strings.Contains(f.MimeType, "pdf")

	humanSize := humanize.IBytes(uint64(f.FileSize))
	emoji := fileEmoji(f.MimeType, f.FileName)

	// Build styled card text
	parts := []styling.StyledTextOption{
		styling.Bold(fmt.Sprintf("%s Yᴏᴜʀ Fɪʟᴇ\n\n", emoji)),
		styling.Bold("📂 Nᴀᴍᴇ : "), styling.Bold(f.FileName), styling.Plain("\n\n"),
		styling.Bold("📦 Sɪᴢᴇ : "), styling.Code(humanSize), styling.Plain("\n\n"),
		styling.Bold("📥 Dᴏᴡɴʟᴏᴀᴅ :\n"), styling.Code(downloadLink), styling.Plain("\n\n"),
	}
	if isMedia {
		parts = append(parts,
			styling.Bold("🖥 Wᴀᴛᴄʜ/Sᴛʀᴇᴀᴍ :\n"), styling.Code(streamLink), styling.Plain("\n\n"),
		)
	}
	parts = append(parts,
		styling.Bold("🔗 Sʜᴀʀᴇ :\n"), styling.Code(shareLink), styling.Plain("\n\n"),
		styling.Italic(fmt.Sprintf("Pᴀɢᴇ %d ᴏꜰ %d", page+1, totalPages)),
	)

	// Row 1
	var linkRow tg.KeyboardButtonRow
	if isVideo {
		linkRow = tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{Text: "🖥 sᴛʀᴇᴀᴍ", URL: streamLink},
			&tg.KeyboardButtonURL{Text: "📥 ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
		}}
	} else {
		linkRow = tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{Text: "📥 ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
			&tg.KeyboardButtonURL{Text: "🔗 sʜᴀʀᴇ", URL: shareLink},
		}}
	}

	shareRow := tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
		&tg.KeyboardButtonURL{Text: "🔗 sʜᴀʀᴇ ʟɪɴᴋ", URL: shareLink},
	}}

	isFirst := page == 0
	isLast := page >= totalPages-1
	var navButtons []tg.KeyboardButtonClass
	if !isFirst {
		navButtons = append(navButtons, &tg.KeyboardButtonCallback{
			Text: "◀ Pʀᴇᴠɪᴏᴜs",
			Data: []byte(fmt.Sprintf("mf_page_%d", page-1)),
		})
	}
	if !isLast {
		navButtons = append(navButtons, &tg.KeyboardButtonCallback{
			Text: "Nᴇxᴛ ▶",
			Data: []byte(fmt.Sprintf("mf_page_%d", page+1)),
		})
	}

	rows := []tg.KeyboardButtonRow{linkRow}
	if isVideo {
		rows = append(rows, shareRow)
	}
	if len(navButtons) > 0 {
		rows = append(rows, tg.KeyboardButtonRow{Buttons: navButtons})
	}
	if config.ValueOf.UpdatesChannel != "" {
		rows = append(rows, tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{Text: "📢 ᴜᴘᴅᴀᴛᴇs ᴄʜᴀɴɴᴇʟ", URL: getUpdatesURL()},
		}})
	}
	markup := &tg.ReplyInlineMarkup{Rows: rows}

	if isEdit {
		// For edits we fall back to plain text since MessagesEditMessage doesn't
		// have a styled text helper — build a plain version without markdown signs.
		plainText := buildPlainCard(emoji, f.FileName, humanSize, downloadLink, streamLink, shareLink, isMedia, page+1, totalPages)
		ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
			Peer:        &tg.InputPeerUser{UserID: userID},
			ID:          msgEditID,
			Message:     plainText,
			ReplyMarkup: markup,
			NoWebpage:   true,
		})
		return dispatcher.EndGroups
	}

	// First send: try to attach the actual file thumbnail
	sent := false
	if config.ValueOf.LogChannelID != 0 {
		tgMsg, fetchErr := m.getLogMessage(ctx, f.MessageID)
		if fetchErr == nil {
			var inputMedia tg.InputMediaClass
			switch media := tgMsg.Media.(type) {
			case *tg.MessageMediaDocument:
				if doc, ok := media.Document.AsNotEmpty(); ok {
					inputMedia = &tg.InputMediaDocument{
						ID: &tg.InputDocument{
							ID:            doc.ID,
							AccessHash:    doc.AccessHash,
							FileReference: doc.FileReference,
						},
					}
				}
			case *tg.MessageMediaPhoto:
				if photo, ok := media.Photo.AsNotEmpty(); ok {
					inputMedia = &tg.InputMediaPhoto{
						ID: &tg.InputPhoto{
							ID:            photo.ID,
							AccessHash:    photo.AccessHash,
							FileReference: photo.FileReference,
						},
					}
				}
			}
			if inputMedia != nil {
				// Build plain caption for media send
				plainText := buildPlainCard(emoji, f.FileName, humanSize, downloadLink, streamLink, shareLink, isMedia, page+1, totalPages)
				_, sendErr := ctx.Raw.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
					Peer:        &tg.InputPeerUser{UserID: userID},
					Media:       inputMedia,
					Message:     plainText,
					ReplyMarkup: markup,
				})
				if sendErr == nil {
					sent = true
				}
			}
		}
	}
	if !sent {
		ctx.Reply(u, ext.ReplyTextStyledTextArray(parts), &ext.ReplyOpts{Markup: markup})
	}

	return dispatcher.EndGroups
}

// buildPlainCard builds a clean card string with no markdown asterisks or underscores.
func buildPlainCard(emoji, fileName, humanSize, downloadLink, streamLink, shareLink string, isMedia bool, page, totalPages int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s Your File\n\n", emoji))
	sb.WriteString(fmt.Sprintf("Name : %s\n\n", fileName))
	sb.WriteString(fmt.Sprintf("Size : %s\n\n", humanSize))
	sb.WriteString(fmt.Sprintf("Download :\n%s\n\n", downloadLink))
	if isMedia {
		sb.WriteString(fmt.Sprintf("Watch/Stream :\n%s\n\n", streamLink))
	}
	sb.WriteString(fmt.Sprintf("Share :\n%s\n\n", shareLink))
	sb.WriteString(fmt.Sprintf("Page %d of %d", page, totalPages))
	return sb.String()
}

func fileEmoji(mimeType, fileName string) string {
	switch {
	case strings.Contains(mimeType, "video"):
		return "🎬"
	case strings.Contains(mimeType, "audio"):
		return "🎵"
	case strings.Contains(mimeType, "pdf"):
		return "📄"
	case strings.Contains(mimeType, "image"):
		return "🖼"
	case strings.Contains(mimeType, "zip"), strings.Contains(mimeType, "rar"),
		strings.Contains(mimeType, "tar"), strings.Contains(mimeType, "7z"):
		return "🗜"
	case strings.HasSuffix(strings.ToLower(fileName), ".apk"):
		return "📱"
	default:
		return "📁"
	}
}

func (m *command) getLogMessage(ctx *ext.Context, messageID int) (*tg.Message, error) {
	channel, err := m.getLogChannelInput(ctx)
	if err != nil {
		return nil, err
	}
	res, err := ctx.Raw.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: channel,
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: messageID}},
	})
	if err != nil {
		return nil, err
	}
	msgs, ok := res.(*tg.MessagesChannelMessages)
	if !ok || len(msgs.Messages) == 0 {
		return nil, fmt.Errorf("no message found")
	}
	msg, ok := msgs.Messages[0].(*tg.Message)
	if !ok {
		return nil, fmt.Errorf("unexpected message type")
	}
	return msg, nil
}

func (m *command) getLogChannelInput(ctx *ext.Context) (*tg.InputChannel, error) {
	cached := ctx.PeerStorage.GetInputPeerById(config.ValueOf.LogChannelID)
	if peer, ok := cached.(*tg.InputPeerChannel); ok {
		return &tg.InputChannel{ChannelID: peer.ChannelID, AccessHash: peer.AccessHash}, nil
	}
	channels, err := ctx.Raw.ChannelsGetChannels(ctx, []tg.InputChannelClass{
		&tg.InputChannel{ChannelID: config.ValueOf.LogChannelID},
	})
	if err != nil {
		return nil, err
	}
	chats := channels.GetChats()
	if len(chats) == 0 {
		return nil, fmt.Errorf("log channel not found")
	}
	ch, ok := chats[0].(*tg.Channel)
	if !ok {
		return nil, fmt.Errorf("unexpected channel type")
	}
	ctx.PeerStorage.AddPeer(ch.GetID(), ch.AccessHash, storage.TypeChannel, "")
	return ch.AsInput(), nil
}

func (m *command) myFilesPageCallback(ctx *ext.Context, u *ext.Update, pageStr string) error {
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 0 {
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: u.CallbackQuery.QueryID,
			Message: "Invalid page",
			Alert:   true,
		})
		return dispatcher.EndGroups
	}

	userID := u.EffectiveChat().GetID()

	ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
		QueryID: u.CallbackQuery.QueryID,
	})

	msgID := u.CallbackQuery.MsgID
	return m.sendFileCard(ctx, u, userID, page, true, msgID)
}

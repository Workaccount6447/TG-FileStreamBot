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
	"github.com/dustin/go-humanize"
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
		ctx.Reply(u, ext.ReplyTextString(
			fmt.Sprintf("__Sᴏʀʀʏ Sɪʀ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ ᴛᴏ ᴜsᴇ ᴍᴇ.__\n\n**[Cᴏɴᴛᴀᴄᴛ Dᴇᴠᴇʟᴏᴘᴇʀ](%s) Tʜᴇʏ Wɪʟʟ Hᴇʟᴘ Yᴏᴜ**", getUpdatesURL()),
		), nil)
		return dispatcher.EndGroups
	}

	return m.sendFileCard(ctx, u, chatId, 0, false, 0)
}

// sendFileCard fetches page `page` for `userID` and either:
//   - sends a new message  (isEdit=false) — used by /myfiles
//   - edits existing message (isEdit=true) — used by prev/next callbacks
func (m *command) sendFileCard(ctx *ext.Context, u *ext.Update, userID int64, page int, isEdit bool, msgEditID int) error {
	db := database.GetDB()
	files, total, err := db.GetUserFiles(context.Background(), userID, page, myFilesPerPage)
	if err != nil || total == 0 || len(files) == 0 {
		msg := "**Yᴏᴜ ʜᴀᴠᴇ ɴᴏ ꜰɪʟᴇs ʏᴇᴛ.**\n\nSend me any media to generate a stream link."
		if isEdit {
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:    &tg.InputPeerUser{UserID: userID},
				ID:      msgEditID,
				Message: msg,
			})
		} else {
			ctx.Reply(u, ext.ReplyTextString(msg), nil)
		}
		return dispatcher.EndGroups
	}

	totalPages := int(math.Ceil(float64(total) / float64(myFilesPerPage)))
	f := files[0]

	// Rebuild links from stored data
	streamLink := f.Link
	downloadLink := streamLink + "&d=true"
	botUsername := ctx.Self.Username
	shareLink := fmt.Sprintf("https://t.me/%s?start=file_%d_%s", botUsername, f.MessageID, f.Hash)

	isVideo := strings.Contains(f.MimeType, "video")
	// FIX Bug D: isAudio and isPDF were declared but only used via isMedia,
	// causing a compile error. Now computed inline inside isMedia directly.
	isMedia := isVideo ||
		strings.Contains(f.MimeType, "audio") ||
		strings.Contains(f.MimeType, "pdf")

	humanSize := humanize.IBytes(uint64(f.FileSize))
	emoji := fileEmoji(f.MimeType, f.FileName)

	// ── Card text ──────────────────────────────────────────────────────────
	var cardText string
	if isMedia {
		cardText = fmt.Sprintf(
			"<i><u>%s Yᴏᴜʀ Fɪʟᴇ</u></i>\n\n"+
				"**📂 Nᴀᴍᴇ :** **%s**\n\n"+
				"**📦 Sɪᴢᴇ :** `%s`\n\n"+
				"**📥 Dᴏᴡɴʟᴏᴀᴅ :**\n`%s`\n\n"+
				"**🖥 Wᴀᴛᴄʜ/Sᴛʀᴇᴀᴍ :**\n`%s`\n\n"+
				"**🔗 Sʜᴀʀᴇ :**\n`%s`\n\n"+
				"__Pᴀɢᴇ %d ᴏꜰ %d__",
			emoji, f.FileName, humanSize,
			downloadLink, streamLink, shareLink,
			page+1, totalPages,
		)
	} else {
		cardText = fmt.Sprintf(
			"<i><u>%s Yᴏᴜʀ Fɪʟᴇ</u></i>\n\n"+
				"**📂 Nᴀᴍᴇ :** **%s**\n\n"+
				"**📦 Sɪᴢᴇ :** `%s`\n\n"+
				"**📥 Dᴏᴡɴʟᴏᴀᴅ :**\n`%s`\n\n"+
				"**🔗 Sʜᴀʀᴇ :**\n`%s`\n\n"+
				"__Pᴀɢᴇ %d ᴏꜰ %d__",
			emoji, f.FileName, humanSize,
			downloadLink, shareLink,
			page+1, totalPages,
		)
	}

	// ── Row 1: Stream + Download (video) or Download + Share (others) ─────
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

	// ── Row 2: Share link row (video only, separate row) ──────────────────
	shareRow := tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
		&tg.KeyboardButtonURL{Text: "🔗 sʜᴀʀᴇ ʟɪɴᴋ", URL: shareLink},
	}}

	// ── Pagination row ────────────────────────────────────────────────────
	// First page only  → [Nᴇxᴛ ▶]
	// Middle page      → [◀ Pʀᴇᴠɪᴏᴜs | Nᴇxᴛ ▶]
	// Last page only   → [◀ Pʀᴇᴠɪᴏᴜs]
	// Single page      → (no nav row)
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

	// ── Updates channel row ───────────────────────────────────────────────
	updRow := tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
		&tg.KeyboardButtonURL{Text: "📢 ᴜᴘᴅᴀᴛᴇs ᴄʜᴀɴɴᴇʟ", URL: getUpdatesURL()},
	}}

	// Assemble markup
	rows := []tg.KeyboardButtonRow{linkRow}
	if isVideo {
		rows = append(rows, shareRow)
	}
	if len(navButtons) > 0 {
		rows = append(rows, tg.KeyboardButtonRow{Buttons: navButtons})
	}
	rows = append(rows, updRow)
	markup := &tg.ReplyInlineMarkup{Rows: rows}

	if isEdit {
		// Editing a message that may have media — use NoWebpage flag to avoid
		// Telegram trying to re-embed a link preview on edit.
		// msgEditID is passed explicitly because in callback updates
		// u.EffectiveMessage is nil — the message ID comes from CallbackQuery.MsgID.
		ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
			Peer:        &tg.InputPeerUser{UserID: userID},
			ID:          msgEditID,
			Message:     cardText,
			ReplyMarkup: markup,
			NoWebpage:   true,
		})
		return dispatcher.EndGroups
	}

	// First send: try to attach the actual file thumbnail from the log channel
	// so the user sees the image/thumbnail alongside the text card.
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
				_, sendErr := ctx.Raw.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
					Peer:        &tg.InputPeerUser{UserID: userID},
					Media:       inputMedia,
					Message:     cardText,
					ReplyMarkup: markup,
				})
				if sendErr == nil {
					sent = true
				}
			}
		}
	}
	if !sent {
		// Fallback: text-only card
		ctx.Reply(u, ext.ReplyTextString(cardText), &ext.ReplyOpts{Markup: markup})
	}

	return dispatcher.EndGroups
}

// fileEmoji returns a fitting emoji for the file card header
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

// getLogMessage fetches a single message from the log channel by its ID.
// It is a method on *command so it has access to ctx without passing it as a
// plain function parameter (avoids the unused-variable compile error).
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

// getLogChannelInput resolves the log channel input from peer storage or
// falls back to a live API call when not yet cached.
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
	ctx.PeerStorage.AddPeer(ch.GetID(), ch.AccessHash, 0, "")
	return ch.AsInput(), nil
}

// myFilesPageCallback is called by handleCallback in start.go when the
// callback data starts with "mf_page_".
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

	// Answer immediately to remove the Telegram loading spinner
	ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
		QueryID: u.CallbackQuery.QueryID,
	})

	// u.EffectiveMessage is nil for callback updates — get message ID from CallbackQuery
	msgID := u.CallbackQuery.MsgID
	return m.sendFileCard(ctx, u, userID, page, true, msgID)
}

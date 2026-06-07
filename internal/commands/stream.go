package commands

import (
	"context"
	"fmt"
	"strings"

	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/database"
	"EverythingSuckz/fsb/internal/utils"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	"github.com/celestix/gotgproto/types"
	"github.com/dustin/go-humanize"
	"github.com/gotd/td/tg"
)

func (m *command) LoadStream(dispatcher dispatcher.Dispatcher) {
	log := m.log.Named("stream")
	defer log.Sugar().Info("Loaded")
	dispatcher.AddHandler(
		handlers.NewMessage(nil, sendLink),
	)
}

func supportedMediaFilter(m *types.Message) (bool, error) {
	if not := m.Media == nil; not {
		return false, dispatcher.EndGroups
	}
	switch m.Media.(type) {
	case *tg.MessageMediaDocument:
		return true, nil
	case *tg.MessageMediaPhoto:
		return true, nil
	case tg.MessageMediaClass:
		return false, dispatcher.EndGroups
	default:
		return false, nil
	}
}

func sendLink(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()
	peerChatId := ctx.PeerStorage.GetPeerById(chatId)
	if peerChatId.Type != int(storage.TypeUser) {
		return dispatcher.EndGroups
	}

	// Ban check
	if database.IsEnabled() {
		db := database.GetDB()
		if db.IsUserBanned(context.Background(), chatId) {
			ctx.Reply(u, ext.ReplyTextString(
				fmt.Sprintf(
					"__Sᴏʀʀʏ Sɪʀ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ ᴛᴏ ᴜsᴇ ᴍᴇ.__\n\n**[Cᴏɴᴛᴀᴄᴛ Dᴇᴠᴇʟᴏᴘᴇʀ](%s) Tʜᴇʏ Wɪʟʟ Hᴇʟᴘ Yᴏᴜ**",
					getUpdatesURL(),
				),
			), nil)
			return dispatcher.EndGroups
		}
	}

	// Allowed users check
	if len(config.ValueOf.AllowedUsers) != 0 && !utils.Contains(config.ValueOf.AllowedUsers, chatId) {
		ctx.Reply(u, ext.ReplyTextString("Yᴏᴜ ᴀʀᴇ ɴᴏᴛ ᴀᴜᴛʜᴏʀɪᴢᴇᴅ ᴛᴏ ᴜsᴇ ᴛʜɪs ʙᴏᴛ."), nil)
		return dispatcher.EndGroups
	}

	supported, err := supportedMediaFilter(u.EffectiveMessage)
	if err != nil {
		return err
	}
	if !supported {
		ctx.Reply(u, ext.ReplyTextString("Sorry, this message type is unsupported."), nil)
		return dispatcher.EndGroups
	}

	// Forward file to log channel (BACKEND — untouched)
	update, err := utils.ForwardMessages(ctx, chatId, config.ValueOf.LogChannelID, u.EffectiveMessage.ID)
	if err != nil {
		utils.Logger.Sugar().Error(err)
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	if len(update.Updates) < 2 {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected update structure from Telegram"), nil)
		return dispatcher.EndGroups
	}
	msgIDUpdate, ok := update.Updates[0].(*tg.UpdateMessageID)
	if !ok {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected update type"), nil)
		return dispatcher.EndGroups
	}
	messageID := msgIDUpdate.ID
	newMsg, ok := update.Updates[1].(*tg.UpdateNewChannelMessage)
	if !ok {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected channel message update"), nil)
		return dispatcher.EndGroups
	}
	msg, ok := newMsg.Message.(*tg.Message)
	if !ok {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected message type"), nil)
		return dispatcher.EndGroups
	}
	file, err := utils.FileFromMedia(msg.Media)
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
		return dispatcher.EndGroups
	}

	// Hash & link generation (BACKEND — untouched)
	fullHash := utils.PackFile(file.FileName, file.FileSize, file.MimeType, file.ID)
	hash := utils.GetShortHash(fullHash)
	streamLink := fmt.Sprintf("%s/stream/%d?hash=%s", config.ValueOf.Host, messageID, hash)
	downloadLink := streamLink + "&d=true"

	// Track in database — store full metadata for /myfiles
	if database.IsEnabled() {
		db := database.GetDB()
		_ = db.AddFileLink(context.Background(), database.FileLink{
			UserID:    chatId,
			Link:      streamLink,
			FileName:  file.FileName,
			FileSize:  file.FileSize,
			MimeType:  file.MimeType,
			MessageID: messageID,
			Hash:      hash,
		})
		db.IncrLinks(context.Background(), chatId)
	}

	// Human-readable file size
	fileSize := humanize.IBytes(uint64(file.FileSize))

	// Share link
	botUsername := ctx.Self.Username
	shareLink := fmt.Sprintf("https://t.me/%s?start=file_%d_%s", botUsername, messageID, hash)

	isVideo := strings.Contains(file.MimeType, "video")
	isMedia := isVideo ||
		strings.Contains(file.MimeType, "audio") ||
		strings.Contains(file.MimeType, "pdf")

	// ── Reply text — matches Python FileStreamBot STREAM_TEXT / STREAM_TEXT_X ──
	var replyText string
	if isMedia {
		replyText = fmt.Sprintf(
			"<i><u>𝗬𝗼𝘂𝗿 𝗟𝗶𝗻𝗸 𝗚𝗲𝗻𝗲𝗿𝗮𝘁𝗲𝗱 !</u></i>\n\n"+
				"**📂 Fɪʟᴇ ɴᴀᴍᴇ :** **%s**\n\n"+
				"**📦 Fɪʟᴇ ꜱɪᴢᴇ :** `%s`\n\n"+
				"**📥 Dᴏᴡɴʟᴏᴀᴅ :** `%s`\n\n"+
				"**🖥 Wᴀᴛᴄʜ :** `%s`\n\n"+
				"**🔗 Sʜᴀʀᴇ :** `%s`\n\n"+
				"Oᴘᴇɴ ᴛʜɪs ʟɪɴᴋ ᴏɴ Bʀᴏᴡsᴇʀ 🌐 ᴛᴏ ᴀᴠᴏɪᴅ ɪssᴜᴇs.",
			file.FileName, fileSize, downloadLink, streamLink, shareLink,
		)
	} else {
		replyText = fmt.Sprintf(
			"<i><u>𝗬𝗼𝘂𝗿 𝗟𝗶𝗻𝗸 𝗚𝗲𝗻𝗲𝗿𝗮𝘁𝗲𝗱 !</u></i>\n\n"+
				"**📂 Fɪʟᴇ ɴᴀᴍᴇ :** **%s**\n\n"+
				"**📦 Fɪʟᴇ ꜱɪᴢᴇ :** `%s`\n\n"+
				"**📥 Dᴏᴡɴʟᴏᴀᴅ :** `%s`\n\n"+
				"**🔗 Sʜᴀʀᴇ :** `%s`\n\n"+
				"Oᴘᴇɴ ᴛʜɪs ʟɪɴᴋ ᴏɴ Bʀᴏᴡsᴇʀ 🌐 ᴛᴏ ᴀᴠᴏɪᴅ ɪssᴜᴇs.",
			file.FileName, fileSize, downloadLink, shareLink,
		)
	}

	// ── Inline buttons — matches Python FileStreamBot button layout ──
	var rows []tg.KeyboardButtonRow
	if isVideo {
		// Video: Stream + Download on row 1
		rows = append(rows, tg.KeyboardButtonRow{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonURL{Text: "sᴛʀᴇᴀᴍ", URL: streamLink},
				&tg.KeyboardButtonURL{Text: "ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
			},
		})
	} else if isMedia {
		// Audio/PDF: Download only on row 1
		rows = append(rows, tg.KeyboardButtonRow{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonURL{Text: "ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
			},
		})
	} else {
		// Other: Download only
		rows = append(rows, tg.KeyboardButtonRow{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonURL{Text: "ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
			},
		})
	}

	// Row 2: Get File (share link) + Revoke File
	rows = append(rows, tg.KeyboardButtonRow{
		Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{Text: "ɢᴇᴛ ғɪʟᴇ", URL: shareLink},
			&tg.KeyboardButtonCallback{Text: "ʀᴇᴠᴏᴋᴇ ғɪʟᴇ", Data: []byte(fmt.Sprintf("revoke_%d", messageID))},
		},
	})
	// Row 3: Close
	rows = append(rows, tg.KeyboardButtonRow{
		Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonCallback{Text: "ᴄʟᴏsᴇ", Data: []byte("close")},
		},
	})

	markup := &tg.ReplyInlineMarkup{Rows: rows}

	if strings.Contains(streamLink, "http://localhost") {
		_, err = ctx.Reply(u, ext.ReplyTextString(replyText), &ext.ReplyOpts{
			ReplyToMessageId: u.EffectiveMessage.ID,
		})
	} else {
		_, err = ctx.Reply(u, ext.ReplyTextString(replyText), &ext.ReplyOpts{
			Markup:           markup,
			ReplyToMessageId: u.EffectiveMessage.ID,
		})
	}
	if err != nil {
		utils.Logger.Sugar().Error(err)
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
	}
	return dispatcher.EndGroups
}

// ── Feature #3: Channel post auto-link ────────────────────────────────────
//
// When the bot is admin in a channel and media is posted, it automatically
// edits the post to add Stream + Download inline buttons — exactly matching
// the Python FileStreamBot channel handler behaviour.
//
// How it works:
//  1. LoadChannelStream registers a handler for ALL channel messages
//  2. The handler checks the peer is a channel (not a private chat)
//  3. It forwards the message to the log channel to get a stable message ID
//  4. It generates stream/download links from the forwarded message
//  5. It edits the original channel post to add the inline buttons
//
// Nothing in the streaming backend is touched — only the post is edited.

func (m *command) LoadChannelStream(d dispatcher.Dispatcher) {
	log := m.log.Named("channel_stream")
	defer log.Sugar().Info("Loaded")
	d.AddHandler(handlers.NewMessage(nil, m.channelAutoLink))
}

func (m *command) channelAutoLink(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()

	// Only fire for channel messages — ignore private chats (handled by sendLink)
	peerType := ctx.PeerStorage.GetPeerById(chatId).Type
	if peerType != int(storage.TypeChannel) {
		return dispatcher.EndGroups
	}

	// Ignore messages without media
	supported, err := supportedMediaFilter(u.EffectiveMessage)
	if err != nil || !supported {
		return dispatcher.EndGroups
	}

	// Ignore messages that already have inline buttons (already processed)
	if u.EffectiveMessage.ReplyMarkup != nil {
		return dispatcher.EndGroups
	}

	// Forward to log channel to get a stable message ID for the stream route
	update, err := utils.ForwardMessages(ctx, chatId, config.ValueOf.LogChannelID, u.EffectiveMessage.ID)
	if err != nil || len(update.Updates) < 2 {
		return dispatcher.EndGroups
	}
	msgIDUpdate, ok := update.Updates[0].(*tg.UpdateMessageID)
	if !ok {
		return dispatcher.EndGroups
	}
	newMsg, ok := update.Updates[1].(*tg.UpdateNewChannelMessage)
	if !ok {
		return dispatcher.EndGroups
	}
	msg, ok := newMsg.Message.(*tg.Message)
	if !ok {
		return dispatcher.EndGroups
	}
	file, err := utils.FileFromMedia(msg.Media)
	if err != nil {
		return dispatcher.EndGroups
	}

	messageID := msgIDUpdate.ID
	fullHash := utils.PackFile(file.FileName, file.FileSize, file.MimeType, file.ID)
	hash := utils.GetShortHash(fullHash)
	streamLink := fmt.Sprintf("%s/stream/%d?hash=%s", config.ValueOf.Host, messageID, hash)
	downloadLink := streamLink + "&d=true"

	isVideo := strings.Contains(file.MimeType, "video")

	// Build button rows — same layout as private-chat stream links
	var rows []tg.KeyboardButtonRow
	if isVideo {
		rows = append(rows, tg.KeyboardButtonRow{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonURL{Text: "🖥 sᴛʀᴇᴀᴍ", URL: streamLink},
				&tg.KeyboardButtonURL{Text: "📥 ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
			},
		})
	} else {
		rows = append(rows, tg.KeyboardButtonRow{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonURL{Text: "📥 ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
			},
		})
	}

	// Edit the original channel post to add the buttons
	channelPeer := ctx.PeerStorage.GetInputPeerById(chatId)
	ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:        channelPeer,
		ID:          u.EffectiveMessage.ID,
		Message:     u.EffectiveMessage.Text,
		ReplyMarkup: &tg.ReplyInlineMarkup{Rows: rows},
	})

	return dispatcher.EndGroups
}

package commands

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/database"
	"EverythingSuckz/fsb/internal/utils"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	"github.com/dustin/go-humanize"
	"github.com/gotd/td/telegram/message/entity"
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

// styledToRaw converts styling options into raw text + entities for MessagesEditMessage.
func styledToRaw(parts []styling.StyledTextOption) (string, []tg.MessageEntityClass) {
	tb := entity.Builder{}
	_ = styling.Perform(&tb, parts...)
	text, entities := tb.Complete()
	return text, entities
}

func (m *command) sendFileCard(ctx *ext.Context, u *ext.Update, userID int64, page int, isEdit bool, msgEditID int) error {
	db := database.GetDB()
	records, total, err := db.GetUserFiles(context.Background(), userID, page, myFilesPerPage)
	if err != nil || total == 0 || len(records) == 0 {
		emptyParts := []styling.StyledTextOption{
			styling.Bold("Yᴏᴜ ʜᴀᴠᴇ ɴᴏ ꜰɪʟᴇs ʏᴇᴛ.\n\n"),
			styling.Plain("Send me any media to generate a stream link."),
		}
		if isEdit {
			text, ents := styledToRaw(emptyParts)
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer: &tg.InputPeerUser{UserID: userID}, ID: msgEditID,
				Message: text, Entities: ents,
			})
		} else {
			ctx.Reply(u, ext.ReplyTextStyledTextArray(emptyParts), nil)
		}
		return dispatcher.EndGroups
	}

	totalPages := int(math.Ceil(float64(total) / float64(myFilesPerPage)))
	rec := records[0]

	// Fetch file info live from the log channel message
	tgMsg, fetchErr := m.getLogMessage(ctx, rec.MessageID)
	if fetchErr != nil {
		ctx.Reply(u, ext.ReplyTextString("❌ Could not fetch file info. The file may have been deleted from the log channel."), nil)
		return dispatcher.EndGroups
	}
	file, fetchErr := utils.FileFromMedia(tgMsg.Media)
	if fetchErr != nil {
		ctx.Reply(u, ext.ReplyTextString("❌ Could not read file info."), nil)
		return dispatcher.EndGroups
	}

	// Rebuild links from live data
	fullHash := utils.PackFile(file.FileName, file.FileSize, file.MimeType, file.ID)
	hash := utils.GetShortHash(fullHash)
	streamLink := fmt.Sprintf("%s/stream/%d?hash=%s", config.ValueOf.Host, rec.MessageID, hash)
	downloadLink := streamLink + "&d=true"
	botUsername := ctx.Self.Username
	shareLink := fmt.Sprintf("https://t.me/%s?start=file_%d_%s", botUsername, rec.MessageID, hash)

	isVideo := strings.Contains(file.MimeType, "video")
	isMedia := isVideo ||
		strings.Contains(file.MimeType, "audio") ||
		strings.Contains(file.MimeType, "pdf")

	humanSize := humanize.IBytes(uint64(file.FileSize))
	emoji := fileEmoji(file.MimeType, file.FileName)

	parts := buildCardParts(emoji, file.FileName, humanSize, downloadLink, streamLink, shareLink, isMedia, page+1, totalPages)

	// Buttons
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
		text, ents := styledToRaw(parts)
		ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
			Peer:        &tg.InputPeerUser{UserID: userID},
			ID:          msgEditID,
			Message:     text,
			Entities:    ents,
			ReplyMarkup: markup,
			NoWebpage:   true,
		})
		return dispatcher.EndGroups
	}

	// First send: attach the actual file as thumbnail
	var inputMedia tg.InputMediaClass
	switch media := tgMsg.Media.(type) {
	case *tg.MessageMediaDocument:
		if doc, ok := media.Document.AsNotEmpty(); ok {
			inputMedia = &tg.InputMediaDocument{ID: &tg.InputDocument{
				ID: doc.ID, AccessHash: doc.AccessHash, FileReference: doc.FileReference,
			}}
		}
	case *tg.MessageMediaPhoto:
		if photo, ok := media.Photo.AsNotEmpty(); ok {
			inputMedia = &tg.InputMediaPhoto{ID: &tg.InputPhoto{
				ID: photo.ID, AccessHash: photo.AccessHash, FileReference: photo.FileReference,
			}}
		}
	}

	if inputMedia != nil {
		text, ents := styledToRaw(parts)
		_, sendErr := ctx.Raw.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
			Peer:        &tg.InputPeerUser{UserID: userID},
			Media:       inputMedia,
			Message:     text,
			Entities:    ents,
			ReplyMarkup: markup,
		})
		if sendErr == nil {
			return dispatcher.EndGroups
		}
	}
	ctx.Reply(u, ext.ReplyTextStyledTextArray(parts), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

func buildCardParts(emoji, fileName, humanSize, downloadLink, streamLink, shareLink string, isMedia bool, page, totalPages int) []styling.StyledTextOption {
	parts := []styling.StyledTextOption{
		styling.Bold(fmt.Sprintf("%s Yᴏᴜʀ Fɪʟᴇ\n\n", emoji)),
		styling.Bold("📂 Nᴀᴍᴇ : "), styling.Bold(fileName), styling.Plain("\n\n"),
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
		styling.Italic(fmt.Sprintf("Pᴀɢᴇ %d ᴏꜰ %d", page, totalPages)),
	)
	return parts
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
			QueryID: u.CallbackQuery.QueryID, Message: "Invalid page", Alert: true,
		})
		return dispatcher.EndGroups
	}
	userID := u.EffectiveChat().GetID()
	ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{QueryID: u.CallbackQuery.QueryID})
	return m.sendFileCard(ctx, u, userID, page, true, u.CallbackQuery.MsgID)
}

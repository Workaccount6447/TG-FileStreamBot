package commands

import (
	"context"
	"fmt"
	"math/rand"
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
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

func (m *command) LoadStream(dispatcher dispatcher.Dispatcher) {
	log := m.log.Named("stream")
	defer log.Sugar().Info("Loaded")
	dispatcher.AddHandler(handlers.NewMessage(nil, m.sendLink))
}

func supportedMediaFilter(m *types.Message) (bool, error) {
	if m.Media == nil {
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

func (m *command) sendLink(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()
	if ctx.PeerStorage.GetPeerById(chatId).Type != int(storage.TypeUser) {
		return dispatcher.EndGroups
	}

	if database.IsEnabled() {
		if database.GetDB().IsUserBanned(context.Background(), chatId) {
			ctx.Reply(u, ext.ReplyTextStyledTextArray([]styling.StyledTextOption{
				styling.Italic("Sᴏʀʀʏ Sɪʀ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ ᴛᴏ ᴜsᴇ ᴍᴇ.\n\n"),
				styling.TextURL("Cᴏɴᴛᴀᴄᴛ Dᴇᴠᴇʟᴏᴘᴇʀ", getUpdatesURL()),
				styling.Bold(" Tʜᴇʏ Wɪʟʟ Hᴇʟᴘ Yᴏᴜ"),
			}), nil)
			return dispatcher.EndGroups
		}
	}

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

	// Forward file to log channel
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

	// Hash & link generation
	fullHash := utils.PackFile(file.FileName, file.FileSize, file.MimeType, file.ID)
	hash := utils.GetShortHash(fullHash)
	streamLink := fmt.Sprintf("%s/stream/%d?hash=%s", config.ValueOf.Host, messageID, hash)
	downloadLink := streamLink + "&d=true"
	botUsername := ctx.Self.Username
	shareLink := fmt.Sprintf("https://t.me/%s?start=file_%d_%s", botUsername, messageID, hash)
	fileSize := humanize.IBytes(uint64(file.FileSize))

	// Post a caption on the forwarded log channel message with full info
	logCaption := fmt.Sprintf(
		"👤 User ID: %d\n📂 File: %s\n📦 Size: %s\n🎭 Mime: %s\n🔗 Stream: %s\n📥 Download: %s",
		chatId, file.FileName, fileSize, file.MimeType, streamLink, downloadLink,
	)
	logChannel, chanErr := utils.GetLogChannelPeer(context.Background(), ctx.Raw, ctx.PeerStorage)
	if chanErr == nil {
		ctx.Raw.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:      &tg.InputPeerChannel{ChannelID: logChannel.ChannelID, AccessHash: logChannel.AccessHash},
			Message:   logCaption,
			ReplyTo:   &tg.InputReplyToMessage{ReplyToMsgID: messageID},
			NoWebpage: true,
			RandomID:  rand.Int63(),
		})
	}

	// Store only user_id + message_id in DB
	if database.IsEnabled() {
		db := database.GetDB()
		_ = db.AddFileLink(context.Background(), chatId, messageID)
		db.IncrLinks(context.Background(), chatId)
	}

	isVideo := strings.Contains(file.MimeType, "video")
	isMedia := isVideo ||
		strings.Contains(file.MimeType, "audio") ||
		strings.Contains(file.MimeType, "pdf")

	var styledParts []styling.StyledTextOption
	styledParts = append(styledParts,
		styling.Bold("𝗬𝗼𝘂𝗿 𝗟𝗶𝗻𝗸 𝗚𝗲𝗻𝗲𝗿𝗮𝘁𝗲𝗱 !\n\n"),
		styling.Bold("📂 Fɪʟᴇ ɴᴀᴍᴇ : "), styling.Bold(file.FileName), styling.Plain("\n\n"),
		styling.Bold("📦 Fɪʟᴇ ꜱɪᴢᴇ : "), styling.Code(fileSize), styling.Plain("\n\n"),
		styling.Bold("📥 Dᴏᴡɴʟᴏᴀᴅ : "), styling.Code(downloadLink), styling.Plain("\n\n"),
	)
	if isMedia {
		styledParts = append(styledParts,
			styling.Bold("🖥 Wᴀᴛᴄʜ : "), styling.Code(streamLink), styling.Plain("\n\n"),
		)
	}
	styledParts = append(styledParts,
		styling.Bold("🔗 Sʜᴀʀᴇ : "), styling.Code(shareLink), styling.Plain("\n\n"),
		styling.Plain("Oᴘᴇɴ ᴛʜɪs ʟɪɴᴋ ᴏɴ Bʀᴏᴡsᴇʀ 🌐 ᴛᴏ ᴀᴠᴏɪᴅ ɪssᴜᴇs."),
	)

	var rows []tg.KeyboardButtonRow
	if isVideo {
		rows = append(rows, tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{Text: "sᴛʀᴇᴀᴍ", URL: streamLink},
			&tg.KeyboardButtonURL{Text: "ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
		}})
	} else {
		rows = append(rows, tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{Text: "ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
		}})
	}
	rows = append(rows, tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
		&tg.KeyboardButtonURL{Text: "ɢᴇᴛ ғɪʟᴇ", URL: shareLink},
		&tg.KeyboardButtonCallback{Text: "ʀᴇᴠᴏᴋᴇ ғɪʟᴇ", Data: []byte(fmt.Sprintf("revoke_%d", messageID))},
	}})
	rows = append(rows, tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
		&tg.KeyboardButtonCallback{Text: "ᴄʟᴏsᴇ", Data: []byte("close")},
	}})

	markup := &tg.ReplyInlineMarkup{Rows: rows}

	if strings.Contains(streamLink, "http://localhost") {
		_, err = ctx.Reply(u, ext.ReplyTextStyledTextArray(styledParts), &ext.ReplyOpts{
			ReplyToMessageId: u.EffectiveMessage.ID,
		})
	} else {
		_, err = ctx.Reply(u, ext.ReplyTextStyledTextArray(styledParts), &ext.ReplyOpts{
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

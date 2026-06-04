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
			devLink := updatesURL()
			ctx.Reply(u, ext.ReplyTextString(
				fmt.Sprintf("Sᴏʀʀʏ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ ᴛᴏ ᴜsᴇ ᴍᴇ.\n\nContact Developer: %s", devLink),
			), nil)
			return dispatcher.EndGroups
		}
	}

	// Allowed users check
	if len(config.ValueOf.AllowedUsers) != 0 && !utils.Contains(config.ValueOf.AllowedUsers, chatId) {
		ctx.Reply(u, ext.ReplyTextString("You are not allowed to use this bot."), nil)
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

	fullHash := utils.PackFile(file.FileName, file.FileSize, file.MimeType, file.ID)
	hash := utils.GetShortHash(fullHash)
	streamLink := fmt.Sprintf("%s/stream/%d?hash=%s", config.ValueOf.Host, messageID, hash)
	downloadLink := streamLink + "&d=true"

	// Track in database
	if database.IsEnabled() {
		db := database.GetDB()
		_ = db.AddLink(context.Background(), chatId, streamLink)
		db.IncrLinks(context.Background(), chatId)
	}

	// Human-readable file size
	fileSize := humanize.IBytes(uint64(file.FileSize))

	// FIX: Share link now contains both the message ID AND the short hash so that
	// only someone who already has the link can use it – random enumeration of
	// message IDs is no longer enough to access a file.
	botUsername := ctx.Self.Username
	shareLink := fmt.Sprintf("https://t.me/%s?start=file_%d_%s", botUsername, messageID, hash)

	// Build the reply text
	isMedia := strings.Contains(file.MimeType, "video") ||
		strings.Contains(file.MimeType, "audio") ||
		strings.Contains(file.MimeType, "pdf")

	var replyText string
	if isMedia {
		replyText = fmt.Sprintf(
			"𝗬𝗼𝘂𝗿 𝗟𝗶𝗻𝗸 𝗚𝗲𝗻𝗲𝗿𝗮𝘁𝗲𝗱 !\n\n"+
				"📂 Fɪʟᴇ ɴᴀᴍᴇ : %s\n\n"+
				"📦 Fɪʟᴇ ꜱɪᴢᴇ : %s\n\n"+
				"📥 Dᴏᴡɴʟᴏᴀᴅ : %s\n\n"+
				"🖥 Wᴀᴛᴄʜ : %s\n\n"+
				"🔗 Sʜᴀʀᴇ : %s\n\n"+
				"Oᴘᴇɴ ᴛʜɪs ʟɪɴᴋ ᴏɴ Bʀᴏᴡsᴇʀ 🌐 ᴛᴏ ᴀᴠᴏɪᴅ ɪssᴜᴇs.",
			file.FileName, fileSize, downloadLink, streamLink, shareLink,
		)
	} else {
		replyText = fmt.Sprintf(
			"𝗬𝗼𝘂𝗿 𝗟𝗶𝗻𝗸 𝗚𝗲𝗻𝗲𝗿𝗮𝘁𝗲𝗱 !\n\n"+
				"📂 Fɪʟᴇ ɴᴀᴍᴇ : %s\n\n"+
				"📦 Fɪʟᴇ ꜱɪᴢᴇ : %s\n\n"+
				"📥 Dᴏᴡɴʟᴏᴀᴅ : %s\n\n"+
				"🔗 Sʜᴀʀᴇ : %s\n\n"+
				"Oᴘᴇɴ ᴛʜɪs ʟɪɴᴋ ᴏɴ Bʀᴏᴡsᴇʀ 🌐 ᴛᴏ ᴀᴠᴏɪᴅ ɪssᴜᴇs.",
			file.FileName, fileSize, downloadLink, shareLink,
		)
	}

	// Inline buttons
	row := tg.KeyboardButtonRow{
		Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{Text: "📥 Dᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
		},
	}
	if isMedia {
		row.Buttons = append(row.Buttons, &tg.KeyboardButtonURL{
			Text: "🖥 Wᴀᴛᴄʜ",
			URL:  streamLink,
		})
	}
	markup := &tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{row}}

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

package commands

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/database"
	"EverythingSuckz/fsb/internal/utils"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	"github.com/gotd/td/tg"
)

func (m *command) LoadStart(d dispatcher.Dispatcher) {
	log := m.log.Named("start")
	defer log.Sugar().Info("Loaded")
	d.AddHandler(handlers.NewCommand("start", m.start))
}

// ── Texts ──────────────────────────────────────────────────────────────────

const aboutText = `⚜ ᴍʏ ɴᴀᴍᴇ : File to link

✦ ᴠᴇʀsɪᴏɴ : 1.1.0
✦ ᴜᴘᴅᴀᴛᴇᴅ ᴏɴ : 06-January-2026
✦ ᴅᴇᴠᴇʟᴏᴘᴇʀ : Royality Bots`

// ── Keyboard helpers ───────────────────────────────────────────────────────

func getUpdatesURL() string {
	if config.ValueOf.UpdatesChannel != "" {
		return "https://t.me/" + config.ValueOf.UpdatesChannel
	}
	return "https://t.me/"
}

func startMarkup() *tg.ReplyInlineMarkup {
	rows := []tg.KeyboardButtonRow{
		{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonCallback{Text: "ʜᴇʟᴘ", Data: []byte("help")},
			&tg.KeyboardButtonCallback{Text: "ᴀʙᴏᴜᴛ", Data: []byte("about")},
			&tg.KeyboardButtonCallback{Text: "ᴄʟᴏsᴇ", Data: []byte("close")},
		}},
		{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonCallback{Text: "ᴅᴏɴᴀᴛᴇ ⭐", Data: []byte("donate")},
		}},
	}
	if config.ValueOf.UpdatesChannel != "" {
		rows = append(rows, tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{Text: "📢 ᴜᴘᴅᴀᴛᴇs ᴄʜᴀɴɴᴇʟ", URL: getUpdatesURL()},
		}})
	}
	return &tg.ReplyInlineMarkup{Rows: rows}
}

func welcomeMessage(firstName string) string {
	return fmt.Sprintf(
		"👋 Hᴇʏ, %s\n\n"+
			"I'ᴍ ᴛᴇʟᴇɢʀᴀᴍ ғɪʟᴇs sᴛʀᴇᴀᴍɪɴɢ ʙᴏᴛ ᴀs ᴡᴇʟʟ ᴅɪʀᴇᴄᴛ ʟɪɴᴋs ɢᴇɴᴇʀᴀᴛᴏʀ\n\n"+
			"ᴡᴏʀᴋɪɴɢ ᴏɴ ᴄʜᴀɴɴᴇʟs ᴀɴᴅ ᴘʀɪᴠᴀᴛᴇ ᴄʜᴀᴛ\n"+
			"‣ 💥Fᴀsᴛ ᴀs ᴀ ʀᴏᴄᴋᴇᴛ🚀 ᴀɴᴅ ғᴇᴇʟɪɴɢ ᴀs ᴀ ᴋɪɴɢ👑",
		firstName,
	)
}

// ── /start ─────────────────────────────────────────────────────────────────

func (m *command) start(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()

	if ctx.PeerStorage.GetPeerById(chatId).Type != int(storage.TypeUser) {
		return dispatcher.EndGroups
	}

	bgCtx := context.Background()

	if database.IsEnabled() {
		if database.GetDB().IsUserBanned(bgCtx, chatId) {
			ctx.Reply(u, ext.ReplyTextString(
				fmt.Sprintf("Sᴏʀʀʏ Sɪʀ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ ᴛᴏ ᴜsᴇ ᴍᴇ.\n\nContact Developer: %s", getUpdatesURL()),
			), nil)
			return dispatcher.EndGroups
		}
	}

	if len(config.ValueOf.AllowedUsers) != 0 && !utils.Contains(config.ValueOf.AllowedUsers, chatId) {
		ctx.Reply(u, ext.ReplyTextString("Yᴏᴜ ᴀʀᴇ ɴᴏᴛ ᴀᴜᴛʜᴏʀɪᴢᴇᴅ ᴛᴏ ᴜsᴇ ᴛʜɪs ʙᴏᴛ."), nil)
		return dispatcher.EndGroups
	}

	if database.IsEnabled() {
		db := database.GetDB()
		if !db.UserExists(bgCtx, chatId) {
			_ = db.AddUser(bgCtx, chatId)
			m.logNewUser(ctx, u, chatId)
		}
	}

	text := u.EffectiveMessage.Text
	if idx := strings.Index(text, " "); idx != -1 {
		payload := text[idx+1:]
		if strings.HasPrefix(payload, "file_") {
			return m.handleFileDeepLink(ctx, u, strings.TrimPrefix(payload, "file_"), chatId)
		}
	}

	firstName := "there"
	if user := u.EffectiveUser(); user != nil && user.FirstName != "" {
		firstName = user.FirstName
	}

	ctx.Reply(u, ext.ReplyTextString(welcomeMessage(firstName)), &ext.ReplyOpts{Markup: startMarkup()})
	return dispatcher.EndGroups
}

// ── Deep-link file handler ─────────────────────────────────────────────────

func (m *command) handleFileDeepLink(ctx *ext.Context, u *ext.Update, payload string, chatId int64) error {
	parts := strings.SplitN(payload, "_", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		ctx.Reply(u, ext.ReplyTextString("Invalid Command"), nil)
		return dispatcher.EndGroups
	}
	messageID, err := strconv.Atoi(parts[0])
	if err != nil || messageID == 0 {
		ctx.Reply(u, ext.ReplyTextString("Invalid Command"), nil)
		return dispatcher.EndGroups
	}
	shortHash := parts[1]

	file, err := utils.FileFromMessage(context.Background(), m.client, messageID)
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString("File Not Found"), nil)
		return dispatcher.EndGroups
	}

	expectedHash := utils.PackFile(file.FileName, file.FileSize, file.MimeType, file.ID)
	if !utils.CheckHash(shortHash, expectedHash) {
		ctx.Reply(u, ext.ReplyTextString("File Not Found"), nil)
		return dispatcher.EndGroups
	}

	tgMsg, err := utils.GetTGMessage(context.Background(), m.client, messageID)
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString("File Not Found"), nil)
		return dispatcher.EndGroups
	}

	var inputMedia tg.InputMediaClass
	switch media := tgMsg.Media.(type) {
	case *tg.MessageMediaDocument:
		doc, ok := media.Document.AsNotEmpty()
		if !ok {
			ctx.Reply(u, ext.ReplyTextString("File Not Found"), nil)
			return dispatcher.EndGroups
		}
		inputMedia = &tg.InputMediaDocument{
			ID: &tg.InputDocument{
				ID:            doc.ID,
				AccessHash:    doc.AccessHash,
				FileReference: doc.FileReference,
			},
		}
	case *tg.MessageMediaPhoto:
		photo, ok := media.Photo.AsNotEmpty()
		if !ok {
			ctx.Reply(u, ext.ReplyTextString("File Not Found"), nil)
			return dispatcher.EndGroups
		}
		inputMedia = &tg.InputMediaPhoto{
			ID: &tg.InputPhoto{
				ID:            photo.ID,
				AccessHash:    photo.AccessHash,
				FileReference: photo.FileReference,
			},
		}
	default:
		logChannel, fwdErr := utils.GetLogChannelPeer(context.Background(), ctx.Raw, ctx.PeerStorage)
		if fwdErr != nil {
			ctx.Reply(u, ext.ReplyTextString("Something Went Wrong"), nil)
			return dispatcher.EndGroups
		}
		ctx.Raw.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
			RandomID: []int64{rand.Int63()},
			FromPeer: &tg.InputPeerChannel{ChannelID: logChannel.ChannelID, AccessHash: logChannel.AccessHash},
			ID:       []int{messageID},
			ToPeer:   &tg.InputPeerUser{UserID: chatId},
		})
		return dispatcher.EndGroups
	}

	sentUpdates, sendErr := ctx.Raw.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer:     &tg.InputPeerUser{UserID: chatId},
		Media:    inputMedia,
		Message:  file.FileName,
		RandomID: rand.Int63(),
	})
	if sendErr != nil {
		ctx.Reply(u, ext.ReplyTextString("Something Went Wrong"), nil)
		return dispatcher.EndGroups
	}

	var sentMsgID int
	if upd, ok := sentUpdates.(*tg.Updates); ok {
		for _, update := range upd.Updates {
			if msgIDUpd, ok := update.(*tg.UpdateMessageID); ok {
				sentMsgID = msgIDUpd.ID
				break
			}
		}
	}
	origMsgID := u.EffectiveMessage.ID
	capturedSentID := sentMsgID
	go func() {
		time.Sleep(1 * time.Hour)
		freshCtx := m.client.CreateContext()
		if capturedSentID != 0 {
			freshCtx.Raw.MessagesDeleteMessages(freshCtx, &tg.MessagesDeleteMessagesRequest{
				Revoke: true, ID: []int{capturedSentID},
			})
		}
		freshCtx.Raw.MessagesDeleteMessages(freshCtx, &tg.MessagesDeleteMessagesRequest{
			Revoke: true, ID: []int{origMsgID},
		})
	}()

	return dispatcher.EndGroups
}

// ── Callback handler ───────────────────────────────────────────────────────

func (m *command) LoadCallbacks(d dispatcher.Dispatcher) {
	log := m.log.Named("callbacks")
	defer log.Sugar().Info("Loaded")
	d.AddHandler(handlers.NewCallbackQuery(nil, m.handleCallback))
}

func (m *command) handleCallback(ctx *ext.Context, u *ext.Update) error {
	query := u.CallbackQuery
	if query == nil {
		return dispatcher.EndGroups
	}

	data := string(query.Data)

	if strings.HasPrefix(data, "mf_page_") {
		return m.myFilesPageCallback(ctx, u, strings.TrimPrefix(data, "mf_page_"))
	}

	if strings.HasPrefix(data, "cf_") {
		return m.clearFilesCallback(ctx, u, strings.TrimPrefix(data, "cf_"))
	}

	if data == "goto_myfiles" {
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{QueryID: query.QueryID})
		return m.myFiles(ctx, u)
	}

	if strings.HasPrefix(data, "revoke_") {
		msgIDStr := strings.TrimPrefix(data, "revoke_")
		msgID, err := strconv.Atoi(msgIDStr)
		if err != nil {
			ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
				QueryID: query.QueryID, Message: "Invalid message ID", Alert: true,
			})
			return dispatcher.EndGroups
		}

		logChannel, chanErr := utils.GetLogChannelPeer(context.Background(), ctx.Raw, ctx.PeerStorage)
		if chanErr == nil {
			ctx.Raw.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
				Channel: logChannel,
				ID:      []int{msgID},
			})
		}

		if u.CallbackQuery != nil {
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:        &tg.InputPeerUser{UserID: u.EffectiveChat().GetID()},
				ID:          u.CallbackQuery.MsgID,
				Message:     "Link Revoked ✅",
				ReplyMarkup: &tg.ReplyInlineMarkup{},
			})
		}
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: query.QueryID, Message: "✅ File link revoked successfully.", Alert: true,
		})
		return dispatcher.EndGroups
	}

	firstName := "there"
	if user := u.EffectiveUser(); user != nil && user.FirstName != "" {
		firstName = user.FirstName
	}

	switch data {
	case "home":
		if u.CallbackQuery != nil {
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:        &tg.InputPeerUser{UserID: u.EffectiveChat().GetID()},
				ID:          u.CallbackQuery.MsgID,
				Message:     welcomeMessage(firstName),
				ReplyMarkup: startMarkup(),
			})
		}
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{QueryID: query.QueryID})

	case "help":
		helpText := fmt.Sprintf(
			"How To Use\n\n"+
				"• Add me as an admin on the channel\n"+
				"• Send me any document or media\n"+
				"• I'll provide a streamable link\n\n"+
				"🔞 Adult content strictly prohibited.\n\n"+
				"Report bugs to Developer: %s",
			getUpdatesURL(),
		)
		helpRows := []tg.KeyboardButtonRow{
			{Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{Text: "ʜᴏᴍᴇ", Data: []byte("home")},
				&tg.KeyboardButtonCallback{Text: "ᴀʙᴏᴜᴛ", Data: []byte("about")},
				&tg.KeyboardButtonCallback{Text: "ᴄʟᴏsᴇ", Data: []byte("close")},
			}},
		}
		if config.ValueOf.UpdatesChannel != "" {
			helpRows = append(helpRows, tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonURL{Text: "📢 ᴜᴘᴅᴀᴛᴇs ᴄʜᴀɴɴᴇʟ", URL: getUpdatesURL()},
			}})
		}
		helpMarkup := &tg.ReplyInlineMarkup{Rows: helpRows}
		if u.CallbackQuery != nil {
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:        &tg.InputPeerUser{UserID: u.EffectiveChat().GetID()},
				ID:          u.CallbackQuery.MsgID,
				Message:     helpText,
				ReplyMarkup: helpMarkup,
			})
		}
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{QueryID: query.QueryID})

	case "about":
		aboutRows := []tg.KeyboardButtonRow{
			{Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{Text: "ʜᴏᴍᴇ", Data: []byte("home")},
				&tg.KeyboardButtonCallback{Text: "ʜᴇʟᴘ", Data: []byte("help")},
				&tg.KeyboardButtonCallback{Text: "ᴄʟᴏsᴇ", Data: []byte("close")},
			}},
		}
		if config.ValueOf.UpdatesChannel != "" {
			aboutRows = append(aboutRows, tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonURL{Text: "📢 ᴜᴘᴅᴀᴛᴇs ᴄʜᴀɴɴᴇʟ", URL: getUpdatesURL()},
			}})
		}
		aboutMarkup := &tg.ReplyInlineMarkup{Rows: aboutRows}
		if u.CallbackQuery != nil {
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:        &tg.InputPeerUser{UserID: u.EffectiveChat().GetID()},
				ID:          u.CallbackQuery.MsgID,
				Message:     aboutText,
				ReplyMarkup: aboutMarkup,
			})
		}
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{QueryID: query.QueryID})

	case "donate":
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: query.QueryID,
			Message: "⭐ Support This Project\n\n" +
				"Help me to motivate and buy me a glass of tea\n\n" +
				"• Helps me to design a new Advanced bot\n" +
				"• Helps me to be motivated\n" +
				"• Helps me to maintain server\n\n" +
				"Your small amount can motivate me a lot.\n\nThanks",
			CacheTime: 30,
			Alert:     true,
		})

	case "close":
		if u.CallbackQuery != nil {
			ctx.Raw.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
				Revoke: true, ID: []int{u.CallbackQuery.MsgID},
			})
		}
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{QueryID: query.QueryID})
	}

	return dispatcher.EndGroups
}

// ── New-user logger ────────────────────────────────────────────────────────

func (m *command) logNewUser(ctx *ext.Context, u *ext.Update, chatId int64) {
	if config.ValueOf.ULogChannelID == 0 {
		return
	}
	firstName := "Unknown"
	if user := u.EffectiveUser(); user != nil && user.FirstName != "" {
		firstName = user.FirstName
	}
	logMsg := fmt.Sprintf(
		"#NewUser\nUser Name : %s\nUser ID : %d",
		firstName, chatId,
	)
	ulogChannelID := config.ValueOf.ULogChannelID
	cachedPeer := ctx.PeerStorage.GetInputPeerById(ulogChannelID)
	var toPeer tg.InputPeerClass
	switch peer := cachedPeer.(type) {
	case *tg.InputPeerChannel:
		toPeer = peer
	default:
		toPeer = &tg.InputPeerChannel{ChannelID: ulogChannelID}
	}
	ctx.Raw.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     toPeer,
		Message:  logMsg,
		RandomID: rand.Int63(),
	})
}

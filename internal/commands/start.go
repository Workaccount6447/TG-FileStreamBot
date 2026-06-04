package commands

import (
	"context"
	"fmt"
	"math/rand"
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

func (m *command) LoadStart(dispatcher dispatcher.Dispatcher) {
	log := m.log.Named("start")
	defer log.Sugar().Info("Loaded")
	dispatcher.AddHandler(handlers.NewCommand("start", start))
}

// ─── Message texts ────────────────────────────────────────────────────────────

const helpText = `• ᴀᴅᴅ ᴍᴇ ᴀs ᴀɴ ᴀᴅᴍɪɴ ᴏɴ ᴛʜᴇ ᴄʜᴀɴɴᴇʟ
• sᴇɴᴅ ᴍᴇ ᴀɴʏ ᴅᴏᴄᴜᴍᴇɴᴛ ᴏʀ ᴍᴇᴅɪᴀ
• ɪ'ʟʟ ᴘʀᴏᴠɪᴅᴇ sᴛʀᴇᴀᴍᴀʙʟᴇ ʟɪɴᴋ

🔞 ᴀᴅᴜʟᴛ ᴄᴏɴᴛᴇɴᴛ sᴛʀɪᴄᴛʟʏ ᴘʀᴏʜɪʙɪᴛᴇᴅ.

ʀᴇᴘᴏʀᴛ ʙᴜɢs ᴛᴏ ᴅᴇᴠᴇʟᴏᴘᴇʀ`

const aboutText = `⚜ ᴍʏ ɴᴀᴍᴇ : File to link

✦ ᴠᴇʀsɪᴏɴ : 1.1.0
✦ ᴜᴘᴅᴀᴛᴇᴅ ᴏɴ : 06-January-2026
✦ ᴅᴇᴠᴇʟᴏᴘᴇʀ : [Royality Bots](https://t.me/RoyalityBots)`

// buildStartMarkup returns the 2-row inline keyboard for the start message.
//
//	Row 1:  [ʜᴇʟᴘ]  [ᴀʙᴏᴜᴛ]
//	Row 2:  [📢 ᴜᴘᴅᴀᴛᴇs ᴄʜᴀɴɴᴇʟ]
func buildStartMarkup(updatesURL string) *tg.ReplyInlineMarkup {
	return &tg.ReplyInlineMarkup{
		Rows: []tg.KeyboardButtonRow{
			{
				Buttons: []tg.KeyboardButtonClass{
					&tg.KeyboardButtonCallback{Text: "ʜᴇʟᴘ", Data: []byte("help")},
					&tg.KeyboardButtonCallback{Text: "ᴀʙᴏᴜᴛ", Data: []byte("about")},
				},
			},
			{
				Buttons: []tg.KeyboardButtonClass{
					&tg.KeyboardButtonURL{Text: "📢 ᴜᴘᴅᴀᴛᴇs ᴄʜᴀɴɴᴇʟ", URL: updatesURL},
				},
			},
		},
	}
}

func updatesURL() string {
	if config.ValueOf.UpdatesChannel != "" {
		return "https://t.me/" + config.ValueOf.UpdatesChannel
	}
	return "https://t.me/"
}

// ─── /start handler ──────────────────────────────────────────────────────────

func start(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()

	// Only respond in private chats
	peerChatId := ctx.PeerStorage.GetPeerById(chatId)
	if peerChatId.Type != int(storage.TypeUser) {
		return dispatcher.EndGroups
	}

	bgCtx := context.Background()

	// 1. Ban check
	if database.IsEnabled() {
		db := database.GetDB()
		if db.IsUserBanned(bgCtx, chatId) {
			devLink := updatesURL()
			ctx.Reply(u, ext.ReplyTextString(
				fmt.Sprintf("Sᴏʀʀʏ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ ᴛᴏ ᴜsᴇ ᴍᴇ.\n\nContact Developer: %s", devLink),
			), nil)
			return dispatcher.EndGroups
		}
	}

	// 2. Allowed-users check
	if len(config.ValueOf.AllowedUsers) != 0 && !utils.Contains(config.ValueOf.AllowedUsers, chatId) {
		ctx.Reply(u, ext.ReplyTextString("You are not allowed to use this bot."), nil)
		return dispatcher.EndGroups
	}

	// 3. Register new user → log to ULOG channel
	if database.IsEnabled() {
		db := database.GetDB()
		if !db.UserExists(bgCtx, chatId) {
			_ = db.AddUser(bgCtx, chatId)
			logNewUser(ctx, u, chatId)
		}
	}

	// 4. Check for deep-link payload: ?start=file_<msgID>_<hash>
	//    FIX: previously the payload was ignored and /start always showed the
	//    welcome message.  Now we detect "file_<id>_<hash>" and serve the file.
	msg := u.EffectiveMessage
	if msg != nil && len(msg.Message) > 7 && msg.Message[:6] == "/start" {
		payload := ""
		if len(msg.Message) > 7 {
			payload = msg.Message[7:] // everything after "/start "
		}
		if len(payload) > 5 && payload[:5] == "file_" {
			return handleFileDeepLink(ctx, u, payload[5:], chatId)
		}
	}

	// 5. Send 👋, wait 1s, delete it
	waveMsg, waveErr := ctx.Reply(u, ext.ReplyTextString("👋"), nil)
	if waveErr == nil && waveMsg != nil {
		time.Sleep(1 * time.Second)
		ctx.Raw.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			Revoke: true,
			ID:     []int{waveMsg.ID},
		})
	}

	// 6. Build personalised start text
	firstName := "there"
	if user := u.EffectiveUser(); user != nil && user.FirstName != "" {
		firstName = user.FirstName
	}
	startText := fmt.Sprintf(
		"**👋 Hᴇʏ, %s**\n\n"+
			"I'ᴍ ᴛᴇʟᴇɢʀᴀᴍ ғɪʟᴇs sᴛʀᴇᴀᴍɪɴɢ ʙᴏᴛ ᴀs ᴡᴇʟʟ ᴅɪʀᴇᴄᴛ ʟɪɴᴋs ɢᴇɴᴇʀᴀᴛᴏʀ\n\n"+
			"ᴡᴏʀᴋɪɴɢ ᴏɴ ᴄʜᴀɴɴᴇʟs ᴀɴᴅ ᴘʀɪᴠᴀᴛᴇ ᴄʜᴀᴛ\n\n"+
			"> **‣ 💥Fᴀsᴛ ᴀs ᴀ ʀᴏᴄᴋᴇᴛ🚀 ᴀɴᴅ ғᴇᴇʟɪɴɢ ᴀs ᴀ ᴋɪɴɢ👑 sᴜᴄʜ ᴛʜᴀᴛ ᴍᴀᴅᴇ ʙʏ\n> ʀᴏʏᴀʟɪᴛʏ ʙᴏᴛꜱ👑**",
		firstName,
	)

	ctx.Reply(u, ext.ReplyTextString(startText), &ext.ReplyOpts{
		Markup: buildStartMarkup(updatesURL()),
	})
	return dispatcher.EndGroups
}

// handleFileDeepLink resolves a "file_<msgID>_<hash>" deep-link payload.
// FIX: share links now include a hash so random users cannot enumerate files.
func handleFileDeepLink(ctx *ext.Context, u *ext.Update, payload string, chatId int64) error {
	// payload format: "<messageID>_<shortHash>"
	var messageID int
	var shortHash string
	if _, err := fmt.Sscanf(payload, "%d_%s", &messageID, &shortHash); err != nil || messageID == 0 || shortHash == "" {
		ctx.Reply(u, ext.ReplyTextString("❌ Invalid share link."), nil)
		return dispatcher.EndGroups
	}

	file, err := utils.FileFromMessage(ctx, nil, messageID)
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("❌ File not found: %s", err.Error())), nil)
		return dispatcher.EndGroups
	}

	expectedHash := utils.PackFile(file.FileName, file.FileSize, file.MimeType, file.ID)
	if !utils.CheckHash(shortHash, expectedHash) {
		ctx.Reply(u, ext.ReplyTextString("❌ Invalid or expired share link."), nil)
		return dispatcher.EndGroups
	}

	streamLink := fmt.Sprintf("%s/stream/%d?hash=%s", config.ValueOf.Host, messageID, utils.GetShortHash(expectedHash))
	downloadLink := streamLink + "&d=true"

	replyText := fmt.Sprintf(
		"**📂 %s**\n\n📥 Download: %s\n🖥 Watch: %s",
		file.FileName, downloadLink, streamLink,
	)

	row := tg.KeyboardButtonRow{
		Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{Text: "📥 Dᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
			&tg.KeyboardButtonURL{Text: "🖥 Wᴀᴛᴄʜ", URL: streamLink},
		},
	}
	markup := &tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{row}}

	ctx.Reply(u, ext.ReplyTextString(replyText), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

// ─── Callback query handler for Help / About buttons ─────────────────────────

func (m *command) LoadCallbacks(d dispatcher.Dispatcher) {
	log := m.log.Named("callbacks")
	defer log.Sugar().Info("Loaded")
	d.AddHandler(handlers.NewCallbackQuery(nil, handleCallback))
}

func handleCallback(ctx *ext.Context, u *ext.Update) error {
	query := u.CallbackQuery
	if query == nil {
		return dispatcher.EndGroups
	}
	data := string(query.Data)

	switch data {
	case "help":
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
			QueryID:   query.QueryID,
			Alert:     false,
			Message:   helpText,
			CacheTime: 30,
		})
	case "about":
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
			QueryID:   query.QueryID,
			Alert:     false,
			Message:   aboutText,
			CacheTime: 30,
		})
	}
	return dispatcher.EndGroups
}

// ─── New-user logger ──────────────────────────────────────────────────────────

func logNewUser(ctx *ext.Context, u *ext.Update, chatId int64) {
	if config.ValueOf.ULogChannelID == 0 {
		return
	}
	firstName := "Unknown"
	user := u.EffectiveUser()
	if user != nil && user.FirstName != "" {
		firstName = user.FirstName
	}
	logMsg := fmt.Sprintf(
		"#NᴇᴡUsᴇʀ\n⬩ ᴜsᴇʀ ɴᴀᴍᴇ : %s\n⬩ ᴜsᴇʀ ɪᴅ : %d",
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

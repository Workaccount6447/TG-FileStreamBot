package commands

import (
	"context"
	"fmt"
	"math/rand"
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

const helpText = `• ᴀᴅᴅ ᴍᴇ ᴀs ᴀɴ ᴀᴅᴍɪɴ ᴏɴ ᴛʜᴇ ᴄʜᴀɴɴᴇʟ
• sᴇɴᴅ ᴍᴇ ᴀɴʏ ᴅᴏᴄᴜᴍᴇɴᴛ ᴏʀ ᴍᴇᴅɪᴀ
• ɪ'ʟʟ ᴘʀᴏᴠɪᴅᴇ sᴛʀᴇᴀᴍᴀʙʟᴇ ʟɪɴᴋ

🔞 ᴀᴅᴜʟᴛ ᴄᴏɴᴛᴇɴᴛ sᴛʀɪᴄᴛʟʏ ᴘʀᴏʜɪʙɪᴛᴇᴅ.

ʀᴇᴘᴏʀᴛ ʙᴜɢs ᴛᴏ ᴅᴇᴠᴇʟᴏᴘᴇʀ`

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
					&tg.KeyboardButtonURL{Text: "📢 ᴜᴘᴅᴀᴛᴇs ᴄʜᴀɴɴᴇʟ", URL: getUpdatesURL()},
				},
			},
		},
	}
}

// ── /start ─────────────────────────────────────────────────────────────────

func (m *command) start(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()

	// Private chats only
	if ctx.PeerStorage.GetPeerById(chatId).Type != int(storage.TypeUser) {
		return dispatcher.EndGroups
	}

	bgCtx := context.Background()

	// Ban check
	if database.IsEnabled() {
		if database.GetDB().IsUserBanned(bgCtx, chatId) {
			ctx.Reply(u, ext.ReplyTextString(
				fmt.Sprintf("Sᴏʀʀʏ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ.\n\nContact Developer: %s", getUpdatesURL()),
			), nil)
			return dispatcher.EndGroups
		}
	}

	// Allowed-users check
	if len(config.ValueOf.AllowedUsers) != 0 && !utils.Contains(config.ValueOf.AllowedUsers, chatId) {
		ctx.Reply(u, ext.ReplyTextString("You are not allowed to use this bot."), nil)
		return dispatcher.EndGroups
	}

	// Register new user
	if database.IsEnabled() {
		db := database.GetDB()
		if !db.UserExists(bgCtx, chatId) {
			_ = db.AddUser(bgCtx, chatId)
			m.logNewUser(ctx, u, chatId)
		}
	}

	// ── FIX 1: Deep-link handling ──────────────────────────────────────────
	// When a user clicks a share link the message text is "/start file_<id>_<hash>".
	// Previously this was never checked so it always showed the welcome screen.
	// Now we parse the payload and serve the file directly.
	text := u.EffectiveMessage.Text
	if idx := strings.Index(text, " "); idx != -1 {
		payload := text[idx+1:] // everything after "/start "
		if strings.HasPrefix(payload, "file_") {
			return m.handleFileDeepLink(ctx, u, strings.TrimPrefix(payload, "file_"), chatId)
		}
	}

	// ── Welcome message ────────────────────────────────────────────────────
	// Wave → short delay → delete wave → send main message
	waveMsg, waveErr := ctx.Reply(u, ext.ReplyTextString("👋"), nil)
	if waveErr == nil && waveMsg != nil {
		time.Sleep(1 * time.Second)
		ctx.Raw.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			Revoke: true,
			ID:     []int{waveMsg.ID},
		})
	}

	firstName := "there"
	if user := u.EffectiveUser(); user != nil && user.FirstName != "" {
		firstName = user.FirstName
	}

	welcomeText := fmt.Sprintf(
		"**👋 Hᴇʏ, %s**\n\n"+
			"I'ᴍ ᴛᴇʟᴇɢʀᴀᴍ ғɪʟᴇs sᴛʀᴇᴀᴍɪɴɢ ʙᴏᴛ ᴀs ᴡᴇʟʟ ᴅɪʀᴇᴄᴛ ʟɪɴᴋs ɢᴇɴᴇʀᴀᴛᴏʀ\n\n"+
			"ᴡᴏʀᴋɪɴɢ ᴏɴ ᴄʜᴀɴɴᴇʟs ᴀɴᴅ ᴘʀɪᴠᴀᴛᴇ ᴄʜᴀᴛ\n\n"+
			"> **‣ 💥Fᴀsᴛ ᴀs ᴀ ʀᴏᴄᴋᴇᴛ🚀 ᴀɴᴅ ғᴇᴇʟɪɴɢ ᴀs ᴀ ᴋɪɴɢ👑 sᴜᴄʜ ᴛʜᴀᴛ ᴍᴀᴅᴇ ʙʏ\n> ʀᴏʏᴀʟɪᴛʏ ʙᴏᴛꜱ👑**",
		firstName,
	)

	ctx.Reply(u, ext.ReplyTextString(welcomeText), &ext.ReplyOpts{Markup: startMarkup()})
	return dispatcher.EndGroups
}

// ── Deep-link file handler ─────────────────────────────────────────────────

// handleFileDeepLink resolves "file_<msgID>_<hash>" share links.
// FIX 2: The hash is verified so random users cannot enumerate message IDs.
func (m *command) handleFileDeepLink(ctx *ext.Context, u *ext.Update, payload string, chatId int64) error {
	var messageID int
	var shortHash string
	if _, err := fmt.Sscanf(payload, "%d_%s", &messageID, &shortHash); err != nil || messageID == 0 || shortHash == "" {
		ctx.Reply(u, ext.ReplyTextString("❌ Invalid share link."), nil)
		return dispatcher.EndGroups
	}

	// Use global bot client (set in bot.StartClient) to fetch the message
	file, err := utils.FileFromMessage(context.Background(), m.client, messageID)
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("❌ File not found: %s", err.Error())), nil)
		return dispatcher.EndGroups
	}

	// Verify hash — prevents enumeration attacks
	expectedHash := utils.PackFile(file.FileName, file.FileSize, file.MimeType, file.ID)
	if !utils.CheckHash(shortHash, expectedHash) {
		ctx.Reply(u, ext.ReplyTextString("❌ Invalid or expired share link."), nil)
		return dispatcher.EndGroups
	}

	hash := utils.GetShortHash(expectedHash)
	streamLink := fmt.Sprintf("%s/stream/%d?hash=%s", config.ValueOf.Host, messageID, hash)
	downloadLink := streamLink + "&d=true"

	replyText := fmt.Sprintf("**📂 %s**\n\n📥 Download: %s\n🖥 Watch: %s",
		file.FileName, downloadLink, streamLink)

	markup := &tg.ReplyInlineMarkup{
		Rows: []tg.KeyboardButtonRow{{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonURL{Text: "📥 Dᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
				&tg.KeyboardButtonURL{Text: "🖥 Wᴀᴛᴄʜ", URL: streamLink},
			},
		}},
	}

	ctx.Reply(u, ext.ReplyTextString(replyText), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

// ── Callback query handler (Help / About buttons) ──────────────────────────

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
	switch string(query.Data) {
	case "help":
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: query.QueryID, Message: helpText, CacheTime: 30,
		})
	case "about":
		ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: query.QueryID, Message: aboutText, CacheTime: 30,
		})
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
	logMsg := fmt.Sprintf("#NᴇᴡUsᴇʀ\n⬩ ᴜsᴇʀ ɴᴀᴍᴇ : %s\n⬩ ᴜsᴇʀ ɪᴅ : %d", firstName, chatId)

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

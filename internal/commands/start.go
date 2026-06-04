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

const startText = `ʜᴇʀᴇ's ʜᴏᴡ ɪ ᴡᴏʀᴋ:

• ᴀᴅᴅ ᴍᴇ ᴀs ᴀɴ ᴀᴅᴍɪɴ ᴏɴ ᴛʜᴇ ᴄʜᴀɴɴᴇʟ
• sᴇɴᴅ ᴍᴇ ᴀɴʏ ᴅᴏᴄᴜᴍᴇɴᴛ ᴏʀ ᴍᴇᴅɪᴀ
• ɪ'ʟʟ ᴘʀᴏᴠɪᴅᴇ sᴛʀᴇᴀᴍᴀʙʟᴇ ʟɪɴᴋ

🔞 ᴀᴅᴜʟᴛ ᴄᴏɴᴛᴇɴᴛ sᴛʀɪᴄᴛʟʏ ᴘʀᴏʜɪʙɪᴛᴇᴅ.

ʀᴇᴘᴏʀᴛ ʙᴜɢs ᴛᴏ Developer`

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
			devLink := "https://t.me/"
			if config.ValueOf.UpdatesChannel != "" {
				devLink = "https://t.me/" + config.ValueOf.UpdatesChannel
			}
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

	// 4. Send 👋, wait 1s, delete it
	waveUpd, waveErr := ctx.Reply(u, ext.ReplyTextString("👋"), nil)
	if waveErr == nil && waveUpd != nil {
		time.Sleep(1 * time.Second)
		if upds, ok := waveUpd.(*tg.Updates); ok {
			for _, upd := range upds.Updates {
				if msgIDUpd, ok := upd.(*tg.UpdateMessageID); ok {
					ctx.Raw.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
						Revoke: true,
						ID:     []int{msgIDUpd.ID},
					})
					break
				}
			}
		}
	}

	// 5. Build inline buttons
	updatesURL := "https://t.me/"
	if config.ValueOf.UpdatesChannel != "" {
		updatesURL = "https://t.me/" + config.ValueOf.UpdatesChannel
	}
	markup := &tg.ReplyInlineMarkup{
		Rows: []tg.KeyboardButtonRow{
			{
				Buttons: []tg.KeyboardButtonClass{
					&tg.KeyboardButtonURL{Text: "ʜᴇʟᴘ & ɪɴꜰᴏ", URL: updatesURL},
					&tg.KeyboardButtonURL{Text: "📢 ᴜᴘᴅᴀᴛᴇs", URL: updatesURL},
				},
			},
		},
	}

	ctx.Reply(u, ext.ReplyTextString(startText), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

// logNewUser posts a #NᴇᴡUsᴇʀ entry to ULOG_CHANNEL
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

	// Resolve ULOG channel peer from peer storage (same pattern as GetLogChannelPeer)
	ulogChannelID := config.ValueOf.ULogChannelID
	cachedPeer := ctx.PeerStorage.GetInputPeerById(ulogChannelID)
	var toPeer tg.InputPeerClass

	switch peer := cachedPeer.(type) {
	case *tg.InputPeerChannel:
		toPeer = peer
	default:
		// Fallback: bare channel ID (works if bot is member)
		toPeer = &tg.InputPeerChannel{ChannelID: ulogChannelID}
	}

	ctx.Raw.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     toPeer,
		Message:  logMsg,
		RandomID: rand.Int63(),
	})
}

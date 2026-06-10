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
	"github.com/gotd/td/telegram/message/styling"
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
			ctx.Reply(u, ext.ReplyTextStyledTextArray([]styling.StyledTextOption{
				styling.Italic(fmt.Sprintf("Sᴏʀʀʏ Sɪʀ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ ᴛᴏ ᴜsᴇ ᴍᴇ.\n\n")),
				styling.TextURL("Cᴏɴᴛᴀᴄᴛ Dᴇᴠᴇʟᴏᴘᴇʀ", getUpdatesURL()),
				styling.Bold(" Tʜᴇʏ Wɪʟʟ Hᴇʟᴘ Yᴏᴜ"),
			}), nil)
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

	// Track in database
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

	fileSize := humanize.IBytes(uint64(file.FileSize))
	botUsername := ctx.Self.Username
	shareLink := fmt.Sprintf("https://t.me/%s?start=file_%d_%s", botUsername, messageID, hash)

	isVideo := strings.Contains(file.MimeType, "video")
	isMedia := isVideo ||
		strings.Contains(file.MimeType, "audio") ||
		strings.Contains(file.MimeType, "pdf")

	// Build styled reply text
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

	// Inline buttons
	var rows []tg.KeyboardButtonRow
	if isVideo {
		rows = append(rows, tg.KeyboardButtonRow{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonURL{Text: "sᴛʀᴇᴀᴍ", URL: streamLink},
				&tg.KeyboardButtonURL{Text: "ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
			},
		})
	} else {
		rows = append(rows, tg.KeyboardButtonRow{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonURL{Text: "ᴅᴏᴡɴʟᴏᴀᴅ", URL: downloadLink},
			},
		})
	}

	// Row 2: Get File + Revoke
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

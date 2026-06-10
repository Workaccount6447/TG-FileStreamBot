package commands

import (
	"context"
	"fmt"
	"strings"

	"EverythingSuckz/fsb/internal/database"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func (m *command) LoadClearFiles(d dispatcher.Dispatcher) {
	log := m.log.Named("clearfiles")
	defer log.Sugar().Info("Loaded")
	d.AddHandler(handlers.NewCommand("clearfiles", m.clearFiles))
}

func (m *command) clearFiles(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()

	if !database.IsEnabled() {
		ctx.Reply(u, ext.ReplyTextString("❌ Database not configured."), nil)
		return dispatcher.EndGroups
	}

	if database.GetDB().IsUserBanned(context.Background(), chatId) {
		ctx.Reply(u, ext.ReplyTextString(
			fmt.Sprintf("Sᴏʀʀʏ Sɪʀ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ ᴛᴏ ᴜsᴇ ᴍᴇ.\n\nContact Developer: %s", getUpdatesURL()),
		), nil)
		return dispatcher.EndGroups
	}

	_, total, err := database.GetDB().GetUserFiles(context.Background(), chatId, 0, 1)
	if err != nil || total == 0 {
		ctx.Reply(u, ext.ReplyTextString("Yᴏᴜ ʜᴀᴠᴇ ɴᴏ ꜰɪʟᴇs ᴛᴏ ᴄʟᴇᴀʀ."), nil)
		return dispatcher.EndGroups
	}

	confirmText := fmt.Sprintf(
		"⚠️ Are you sure?\n\n"+
			"This will permanently delete all %d file(s) from your history.\n"+
			"Stream links will stop working.\n\n"+
			"This cannot be undone.",
		total,
	)

	markup := &tg.ReplyInlineMarkup{
		Rows: []tg.KeyboardButtonRow{
			{Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "✅ Yᴇs, ᴅᴇʟᴇᴛᴇ ᴀʟʟ",
					Data: []byte("cf_confirm"),
				},
				&tg.KeyboardButtonCallback{
					Text: "❌ Cᴀɴᴄᴇʟ",
					Data: []byte("cf_cancel"),
				},
			}},
		},
	}

	ctx.Reply(u, ext.ReplyTextString(confirmText), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

func (m *command) clearFilesCallback(ctx *ext.Context, u *ext.Update, action string) error {
	query := u.CallbackQuery
	userID := u.EffectiveChat().GetID()

	ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
		QueryID: query.QueryID,
	})

	switch strings.TrimSpace(action) {
	case "confirm":
		deleted, err := database.GetDB().DeleteUserFiles(context.Background(), userID)
		var resultText string
		if err != nil {
			resultText = fmt.Sprintf("❌ Something went wrong: %s", err.Error())
		} else if deleted == 0 {
			resultText = "No files found to delete."
		} else {
			resultText = fmt.Sprintf(
				"✅ Done!\n\n%d file(s) have been removed from your history.\n\nAll stream links for these files are now invalid.",
				deleted,
			)
		}
		if u.CallbackQuery != nil {
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:        &tg.InputPeerUser{UserID: userID},
				ID:          u.CallbackQuery.MsgID,
				Message:     resultText,
				ReplyMarkup: &tg.ReplyInlineMarkup{},
			})
		}

	case "cancel":
		if u.CallbackQuery != nil {
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:        &tg.InputPeerUser{UserID: userID},
				ID:          u.CallbackQuery.MsgID,
				Message:     "Cancelled. Your files are safe. ✅",
				ReplyMarkup: &tg.ReplyInlineMarkup{},
			})
		}
	}

	return dispatcher.EndGroups
}

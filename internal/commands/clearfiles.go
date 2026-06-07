package commands

// Feature #7 — /clearfiles
//
// Lets a user wipe ALL their stored files from the DB in one command.
// Two-step confirmation to avoid accidental deletion:
//
//   Step 1: user sends /clearfiles
//           → bot sends a confirmation message with [Yes, delete all] + [Cancel] buttons
//
//   Step 2a: user taps [Yes, delete all]  (callback: cf_confirm)
//           → bot deletes all FileLink records for that user
//           → bot edits the message to show how many files were deleted
//
//   Step 2b: user taps [Cancel]            (callback: cf_cancel)
//           → bot edits the message to "Cancelled."

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
			fmt.Sprintf("__Sᴏʀʀʏ Sɪʀ, Yᴏᴜ ᴀʀᴇ Bᴀɴɴᴇᴅ ᴛᴏ ᴜsᴇ ᴍᴇ.__\n\n**[Cᴏɴᴛᴀᴄᴛ Dᴇᴠᴇʟᴏᴘᴇʀ](%s) Tʜᴇʏ Wɪʟʟ Hᴇʟᴘ Yᴏᴜ**", getUpdatesURL()),
		), nil)
		return dispatcher.EndGroups
	}

	// Check they actually have files first
	_, total, err := database.GetDB().GetUserFiles(context.Background(), chatId, 0, 1)
	if err != nil || total == 0 {
		ctx.Reply(u, ext.ReplyTextString("**Yᴏᴜ ʜᴀᴠᴇ ɴᴏ ꜰɪʟᴇs ᴛᴏ ᴄʟᴇᴀʀ.**"), nil)
		return dispatcher.EndGroups
	}

	// Step 1: show confirmation
	confirmText := fmt.Sprintf(
		"⚠️ **Aʀᴇ ʏᴏᴜ sᴜʀᴇ?**\n\n"+
			"This will permanently delete all **%d file(s)** from your history.\n"+
			"Stream links will **stop working**.\n\n"+
			"_This cannot be undone._",
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

// clearFilesCallback handles cf_confirm and cf_cancel.
// Called from handleCallback in start.go via the "cf_" prefix check.
func (m *command) clearFilesCallback(ctx *ext.Context, u *ext.Update, action string) error {
	query := u.CallbackQuery
	userID := u.EffectiveChat().GetID()

	// Answer immediately to remove the spinner
	ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
		QueryID: query.QueryID,
	})

	switch strings.TrimSpace(action) {
	case "confirm":
		deleted, err := database.GetDB().DeleteUserFiles(context.Background(), userID)
		var resultText string
		if err != nil {
			resultText = fmt.Sprintf("❌ **Something went wrong:** `%s`", err.Error())
		} else if deleted == 0 {
			resultText = "**ɴᴏ ꜰɪʟᴇs ꜰᴏᴜɴᴅ ᴛᴏ ᴅᴇʟᴇᴛᴇ.**"
		} else {
			resultText = fmt.Sprintf(
				"✅ **Dᴏɴᴇ!**\n\n"+
					"`%d` ꜰɪʟᴇ(s) ʜᴀᴠᴇ ʙᴇᴇɴ ʀᴇᴍᴏᴠᴇᴅ ꜰʀᴏᴍ ʏᴏᴜʀ ʜɪsᴛᴏʀʏ.\n\n"+
					"_Aʟʟ sᴛʀᴇᴀᴍ ʟɪɴᴋs ꜰᴏʀ ᴛʜᴇsᴇ ꜰɪʟᴇs ᴀʀᴇ ɴᴏᴡ ɪɴᴠᴀʟɪᴅ._",
				deleted,
			)
		}
		if u.EffectiveMessage != nil {
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:        &tg.InputPeerUser{UserID: userID},
				ID:          u.EffectiveMessage.ID,
				Message:     resultText,
				ReplyMarkup: &tg.ReplyInlineMarkup{}, // remove all buttons
			})
		}

	case "cancel":
		if u.EffectiveMessage != nil {
			ctx.Raw.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:        &tg.InputPeerUser{UserID: userID},
				ID:          u.EffectiveMessage.ID,
				Message:     "**Cᴀɴᴄᴇʟʟᴇᴅ.** Yᴏᴜʀ ꜰɪʟᴇs ᴀʀᴇ sᴀꜰᴇ. ✅",
				ReplyMarkup: &tg.ReplyInlineMarkup{},
			})
		}
	}

	return dispatcher.EndGroups
}

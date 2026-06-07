package commands

// Feature #4 — /stats (user-facing personal dashboard)
//
// Shows:
//   • Total files uploaded
//   • Total storage used across all files
//   • Total stream links generated (from users.links counter)
//   • Most recent file name + upload date
//   • Account join date

import (
	"context"
	"fmt"
	"time"

	"EverythingSuckz/fsb/internal/database"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/dustin/go-humanize"
	"github.com/gotd/td/tg"
)

func (m *command) LoadStats(d dispatcher.Dispatcher) {
	log := m.log.Named("stats")
	defer log.Sugar().Info("Loaded")
	d.AddHandler(handlers.NewCommand("stats", m.userStats))
}

func (m *command) userStats(ctx *ext.Context, u *ext.Update) error {
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

	// Send a "loading" message first — stats aggregation can take a moment
	loading, _ := ctx.Reply(u, ext.ReplyTextString("__Fᴇᴛᴄʜɪɴɢ ʏᴏᴜʀ sᴛᴀᴛs...__"), nil)

	stats, err := database.GetDB().GetUserStats(context.Background(), chatId)
	if err != nil || stats == nil {
		ctx.Reply(u, ext.ReplyTextString("❌ Could not fetch your stats. Try again later."), nil)
		return dispatcher.EndGroups
	}

	// Delete the loading message
	if loading != nil {
		ctx.Raw.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			Revoke: true,
			ID:     []int{loading.ID},
		})
	}

	// ── Most recent file line ──────────────────────────────────────────────
	recentLine := "__ɴᴏɴᴇ ʏᴇᴛ__"
	if stats.NewestFile != nil {
		recentLine = fmt.Sprintf(
			"`%s` (%s)",
			stats.NewestFile.FileName,
			humanize.Time(stats.NewestFile.CreatedAt),
		)
	}

	// ── Join date ──────────────────────────────────────────────────────────
	joinLine := "__ᴜɴᴋɴᴏᴡɴ__"
	if !stats.JoinDate.IsZero() {
		joinLine = stats.JoinDate.Format("02 Jan 2006")
	}

	// ── Account age ───────────────────────────────────────────────────────
	ageLine := ""
	if !stats.JoinDate.IsZero() {
		days := int(time.Since(stats.JoinDate).Hours() / 24)
		switch {
		case days == 0:
			ageLine = "joined today"
		case days == 1:
			ageLine = "1 day ago"
		default:
			ageLine = fmt.Sprintf("%d days ago", days)
		}
	}

	text := fmt.Sprintf(
		"**📊 Yᴏᴜʀ Sᴛᴀᴛs**\n\n"+
			"**👤 Aᴄᴄᴏᴜɴᴛ**\n"+
			"  ⬩ Jᴏɪɴᴇᴅ : `%s`",
		joinLine,
	)
	if ageLine != "" {
		text += fmt.Sprintf(" _(%s)_", ageLine)
	}
	text += fmt.Sprintf(
		"\n\n"+
			"**📁 Fɪʟᴇs**\n"+
			"  ⬩ Tᴏᴛᴀʟ ᴜᴘʟᴏᴀᴅᴇᴅ : `%d`\n"+
			"  ⬩ Tᴏᴛᴀʟ sɪᴢᴇ       : `%s`\n"+
			"  ⬩ Lɪɴᴋs ɢᴇɴᴇʀᴀᴛᴇᴅ : `%d`\n\n"+
			"**🕐 Mᴏsᴛ Rᴇᴄᴇɴᴛ Fɪʟᴇ**\n"+
			"  ⬩ %s",
		stats.TotalFiles,
		humanize.IBytes(uint64(stats.TotalSize)),
		stats.LinksCount,
		recentLine,
	)

	markup := &tg.ReplyInlineMarkup{
		Rows: []tg.KeyboardButtonRow{
			{Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{Text: "📂 Mʏ Fɪʟᴇs", Data: []byte("goto_myfiles")},
				&tg.KeyboardButtonCallback{Text: "ᴄʟᴏsᴇ", Data: []byte("close")},
			}},
		},
	}

	ctx.Reply(u, ext.ReplyTextString(text), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

package commands

import (
	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/database"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func isOwner(userID int64) bool {
	return config.ValueOf.OwnerID != 0 && userID == config.ValueOf.OwnerID
}

func ownerOnly(next func(*ext.Context, *ext.Update) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, u *ext.Update) error {
		user := u.EffectiveUser()
		if user == nil || !isOwner(user.ID) {
			ctx.Reply(u, ext.ReplyTextString("⛔ You are not authorised to use this command."), nil)
			return dispatcher.EndGroups
		}
		return next(ctx, u)
	}
}

func requireDB(ctx *ext.Context, u *ext.Update) bool {
	if !database.IsEnabled() {
		ctx.Reply(u, ext.ReplyTextString("❌ Database not configured. Set DATABASE_URL in your env file."), nil)
		return false
	}
	return true
}

func (m *command) LoadAdmin(d dispatcher.Dispatcher) {
	log := m.log.Named("admin")
	defer log.Sugar().Info("Loaded")
	d.AddHandler(handlers.NewCommand("ban", ownerOnly(banUser)))
	d.AddHandler(handlers.NewCommand("unban", ownerOnly(unbanUser)))
	d.AddHandler(handlers.NewCommand("status", ownerOnly(status)))
	d.AddHandler(handlers.NewCommand("broadcast", ownerOnly(broadcast)))
}

// /ban <user_id>
func banUser(ctx *ext.Context, u *ext.Update) error {
	if !requireDB(ctx, u) {
		return dispatcher.EndGroups
	}
	parts := strings.Fields(u.EffectiveMessage.Text)
	if len(parts) < 2 {
		ctx.Reply(u, ext.ReplyTextString("Usage: /ban <user_id>"), nil)
		return dispatcher.EndGroups
	}
	targetID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString("❌ Invalid user ID."), nil)
		return dispatcher.EndGroups
	}
	db := database.GetDB()
	bgCtx := context.Background()
	err = db.BanUser(bgCtx, targetID)
	if err == database.ErrUserAlreadyBanned {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("`%d` is Already Banned", targetID)), nil)
		return dispatcher.EndGroups
	}
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("❌ Error: %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	_ = db.DeleteUser(bgCtx, targetID)

	// Notify the banned user
	ctx.Raw.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     &tg.InputPeerUser{UserID: targetID},
		Message:  "**You are Banned from using this bot.**",
		RandomID: time.Now().UnixNano(),
	})

	ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("`%d` is Banned", targetID)), nil)
	return dispatcher.EndGroups
}

// /unban <user_id>
func unbanUser(ctx *ext.Context, u *ext.Update) error {
	if !requireDB(ctx, u) {
		return dispatcher.EndGroups
	}
	parts := strings.Fields(u.EffectiveMessage.Text)
	if len(parts) < 2 {
		ctx.Reply(u, ext.ReplyTextString("Usage: /unban <user_id>"), nil)
		return dispatcher.EndGroups
	}
	targetID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString("❌ Invalid user ID."), nil)
		return dispatcher.EndGroups
	}
	db := database.GetDB()
	bgCtx := context.Background()
	err = db.UnbanUser(bgCtx, targetID)
	if err == database.ErrUserNotBanned {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("`%d` is not Banned", targetID)), nil)
		return dispatcher.EndGroups
	}
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("❌ Error: %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	// Notify the unbanned user
	ctx.Raw.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     &tg.InputPeerUser{UserID: targetID},
		Message:  "**You are Unbanned! You can now use the bot.**",
		RandomID: time.Now().UnixNano(),
	})
	ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("`%d` is Unbanned", targetID)), nil)
	return dispatcher.EndGroups
}

// /status
func status(ctx *ext.Context, u *ext.Update) error {
	if !requireDB(ctx, u) {
		return dispatcher.EndGroups
	}
	db := database.GetDB()
	bgCtx := context.Background()
	totalUsers, _ := db.TotalUsers(bgCtx)
	bannedUsers, _ := db.TotalBanned(bgCtx)
	totalLinks, _ := db.TotalLinks(bgCtx)
	text := fmt.Sprintf(
		"**Total Users in DB:** `%d`\n**Banned Users in DB:** `%d`\n**Total Links Generated:** `%d`",
		totalUsers, bannedUsers, totalLinks,
	)
	ctx.Reply(u, ext.ReplyTextString(text), nil)
	return dispatcher.EndGroups
}

// /broadcast — reply to a message to broadcast it
func broadcast(ctx *ext.Context, u *ext.Update) error {
	if !requireDB(ctx, u) {
		return dispatcher.EndGroups
	}
	replyMsg := u.EffectiveMessage.ReplyToMessage
	if replyMsg == nil {
		ctx.Reply(u, ext.ReplyTextString("Reply to a message to broadcast it."), nil)
		return dispatcher.EndGroups
	}

	db := database.GetDB()
	bgCtx := context.Background()
	totalUsers, _ := db.TotalUsers(bgCtx)

	_, _ = ctx.Reply(u, ext.ReplyTextString(
		fmt.Sprintf("📡 Broadcast started to %d users...", totalUsers),
	), nil)

	cursor, err := db.GetAllUsers(bgCtx)
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("❌ DB error: %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	defer cursor.Close(bgCtx)

	start := time.Now()
	done, success, failed := 0, 0, 0
	var failedLog []string

	for cursor.Next(bgCtx) {
		var user database.User
		if err := cursor.Decode(&user); err != nil {
			continue
		}
		_, sendErr := ctx.Raw.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:     &tg.InputPeerUser{UserID: user.ID},
			Message:  replyMsg.Text,
			RandomID: time.Now().UnixNano(),
		})
		if sendErr != nil {
			failed++
			failedLog = append(failedLog, fmt.Sprintf("%d: %s", user.ID, sendErr.Error()))
		} else {
			success++
		}
		done++
	}

	elapsed := time.Since(start)
	result := fmt.Sprintf(
		"✅ Broadcast completed in `%s`\n\nTotal users: %d\nDone: %d | Success: %d | Failed: %d",
		elapsed.Round(time.Second).String(), totalUsers, done, success, failed,
	)
	if len(failedLog) > 0 {
		// Write failed log to file and send
		logContent := strings.Join(failedLog, "\n")
		_ = os.WriteFile("broadcast_failed.txt", []byte(logContent), 0644)
		ctx.Reply(u, ext.ReplyTextString(result+"\n\nFailed log saved to broadcast_failed.txt"), nil)
	} else {
		ctx.Reply(u, ext.ReplyTextString(result), nil)
	}
	return dispatcher.EndGroups
}

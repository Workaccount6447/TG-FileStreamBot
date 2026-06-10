package commands

import (
	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/database"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/telegram/message/styling"
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
	d.AddHandler(handlers.NewCommand("broadcast", ownerOnly(m.broadcast)))
}

func resolveTargetID(u *ext.Update) (targetID int64, display string, errMsg string) {
	parts := strings.Fields(u.EffectiveMessage.Text)

	if len(parts) >= 2 {
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, "", "❌ Invalid user ID. Provide a numeric ID or reply to a message."
		}
		return id, fmt.Sprintf("`%d`", id), ""
	}

	reply := u.EffectiveMessage.ReplyToMessage
	if reply != nil && reply.Message != nil {
		fromID := reply.Message.FromID
		if peerUser, ok := fromID.(*tg.PeerUser); ok {
			id := peerUser.UserID
			name := fmt.Sprintf("%d", id)
			if u.Entities != nil {
				if tgUser, exists := u.Entities.Users[id]; exists {
					if user, ok := tgUser.AsNotEmpty(); ok {
						name = user.FirstName
						if user.Username != "" {
							name = "@" + user.Username
						}
					}
				}
			}
			return id, fmt.Sprintf("[%s](tg://user?id=%d)", name, id), ""
		}
	}

	return 0, "", "Usage: `/ban <user_id>` or reply to a user's message with `/ban`"
}

func banUser(ctx *ext.Context, u *ext.Update) error {
	if !requireDB(ctx, u) {
		return dispatcher.EndGroups
	}

	targetID, displayName, errMsg := resolveTargetID(u)
	if errMsg != "" {
		ctx.Reply(u, ext.ReplyTextString(errMsg), nil)
		return dispatcher.EndGroups
	}

	if isOwner(targetID) {
		ctx.Reply(u, ext.ReplyTextString("❌ You cannot ban the owner."), nil)
		return dispatcher.EndGroups
	}

	db := database.GetDB()
	bgCtx := context.Background()
	err := db.BanUser(bgCtx, targetID)
	if err == database.ErrUserAlreadyBanned {
		ctx.Reply(u, ext.ReplyTextStyledTextArray([]styling.StyledTextOption{
			styling.Plain(displayName + " "),
			styling.Bold("is Already Banned"),
		}), nil)
		return dispatcher.EndGroups
	}
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("something went wrong: %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	_ = db.DeleteUser(bgCtx, targetID)

	ctx.Raw.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     &tg.InputPeerUser{UserID: targetID},
		Message:  "Yᴏᴜ ᴀʀᴇ ʙᴀɴɴᴇᴅ ꜰʀᴏᴍ ᴜsɪɴɢ ᴛʜɪs ʙᴏᴛ.",
		RandomID: rand.Int63(),
	})

	ctx.Reply(u, ext.ReplyTextStyledTextArray([]styling.StyledTextOption{
		styling.Plain(displayName + " "),
		styling.Bold("is Banned ✅"),
	}), nil)
	return dispatcher.EndGroups
}

func unbanUser(ctx *ext.Context, u *ext.Update) error {
	if !requireDB(ctx, u) {
		return dispatcher.EndGroups
	}

	targetID, displayName, errMsg := resolveTargetID(u)
	if errMsg != "" {
		ctx.Reply(u, ext.ReplyTextString(errMsg), nil)
		return dispatcher.EndGroups
	}

	db := database.GetDB()
	bgCtx := context.Background()
	err := db.UnbanUser(bgCtx, targetID)
	if err == database.ErrUserNotBanned {
		ctx.Reply(u, ext.ReplyTextStyledTextArray([]styling.StyledTextOption{
			styling.Plain(displayName + " "),
			styling.Bold("is not Banned"),
		}), nil)
		return dispatcher.EndGroups
	}
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("something went wrong: %s", err.Error())), nil)
		return dispatcher.EndGroups
	}

	ctx.Raw.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     &tg.InputPeerUser{UserID: targetID},
		Message:  "Yᴏᴜ ᴀʀᴇ ᴜɴʙᴀɴɴᴇᴅ. Yᴏᴜ ᴄᴀɴ ᴜsᴇ ᴛʜɪs ʙᴏᴛ ɴᴏᴡ.",
		RandomID: rand.Int63(),
	})

	ctx.Reply(u, ext.ReplyTextStyledTextArray([]styling.StyledTextOption{
		styling.Plain(displayName + " "),
		styling.Bold("is Unbanned ✅"),
	}), nil)
	return dispatcher.EndGroups
}

// /status — owner-only bot stats
func status(ctx *ext.Context, u *ext.Update) error {
	if !requireDB(ctx, u) {
		return dispatcher.EndGroups
	}
	db := database.GetDB()
	bgCtx := context.Background()
	totalUsers, _ := db.TotalUsers(bgCtx)
	bannedUsers, _ := db.TotalBanned(bgCtx)
	totalLinks, _ := db.TotalLinks(bgCtx)

	parts := []styling.StyledTextOption{
		styling.Bold("📊 Bᴏᴛ Sᴛᴀᴛᴜs\n\n"),
		styling.Bold("👥 Tᴏᴛᴀʟ Usᴇʀs : "), styling.Code(fmt.Sprintf("%d", totalUsers)), styling.Plain("\n"),
		styling.Bold("🚫 Bᴀɴɴᴇᴅ Usᴇʀs : "), styling.Code(fmt.Sprintf("%d", bannedUsers)), styling.Plain("\n"),
		styling.Bold("🔗 Lɪɴᴋs Gᴇɴᴇʀᴀᴛᴇᴅ : "), styling.Code(fmt.Sprintf("%d", totalLinks)),
	}
	ctx.Reply(u, ext.ReplyTextStyledTextArray(parts), nil)
	return dispatcher.EndGroups
}

func (m *command) broadcast(ctx *ext.Context, u *ext.Update) error {
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

	ctx.Reply(u, ext.ReplyTextString(
		"Broadcast initiated! You will be notified with log file when all the users are notified.",
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

	ownerID := u.EffectiveChat().GetID()

	for cursor.Next(bgCtx) {
		var user database.User
		if err := cursor.Decode(&user); err != nil {
			continue
		}

		var sendErr error
		if replyMsg.Media != nil {
			_, sendErr = ctx.Raw.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
				RandomID: []int64{rand.Int63()},
				FromPeer: &tg.InputPeerUser{UserID: ownerID},
				ID:       []int{replyMsg.ID},
				ToPeer:   &tg.InputPeerUser{UserID: user.ID},
			})
		} else {
			_, sendErr = ctx.Raw.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
				Peer:     &tg.InputPeerUser{UserID: user.ID},
				Message:  replyMsg.Text,
				RandomID: rand.Int63(),
			})
		}

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
		"broadcast completed in `%s`\n\nTotal users %d.\nTotal done %d, %d success and %d failed.",
		elapsed.Round(time.Second).String(), totalUsers, done, success, failed,
	)
	if len(failedLog) > 0 {
		logContent := strings.Join(failedLog, "\n")
		_ = os.WriteFile("broadcast.txt", []byte(logContent), 0644)
	}
	ctx.Reply(u, ext.ReplyTextString(result), nil)
	return dispatcher.EndGroups
}

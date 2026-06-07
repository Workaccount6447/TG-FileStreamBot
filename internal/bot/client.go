package bot

import (
	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/commands"
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/gotd/td/tg"
)

var Bot *gotgproto.Client

func StartClient(log *zap.Logger) (*gotgproto.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	resultChan := make(chan struct {
		client *gotgproto.Client
		err    error
	})
	go func(ctx context.Context) {
		client, err := gotgproto.NewClient(
			int(config.ValueOf.ApiID),
			config.ValueOf.ApiHash,
			gotgproto.ClientTypeBot(config.ValueOf.BotToken),
			&gotgproto.ClientOpts{
				// SimpleSession keeps session in memory — no SQLite file needed.
				// The bot will re-auth on every restart (token-based bots do this instantly).
				Session:          sessionMaker.SimpleSession(),
				DisableCopyright: true,
			},
		)
		resultChan <- struct {
			client *gotgproto.Client
			err    error
		}{client, err}
	}(ctx)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		if result.err != nil {
			return nil, result.err
		}
		commands.Load(log, result.client.Dispatcher, result.client)
		log.Info("Client started", zap.String("username", result.client.Self.Username))
		Bot = result.client
		// FIX Bug C: setBotCommands was defined but never called.
		// Register /start and /myfiles in Telegram's command menu.
		go setBotCommands(result.client)
		return result.client, nil
	}
}

func setBotCommands(client *gotgproto.Client) {
	ctx := client.CreateContext()
	ctx.Raw.BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		LangCode: "",
		Commands: []tg.BotCommand{
			{Command: "start", Description: "Start the bot"},
			{Command: "myfiles", Description: "Browse your uploaded files with stream links"},
			{Command: "stats", Description: "View your personal usage statistics"},
			{Command: "clearfiles", Description: "Delete all your files from history"},
		},
	})
}

package bot

import (
	"EverythingSuckz/fsb/config"
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/sessionMaker"
)

var Bot *gotgproto.Client

// StartClient connects the main bot account to Telegram and returns the
// client. This is web-only mode: no command dispatcher/handlers are loaded
// (no /start, /myfiles, /admin, etc.) — a separate bot process is assumed to
// handle user-facing commands and forwarding files into the log channel.
// This client is only used as the default worker for fetching/streaming
// files that are already in the log channel.
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
		log.Info("Client started", zap.String("username", result.client.Self.Username))
		Bot = result.client
		return result.client, nil
	}
}

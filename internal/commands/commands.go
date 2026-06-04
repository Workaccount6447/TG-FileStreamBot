package commands

import (
	"reflect"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher"
	"go.uber.org/zap"
)

type command struct {
	log    *zap.Logger
	client *gotgproto.Client
}

func Load(log *zap.Logger, dispatcher dispatcher.Dispatcher, client *gotgproto.Client) {
	log = log.Named("commands")
	defer log.Info("Initialized all command handlers")
	cmd := &command{log: log, client: client}
	Type := reflect.TypeOf(cmd)
	Value := reflect.ValueOf(cmd)
	for i := 0; i < Type.NumMethod(); i++ {
		Type.Method(i).Func.Call([]reflect.Value{Value, reflect.ValueOf(dispatcher)})
	}
}

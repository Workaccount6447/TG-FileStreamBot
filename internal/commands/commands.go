package commands

import (
	"reflect"
	"strings"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher"
	"go.uber.org/zap"
)

type command struct {
	log    *zap.Logger
	client *gotgproto.Client
}

func Load(log *zap.Logger, d dispatcher.Dispatcher, client *gotgproto.Client) {
	log = log.Named("commands")
	defer log.Info("Initialized all command handlers")
	cmd := &command{log: log, client: client}
	dispatcherType := reflect.TypeOf(d)
	cmdType := reflect.TypeOf(cmd)
	cmdValue := reflect.ValueOf(cmd)
	dispatcherValue := reflect.ValueOf(d)

	for i := 0; i < cmdType.NumMethod(); i++ {
		method := cmdType.Method(i)

		// Only call methods whose name starts with "Load" — skip all helpers,
		// callbacks, and other methods that have different signatures.
		// This was the root crash: the old code called every method on *command
		// with a dispatcher.Dispatcher arg, panicking on any method that
		// doesn't have that exact signature (all the new helper methods added).
		if !strings.HasPrefix(method.Name, "Load") {
			continue
		}

		// Verify the method takes exactly one argument of type dispatcher.Dispatcher
		// so we never panic on a Load* method with a different signature.
		mt := method.Type
		// mt.In(0) is the receiver (*command), mt.In(1) would be the first arg
		if mt.NumIn() != 2 {
			continue
		}
		if !mt.In(1).Implements(dispatcherType) && mt.In(1) != dispatcherType {
			continue
		}

		method.Func.Call([]reflect.Value{cmdValue, dispatcherValue})
	}
}

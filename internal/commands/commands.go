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

// dispatcherIfaceType is the reflect.Type of the dispatcher.Dispatcher interface.
// reflect.TypeOf((*dispatcher.Dispatcher)(nil)).Elem() is the correct way to get
// an interface type for use with Type.Implements() — passing a concrete value to
// reflect.TypeOf gives a concrete type, not an interface, which panics in Implements.
var dispatcherIfaceType = reflect.TypeOf((*dispatcher.Dispatcher)(nil)).Elem()

func Load(log *zap.Logger, d dispatcher.Dispatcher, client *gotgproto.Client) {
	log = log.Named("commands")
	defer log.Info("Initialized all command handlers")
	cmd := &command{log: log, client: client}
	cmdType := reflect.TypeOf(cmd)
	cmdValue := reflect.ValueOf(cmd)
	dispatcherValue := reflect.ValueOf(d)

	for i := 0; i < cmdType.NumMethod(); i++ {
		method := cmdType.Method(i)

		// Only call methods whose name starts with "Load"
		if !strings.HasPrefix(method.Name, "Load") {
			continue
		}

		// Verify signature: receiver(*command) + exactly 1 arg that satisfies dispatcher.Dispatcher
		// method.Type.In(0) = *command (receiver)
		// method.Type.In(1) = first argument
		mt := method.Type
		if mt.NumIn() != 2 {
			continue
		}
		argType := mt.In(1)
		// argType must either BE the interface or implement it
		if argType != dispatcherIfaceType && !argType.Implements(dispatcherIfaceType) {
			continue
		}

		method.Func.Call([]reflect.Value{cmdValue, dispatcherValue})
	}
}

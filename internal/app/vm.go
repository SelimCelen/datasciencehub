package app

import (
	"fmt"
	"time"

	"github.com/robertkrimen/otto"
)

func (app *AppContext) runScript(script *otto.Script, input interface{}, params map[string]interface{}) (interface{}, error) {
	vm := app.VMFactory()
	vm.Set("input", input)
	vm.Set("params", params)

	done := make(chan struct{})
	var value otto.Value
	var err error

	go func() {
		defer close(done)
		value, err = vm.Run(script)
	}()

	select {
	case <-done:
		if err != nil {
			return nil, err
		}
		return value.Export()
	case <-time.After(app.Config.JSTimeout):
		return nil, fmt.Errorf("execution timed out after %v", app.Config.JSTimeout)
	}
}

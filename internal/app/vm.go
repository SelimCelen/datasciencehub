package app

import (
	"github.com/dop251/goja"
)

func (app *AppContext) runScript(program *goja.Program, input interface{}, params map[string]interface{}) (interface{}, error) {
	vm := app.VMFactory()
	vm.Set("input", input)
	vm.Set("params", params)

	value, err := vm.RunProgram(program)
	if err != nil {
		return nil, err
	}

	return value.Export(), nil
}

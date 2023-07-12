package reflect

import "reflect"

var ErrType = MakeType[error]()

func GetErrorLocation(fn reflect.Type) (int, bool) {
	for i, output := range GetFuncOutputs(fn) {
		if output == ErrType {
			return i, true
		}
	}
	return 0, false
}

func GetFuncOutputs(fn reflect.Type) []reflect.Type {
	returnCount := fn.NumOut()
	out := make([]reflect.Type, 0, returnCount)
	for i := 0; i < returnCount; i++ {
		out = append(out, fn.Out(i))
	}
	return out
}

func GetFuncParams(fn reflect.Type) []reflect.Type {
	paramCount := fn.NumIn()
	params := make([]reflect.Type, 0, paramCount)
	for i := 0; i < paramCount; i++ {
		params = append(params, fn.In(i))
	}
	return params
}

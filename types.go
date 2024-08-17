package bodyclose

import (
	"go/types"

	"github.com/gostaticanalysis/analysisutil"
)

func methodOf(t types.Type, name string) *types.Func {
	switch t := t.(type) {
	case *types.Interface:
		for i := 0; i < t.NumMethods(); i++ {
			if f := t.Method(i); f.Name() == name {
				return f
			}
		}
	default:
		if f := analysisutil.MethodOf(t, name); f != nil {
			return f
		}
	}

	underlying := t.Underlying()
	if t != underlying {
		return methodOf(underlying, name)
	}

	return nil
}

func getField(t types.Type, i int) *types.Var {
	switch t := t.(type) {
	case *types.Pointer:
		return getField(t.Elem(), i)
	case *types.Named:
		return getField(t.Underlying(), i)
	case *types.Struct:
		if i < t.NumFields() {
			return t.Field(i)
		}
	}

	return nil
}

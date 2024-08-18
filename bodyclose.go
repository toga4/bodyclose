package bodyclose

import (
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"slices"

	"github.com/gostaticanalysis/analysisutil"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

var Analyzer = &analysis.Analyzer{
	Name: "bodyclose",
	Doc:  "bodyclose is ...",
	Run:  run,
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
	},
}

func run(pass *analysis.Pass) (interface{}, error) {
	httpResponseType := analysisutil.TypeOf(pass, "net/http", "*Response")
	_, bodyVar := analysisutil.Field(httpResponseType, "Body")
	ioReadCloserType := bodyVar.Type()
	closeMethod := methodOf(ioReadCloserType, "Close")

	fmt.Printf("httpResponseType: %v\n", httpResponseType)
	fmt.Printf("ioReadCloseType: %v\n", ioReadCloserType)
	fmt.Printf("bodyVar: %v\n", bodyVar)
	fmt.Printf("closeMethod: %v\n", closeMethod)

	runner := &runner{
		pass:             pass,
		httpResponseType: httpResponseType,
		ioReadCloseType:  ioReadCloserType,
		bodyVar:          bodyVar,
		closeMethod:      closeMethod,
		debug:            true,
	}

	runner.run(pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA).SrcFuncs)

	return nil, nil
}

type runner struct {
	pass *analysis.Pass

	httpResponseType types.Type
	ioReadCloseType  types.Type
	bodyVar          *types.Var
	closeMethod      *types.Func

	debug bool
}

func (r *runner) isHttpResponseType(t types.Type) bool {
	return reflect.DeepEqual(r.httpResponseType, t)
}

func (r *runner) isIoReadCloserType(t types.Type) bool {
	return reflect.DeepEqual(r.ioReadCloseType, t)
}

func (r *runner) isBodyVarFieldAddr(fa *ssa.FieldAddr) bool {
	return reflect.DeepEqual(r.bodyVar, getField(fa.X.Type(), fa.Field))
}

func (r *runner) isCloseMethod(f *types.Func) bool {
	return reflect.DeepEqual(r.closeMethod, f)
}

func (r *runner) run(funcs []*ssa.Function) {
	for _, f := range funcs {
		r.logFunction(f)

		for _, b := range f.Blocks {
			for _, instr := range b.Instrs {
				ins := &inspector{
					runner: r,
				}

				r.logInstruction(instr)

				call, ok := ins.asCallReturnsResp(instr)
				if !ok {
					continue
				}

				// Skip if all returned *http.Response vars are already closed in the static callee.
				if staticCallee := call.Call.StaticCallee(); staticCallee != nil {
					if ins.inspectFunction(staticCallee) {
						continue
					}
				}

				if !ins.inspectCallReturnsResp(call) {
					r.pass.Reportf(call.Pos(), "response body must be closed")
				}

				r.logInspections(ins)
			}
		}
	}
}

type inspector struct {
	*runner
	traces []string
}

func (r *inspector) asCallReturnsResp(instr ssa.Instruction) (*ssa.Call, bool) {
	call, ok := instr.(*ssa.Call)
	if !ok {
		return nil, false
	}

	results := call.Call.Signature().Results()
	for i := 0; i < results.Len(); i++ {
		if r.isHttpResponseType(results.At(i).Type()) {
			return call, true
		}
	}

	return nil, false
}

func (r *inspector) inspectFunction(f *ssa.Function) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectFunction:\t%[1]p\t%[1]v", f))

	callResults := []bool{}

	for _, b := range f.Blocks {
		for _, instr := range b.Instrs {
			ins := &inspector{
				runner: r.runner,
			}

			call, ok := ins.asCallReturnsResp(instr)
			if !ok {
				continue
			}

			// Skip if all returned *http.Response vars are already closed in the static callee.
			if staticCallee := call.Call.StaticCallee(); staticCallee != nil {
				if ins.inspectFunction(staticCallee) {
					continue
				}
			}

			callResults = append(callResults, ins.inspectCallReturnsResp(call))
		}
	}

	// if no call instruction, it means the function does not return *http.Response.
	if len(callResults) == 0 {
		return false
	}

	// Check if all returned responses are closed.
	for _, res := range callResults {
		if !res {
			return false
		}
	}
	return true
}

func (r *inspector) inspectCallReturnsResp(call *ssa.Call) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectCallReturnsResp:\t%[1]p\t%[1]v", call))

	results := []bool{}

	for _, referrer := range *call.Referrers() {
		switch ref := referrer.(type) {
		case *ssa.Extract:
			if r.isHttpResponseType(ref.Type()) {
				results = append(results, r.inspectExtractResponse(ref))
			}
		case *ssa.FieldAddr:
			if r.isHttpResponseType(ref.X.Type()) {
				results = append(results, r.inspectFieldAddrBody(ref))
			}
		}
	}

	// if no referrer using response, it means the response is not used.
	if len(results) == 0 {
		return false
	}

	// Check if all returned responses are closed.
	for _, res := range results {
		if !res {
			return false
		}
	}
	return true
}

func (r *inspector) inspectExtractResponse(extract *ssa.Extract) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectExtract:\t\t%[1]p\t%[1]v", extract))

	for _, referrer := range *extract.Referrers() {
		switch ref := referrer.(type) {
		case *ssa.FieldAddr:
			if r.inspectFieldAddrBody(ref) {
				return true
			}
		case *ssa.Store:
			if r.inspectStore(ref) {
				return true
			}
		case *ssa.Call:
			if r.inspectCallInstr(ref, extract) {
				return true
			}
		case *ssa.Return:
			// return response without closing the body. It means the caller should close the body of the response.
			return true
		}
	}
	return false
}

func (r *inspector) inspectFieldAddrBody(fieldAddr *ssa.FieldAddr) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectFieldAddr:\t%[1]p\t%[1]v", fieldAddr))

	if !r.isBodyVarFieldAddr(fieldAddr) {
		return false
	}

	for _, ref := range *fieldAddr.Referrers() {
		switch ref := ref.(type) {
		case *ssa.UnOp:
			if r.inspectUnOp(ref) {
				return true
			}
		}
	}
	return false
}

func (r *inspector) inspectUnOp(unOp *ssa.UnOp) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectUnOp:\t\t%[1]p\t%[1]v", unOp))

	// Skip if not dereference
	if unOp.Op != token.MUL {
		return false
	}
	t, ok := unOp.X.Type().(*types.Pointer)
	if !ok {
		return false
	}

	switch t := t.Elem(); {
	case r.isHttpResponseType(t):
		return r.inspectDereferResponse(unOp)
	case r.isIoReadCloserType(t):
		return r.inspectDereferBody(unOp)
	default:
		r.traces[len(r.traces)-1] += fmt.Sprintf("\ttype=%s", t.String())
	}
	return false
}

func (r *inspector) inspectDereferResponse(unOp *ssa.UnOp) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectDereferResponse:\t%[1]p\t%[1]v", unOp))

	for _, ref := range *unOp.Referrers() {
		switch ref := ref.(type) {
		case *ssa.FieldAddr:
			if r.inspectFieldAddrBody(ref) {
				return true
			}
		case *ssa.Store:
			if r.inspectStore(ref) {
				return true
			}
		}
	}
	return false
}

func (r *inspector) inspectDereferBody(unOp *ssa.UnOp) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectDereferBody:\t%[1]p\t%[1]v", unOp))

	for _, ref := range *unOp.Referrers() {
		switch ref := ref.(type) {
		case *ssa.Store:
			if r.inspectStore(ref) {
				return true
			}
		case *ssa.ChangeInterface:
			if r.inspectChangeInterface(ref) {
				return true
			}
		case *ssa.Call:
			if r.inspectCallInstr(ref, unOp) {
				return true
			}
		case *ssa.Defer:
			if r.inspectCallInstr(ref, unOp) {
				return true
			}
		case *ssa.Return:
			// return body without closing. It means the caller should close the body.
			return true
		}
	}
	return false
}

func (r *inspector) inspectChangeInterface(changeInterface *ssa.ChangeInterface) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectChangeInterface:\t%[1]p\t%[1]v", changeInterface))

	for _, ref := range *changeInterface.Referrers() {
		switch ref := ref.(type) {
		case *ssa.Store:
			if r.inspectStore(ref) {
				return true
			}
		case *ssa.Call:
			if r.inspectCallInstr(ref, changeInterface) {
				return true
			}
		case *ssa.Defer:
			if r.inspectCallInstr(ref, changeInterface) {
				return true
			}
		}
	}
	return false
}

func (r *inspector) inspectStore(store *ssa.Store) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectStore:\t\t%[1]p\t%[1]v, val=%v", store, store.Val))

	for _, ref := range *store.Addr.Referrers() {
		switch ref := ref.(type) {
		case *ssa.MakeClosure:
			// The store instruction stored value to use in closure. The type of the value is *http.Response or io.ReadCloser.
			if r.isClosureCalled(ref) {
				switch addr := store.Addr.(type) {
				case *ssa.Alloc:
					if r.inspectClosure(ref.Fn.(*ssa.Function), addr.Comment) {
						return true
					}
				}
			}
		}
	}
	return false
}

func (r *inspector) inspectClosure(f *ssa.Function, bindingValue string) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectClosure:\t\t%[1]p\t%[1]v, varName=%s", f, bindingValue))

	for _, b := range f.Blocks {
		for _, instr := range b.Instrs {
			switch instr := instr.(type) {
			case *ssa.UnOp:
				if instr.X.Name() == bindingValue && r.inspectUnOp(instr) {
					return true
				}
			}
		}
	}
	return false
}

func (r *inspector) isClosureCalled(mc *ssa.MakeClosure) bool {
	r.traces = append(r.traces, fmt.Sprintf("isClosureCalled:\t%[1]p\t%[1]v", mc))

	for _, ref := range *mc.Referrers() {
		switch ref.(type) {
		case *ssa.Defer, *ssa.Call:
			return true
		}
	}
	return false
}

func (r *inspector) inspectCallInstr(call ssa.CallInstruction, arg ssa.Value) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectCallInstr:\t%[1]p\t%[1]v", call))

	if r.isCloseCallInstr(call) {
		return true
	}

	if !r.isInCallParameter(call, arg) {
		return false
	}

	for _, param := range call.Common().StaticCallee().Params {
		if r.inspectCallParameter(param) {
			return true
		}
	}

	return false
}

func (r *inspector) isCloseCallInstr(call ssa.CallInstruction) bool {
	r.traces = append(r.traces, fmt.Sprintf("isCloseCallInstr:\t%[1]p\t%[1]v", call))
	return r.isCloseMethod(call.Common().Method)
}

func (r *inspector) isInCallParameter(call ssa.CallInstruction, arg ssa.Value) bool {
	r.traces = append(r.traces, fmt.Sprintf("isInCallParameter:\t%[1]p\t%[1]v", call))

	switch fn := call.Common().Value.(type) {
	case *ssa.Function:
		if fn.Signature.Recv() != nil {
			return false
		}
	default:
		return false
	}

	return slices.Contains(call.Common().Args, arg)
}

func (r *inspector) inspectCallParameter(param *ssa.Parameter) bool {
	r.traces = append(r.traces, fmt.Sprintf("inspectCallParameter:\t%[1]p\t%[1]v", param))

	for _, ref := range *param.Referrers() {
		switch ref := ref.(type) {
		case *ssa.FieldAddr:
			if r.inspectFieldAddrBody(ref) {
				return true
			}
		case *ssa.Store:
			if r.inspectStore(ref) {
				return true
			}
		case *ssa.Call:
			if r.inspectCallInstr(ref, param) {
				return true
			}
		case *ssa.Defer:
			if r.inspectCallInstr(ref, param) {
				return true
			}
		}
	}

	return false
}

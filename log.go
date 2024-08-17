package bodyclose

import (
	"fmt"
	"strings"

	"golang.org/x/tools/go/ssa"
)

func (r *runner) logFunction(f *ssa.Function) {
	if !r.debug {
		return
	}

	fmt.Printf("\n\n\n(%[1]p) %[1]v\n", f)
}

func (r *runner) logInstruction(instr ssa.Instruction) {
	if !r.debug {
		return
	}

	b := instr.Block().String()
	t := fmt.Sprintf("%T", instr)
	fmt.Printf("%[1]v\t%[2]p\t%[3]s%[4]s%[2]v\n", b, instr, t, fitTabs(t, 3))

	if instr, ok := instr.(ssa.Value); ok {
		if instr.Referrers() != nil {
			fmt.Printf("\t[Referrers]\n")
			for _, ref := range *instr.Referrers() {
				fmt.Printf("\t\t%[1]p %[1]T %[1]v\n", ref)
			}
		}
	}

	switch instr := instr.(type) {
	case *ssa.Call:
		fmt.Printf("\t[Call.Common]\n")
		fmt.Printf("\t\t%[1]p %[1]T %[1]v\n", instr.Call.StaticCallee())
	case *ssa.Store:
		fmt.Printf("\t[Addr.Name] %[1]v\n", instr.Addr.Name())
		fmt.Printf("\t[Addr.Referrers]\n")
		for _, ref := range *instr.Addr.Referrers() {
			fmt.Printf("\t\t%[1]p %[1]T %[1]v\n", ref)
		}
	case *ssa.MakeClosure:
		fmt.Printf("\t[Fn]\n")
		fmt.Printf("\t\t%[1]p %[1]T %[1]v\n", instr.Fn)
	case *ssa.Return:
		if len(instr.Results) != 0 {
			fmt.Printf("\t[Results]\n")
		}
		for _, result := range instr.Results {
			fmt.Printf("\t\t%[1]p %[1]T %[1]v\n", result)
		}
	}
}

func (r *runner) logInspections(i *inspector) {
	if !r.debug {
		return
	}

	fmt.Printf("\t[Inspection Traces]\n")
	for _, trace := range i.traces {
		fmt.Printf("\t\t%s\n", trace)
	}
}

func fitTabs(s string, max int) string {
	t := (max*8 - len(s) - 1) / 8
	return strings.Repeat("\t", 1+t)
}

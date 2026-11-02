package errors

import (
	"runtime"
)

const maxStackPCs = 64

type trace struct {
	message   string
	pcs       []uintptr
	truncated bool
}

func capture(message string) trace {
	var pcs [maxStackPCs + 1]uintptr
	// Skip runtime.Callers, capture, and the public constructor.
	n := runtime.Callers(3, pcs[:])
	truncated := n > maxStackPCs
	if truncated {
		n = maxStackPCs
	}
	return trace{message: message, pcs: pcs[:n:n], truncated: truncated}
}

func (t *trace) Error() string { return t.message }

func (t *trace) stack() *trace { return t }

func frames(pcs []uintptr) []runtime.Frame {
	if len(pcs) == 0 {
		return nil
	}
	iter := runtime.CallersFrames(pcs)
	var result []runtime.Frame
	for {
		frame, more := iter.Next()
		result = append(result, frame)
		if !more {
			return result
		}
	}
}

func commonSuffix(child, parent []runtime.Frame) int {
	n := 0
	// Keep the capture frame, even when the entire stack matches.
	for n < len(child)-1 && n < len(parent) {
		a, b := child[len(child)-1-n], parent[len(parent)-1-n]
		if a.PC != b.PC || a.Function != b.Function || a.File != b.File || a.Line != b.Line {
			break
		}
		n++
	}
	return n
}

package errors

import (
	"fmt"
	"io"
	"runtime"
	"strings"
)

const (
	maxTreeDepth = 64
	maxTreeNodes = 1024
)

// Formatter returns an output-only adapter for err. It does not capture a
// stack or change the error tree. %+v traverses both forms of Unwrap, including
// standard-library wrappers; other string formats use err.Error().
//
// A nil error formats as "<nil>". Traversal stops at a depth of 64 or 1024
// nodes and prints an omission marker, including for cyclic error trees.
// Error and Unwrap methods must themselves terminate.
func Formatter(err error) fmt.Formatter { return formatter{err} }

type formatter struct{ err error }

func (f formatter) Format(s fmt.State, verb rune) {
	message := "<nil>"
	if f.err != nil {
		message = f.err.Error()
	}
	format(s, verb, message, f.err)
}

func format(s fmt.State, verb rune, message string, err error) {
	if verb != 'v' || !s.Flag('+') {
		fmt.Fprintf(s, fmt.FormatString(s, verb), message)
		return
	}
	io.WriteString(s, message)
	if err == nil {
		return
	}
	io.WriteString(s, "\n\ntraces:\n")
	p := treePrinter{out: s}
	p.visit(err, "root", 0, nil)
}

type treePrinter struct {
	out   io.Writer
	nodes int
}

func (p *treePrinter) visit(err error, path string, depth int, parent []runtime.Frame) {
	indent := strings.Repeat("  ", depth+1)
	p.nodes++
	if depth >= maxTreeDepth {
		fmt.Fprintf(p.out, "%s%s ... depth limit reached\n", indent, path)
		return
	}
	t, traced := err.(interface{ stack() *trace })
	kind := "untraced"
	if traced {
		kind = "capture"
	} else {
		switch err.(type) {
		case interface{ Unwrap() []error }:
			kind = "standard join"
		case interface{ Unwrap() error }:
			kind = "standard wrapper"
		}
	}
	fmt.Fprintf(p.out, "%s%s [%s]\n", indent, path, kind)
	if depth != 0 {
		fmt.Fprintf(p.out, "%s  %s\n", indent, strings.ReplaceAll(err.Error(), "\n", "\n"+indent+"  "))
	}
	if traced {
		stack := t.stack()
		current := frames(stack.pcs)
		common := 0
		// A truncated tail is not necessarily the bottom of a stack.
		if !stack.truncated {
			common = commonSuffix(current, parent)
		}
		for _, frame := range current[:len(current)-common] {
			fmt.Fprintf(p.out, "%s  %s\n%s    %s:%d\n", indent, frame.Function, indent, frame.File, frame.Line)
		}
		if common > 0 {
			fmt.Fprintf(p.out, "%s  ... %d common caller frames omitted\n", indent, common)
		}
		if stack.truncated {
			fmt.Fprintf(p.out, "%s  ... stack truncated after %d PCs\n", indent, maxStackPCs)
			parent = nil
		} else {
			parent = current
		}
	}
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		for i, child := range e.Unwrap() {
			if child != nil && !p.child(child, fmt.Sprintf("%s/%d", path, i), depth+1, parent) {
				break
			}
		}
	case interface{ Unwrap() error }:
		if child := e.Unwrap(); child != nil {
			p.child(child, path+"/0", depth+1, parent)
		}
	}
}

func (p *treePrinter) child(err error, path string, depth int, parent []runtime.Frame) bool {
	if p.nodes >= maxTreeNodes {
		fmt.Fprintf(p.out, "%s%s ... node limit reached\n", strings.Repeat("  ", depth+1), path)
		return false
	}
	p.visit(err, path, depth, parent)
	return true
}

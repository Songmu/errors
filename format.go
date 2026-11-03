package errors

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"unicode/utf8"
)

const (
	maxTreeDepth    = 64
	maxTreeNodes    = 1024
	maxMessageBytes = 256
	maxMessageLines = 4
)

// Formatter returns an output-only adapter for err. It does not capture a
// stack or change the error tree. %+v traverses both forms of Unwrap, including
// standard-library wrappers; other string formats use err.Error().
//
// A nil error formats as "<nil>". Traversal stops at a depth of 64 or 1024
// nodes and prints an omission marker, including for cyclic error trees.
// The root message is complete when Error succeeds. Nested Join messages are
// summarized; other nested messages are limited to four lines and 256 bytes
// with an omission marker.
// Panics are marked without suppressing later siblings.
// Error and Unwrap methods must themselves terminate.
func Formatter(err error) fmt.Formatter { return formatter{err} }

type formatter struct{ err error }

func (f formatter) Format(s fmt.State, verb rune) {
	format(s, verb, f.err)
}

func format(s fmt.State, verb rune, err error) {
	if verb == 'v' && s.Flag('+') {
		formatDetailed(s, err)
		return
	}
	message := "<nil>"
	if err != nil {
		message = err.Error()
	}
	if (verb == 'v' || verb == 's') &&
		!s.Flag(' ') && !s.Flag('+') && !s.Flag('-') && !s.Flag('#') && !s.Flag('0') {
		_, hasWidth := s.Width()
		_, hasPrecision := s.Precision()
		if !hasWidth && !hasPrecision {
			io.WriteString(s, message)
			return
		}
	}
	fmt.Fprintf(s, fmt.FormatString(s, verb), message)
}

func formatDetailed(s fmt.State, err error) {
	message := "<nil>"
	if err != nil {
		message = diagnosticMessage(err)
	}
	io.WriteString(s, message)
	if err == nil {
		return
	}
	io.WriteString(s, "\n\ntraces:\n")
	p := treePrinter{out: s}
	p.visit(err, "root", 0, nil)
}

func diagnosticMessage(err error) (message string) {
	defer func() {
		if recover() != nil {
			message = "<error message unavailable (panic)>"
		}
	}()
	return err.Error()
}

type treePrinter struct {
	out   io.Writer
	nodes int
}

func (p *treePrinter) visit(err error, path string, depth int, parent []runtime.Frame) {
	indent := strings.Repeat("  ", depth+1)
	defer func() {
		if recover() != nil {
			fmt.Fprintf(p.out, "%s%s ... node details unavailable (panic)\n", indent, path)
		}
	}()
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
			kind = "join"
		case interface{ Unwrap() error }:
			kind = "wrapper"
		}
	}
	fmt.Fprintf(p.out, "%s%s [%s]\n", indent, path, kind)
	if depth != 0 {
		fmt.Fprintf(p.out, "%s  %s\n", indent, strings.ReplaceAll(nodeMessage(err), "\n", "\n"+indent+"  "))
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
			unit := "frames"
			if common == 1 {
				unit = "frame"
			}
			fmt.Fprintf(p.out, "%s  ... %d common caller %s omitted\n", indent, common, unit)
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

func nodeMessage(err error) string {
	if e, ok := err.(*multiError); ok && e.joined != nil {
		unit := "causes"
		if len(e.children) == 1 {
			unit = "cause"
		}
		return fmt.Sprintf("%d joined %s (see child details)", len(e.children), unit)
	}
	message := diagnosticMessage(err)
	end := len(message)
	if end > maxMessageBytes {
		end = maxMessageBytes
		for end > 0 && !utf8.RuneStart(message[end]) {
			end--
		}
	}
	lines := 1
	for i := 0; i < end; i++ {
		if message[i] == '\n' {
			if lines == maxMessageLines {
				end = i
				break
			}
			lines++
		}
	}
	if end < len(message) {
		return message[:end] + "\n... message truncated"
	}
	return message
}

func (p *treePrinter) child(err error, path string, depth int, parent []runtime.Frame) bool {
	if p.nodes >= maxTreeNodes {
		fmt.Fprintf(p.out, "%s%s ... node limit reached\n", strings.Repeat("  ", depth+1), path)
		return false
	}
	p.visit(err, path, depth, parent)
	return true
}

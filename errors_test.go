package errors

import (
	stderrors "errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

type namedError string

func (e namedError) Error() string { return string(e) }

type nilError struct{}

func (*nilError) Error() string { return "typed nil" }

type mutableError struct {
	message string
	calls   int
}

func (e *mutableError) Error() string {
	e.calls++
	return e.message
}

func TestJoinLazyMessage(t *testing.T) {
	var typedNil *mutableError
	if err := Join(typedNil); err == nil || !Is(err, typedNil) {
		t.Fatal("Join must retain typed nil without calling Error")
	}
	child := &mutableError{message: "before"}
	got, want := Join(child, io.EOF), stderrors.Join(child, io.EOF)
	for range 100 {
		got = Join(got, io.EOF)
		want = stderrors.Join(want, io.EOF)
	}
	if child.calls != 0 {
		t.Fatal("Join evaluated child messages during construction")
	}
	child.message = "after"
	if got.Error() != want.Error() {
		t.Fatal("Join did not reflect the current child message")
	}
	if text := fmt.Sprintf("%+v", got); !strings.HasPrefix(text, "after\nEOF") {
		t.Fatal("detailed Join formatting used a stale message")
	}
	for _, format := range []string{"%v", "%s"} {
		for _, value := range []any{got, Formatter(got)} {
			child.message += " latest"
			expected := want.Error()
			calls := child.calls
			if text := fmt.Sprintf(format, value); text != expected {
				t.Fatal("ordinary Join formatting used a stale message")
			}
			if child.calls != calls+1 {
				t.Fatal("ordinary Join formatting must evaluate each child message once")
			}
		}
	}
}

func TestConstructors(t *testing.T) {
	a, b := stderrors.New("a"), namedError("b")
	standardMulti, tracedMulti := stderrors.Join(a, b), Join(a, b)
	var typedNil *nilError
	invalidFormat := "%w"
	tests := []struct {
		name string
		want error
		got  error
	}{
		{"new", stderrors.New("hello"), New("hello")},
		{"plain", fmt.Errorf("value=%d", 42), Errorf("value=%d", 42)},
		{"single", fmt.Errorf("read: %w", a), Errorf("read: %w", a)},
		{"multiple", fmt.Errorf("a=%w; b=%w", a, b), Errorf("a=%w; b=%w", a, b)},
		{"indexed", fmt.Errorf("%[2]w / %[1]w / %[2]w", a, b), Errorf("%[2]w / %[1]w / %[2]w", a, b)},
		{"invalid", fmt.Errorf(invalidFormat, 42), Errorf(invalidFormat, 42)},
		{"nil wrap", fmt.Errorf("%w", nil), Errorf("%w", nil)},
		{"typed nil wrap", fmt.Errorf("%w", typedNil), Errorf("%w", typedNil)},
		{"join", stderrors.Join(nil, a, b), Join(nil, a, b)},
		{"single join", stderrors.Join(a), Join(a)},
		{"single standard multi join", stderrors.Join(standardMulti), Join(standardMulti)},
		{"single traced multi join", stderrors.Join(tracedMulti), Join(tracedMulti)},
		{"nil join", stderrors.Join(nil, nil), Join(nil, nil)},
		{"empty join", stderrors.Join(), Join()},
		{"typed nil join", stderrors.Join(typedNil), Join(typedNil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want == nil || tt.got == nil {
				if tt.want != tt.got {
					t.Fatalf("nil mismatch: got %v, want %v", tt.got, tt.want)
				}
				return
			}
			if tt.got.Error() != tt.want.Error() {
				t.Fatalf("got %q, want %q", tt.got.Error(), tt.want.Error())
			}
			wantSingle, ws := tt.want.(interface{ Unwrap() error })
			gotSingle, gs := tt.got.(interface{ Unwrap() error })
			wantMulti, wm := tt.want.(interface{ Unwrap() []error })
			gotMulti, gm := tt.got.(interface{ Unwrap() []error })
			if ws != gs || wm != gm {
				t.Fatalf("unwrap shape: got (%v,%v), want (%v,%v)", gs, gm, ws, wm)
			}
			if ws && gotSingle.Unwrap() != wantSingle.Unwrap() {
				t.Fatal("single cause changed")
			}
			if wm && !reflect.DeepEqual(gotMulti.Unwrap(), wantMulti.Unwrap()) {
				t.Fatal("multiple causes changed")
			}
			if Unwrap(tt.got) != stderrors.Unwrap(tt.want) {
				t.Fatal("Unwrap differs from standard library")
			}
		})
	}
	if New("same") == New("same") {
		t.Fatal("New must return distinct errors")
	}
}

type matchError struct{ target error }

func (e matchError) Error() string        { return "custom match" }
func (e matchError) Is(target error) bool { return target == e.target }
func (e matchError) As(target any) bool {
	if p, ok := target.(*namedError); ok {
		*p = "custom"
		return true
	}
	return false
}

func TestSearch(t *testing.T) {
	target := namedError("target")
	err := Errorf("outer: %w", Join(io.EOF, target, ErrUnsupported))
	if !Is(err, io.EOF) || !Is(err, stderrors.ErrUnsupported) || ErrUnsupported != stderrors.ErrUnsupported {
		t.Fatal("sentinel search failed")
	}
	var got namedError
	if !As(err, &got) || got != target {
		t.Fatal("As failed")
	}
	if got, ok := AsType[namedError](err); !ok || got != target {
		t.Fatal("AsType failed")
	}
	custom := Errorf("%w", matchError{target: io.EOF})
	if !Is(custom, io.EOF) || !As(custom, &got) || got != "custom" {
		t.Fatal("custom matching failed")
	}
	if got, ok := AsType[namedError](custom); !ok || got != "custom" {
		t.Fatal("custom AsType failed")
	}
	if _, ok := AsType[namedError](nil); ok || As(nil, &got) || !Is(nil, nil) {
		t.Fatal("nil search differs")
	}
}

func TestCaptureCallSite(t *testing.T) {
	for _, constructor := range []string{"New", "Errorf", "Join"} {
		t.Run(constructor, func(t *testing.T) {
			var err error
			var line int
			_, file, _, _ := runtime.Caller(0)
			switch constructor {
			case "New":
				_, _, line, _ = runtime.Caller(0)
				err = New("new")
			case "Errorf":
				_, _, line, _ = runtime.Caller(0)
				err = Errorf("wrapped: %w", io.EOF)
			case "Join":
				_, _, line, _ = runtime.Caller(0)
				err = Join(io.EOF)
			}
			first := frames(err.(interface{ stack() *trace }).stack().pcs)[0]
			if first.File != file || first.Line != line+1 || !strings.Contains(first.Function, "TestCaptureCallSite") {
				t.Fatalf("first frame: %+v; want %s:%d", first, file, line+1)
			}
		})
	}
}

func TestOrdinaryFormatting(t *testing.T) {
	for _, err := range []error{New("héllo\nworld"), Errorf("read: %w", io.EOF), Join(io.EOF, io.ErrClosedPipe)} {
		for _, format := range []string{"%s", "%v", "%q", "%+q", "%#q", "%20s", "%-20v", "%.3s", "%.0v", "%20.3q", "%#v", "%x", "% v", "%+s", "%-v", "%#s", "%0s"} {
			want := fmt.Sprintf(format, err.Error())
			for _, value := range []any{err, Formatter(err)} {
				if got := fmt.Sprintf(format, value); got != want {
					t.Errorf("%s: got %q, want %q", format, got, want)
				}
			}
		}
	}
	for _, format := range []string{"%v", "%+v", "%s", "%q"} {
		if got, want := fmt.Sprintf(format, Formatter(nil)), fmt.Sprintf(format, "<nil>"); got != want {
			t.Fatalf("nil: got %q, want %q", got, want)
		}
	}
	for _, format := range []string{"%v", "%s"} {
		if got := fmt.Sprintf(format, Formatter((*nilError)(nil))); got != "typed nil" {
			t.Fatalf("safe typed nil: got %q, want %q", got, "typed nil")
		}
	}
	child := New("child")
	format := "details: %+w"
	if got, want := Errorf(format, child).Error(), fmt.Errorf(format, child).Error(); got != want {
		t.Fatalf("flagged wrap: got %q, want %q", got, want)
	}
}

func TestDetailedTree(t *testing.T) {
	a, b := New("first"), New("second")
	standard := fmt.Errorf("batch: %w", stderrors.Join(a, b))
	err := Errorf("operation: %w", standard)
	for _, value := range []any{err, Formatter(err)} {
		got := fmt.Sprintf("%+v", value)
		for _, part := range []string{err.Error() + "\n\ntraces:\n", "root [capture]", "root/0 [wrapper]", "root/0/0 [join]", "root/0/0/0 [capture]", "root/0/0/1 [capture]", "TestDetailedTree", "common caller frames omitted"} {
			if !strings.Contains(got, part) {
				t.Errorf("missing %q in:\n%s", part, got)
			}
		}
	}
	got := fmt.Sprintf("%+v", Formatter(standard))
	if !strings.Contains(got, "root/0/0 [capture]") || !strings.Contains(got, "root/0/1 [capture]") {
		t.Fatalf("standard outermost tree lost branches:\n%s", got)
	}
	multi := Errorf("%w / %w", a, b)
	if got := fmt.Sprintf("%+v", multi); !strings.Contains(got, "root/1 [capture]") {
		t.Fatalf("multiple %%w lost a branch:\n%s", got)
	}
	shared := stderrors.Join(a, a)
	if got := fmt.Sprintf("%+v", Formatter(shared)); strings.Count(got, "TestDetailedTree") != 2 {
		t.Fatalf("shared sibling traces must both remain:\n%s", got)
	}
	custom := customWrapper{cyclicError{a, b}}
	got = fmt.Sprintf("%+v", Formatter(custom))
	if !strings.Contains(got, "root [wrapper]") || !strings.Contains(got, "root/0 [join]") || strings.Contains(got, "standard") {
		t.Fatalf("custom unwrap labels must describe structure, not origin:\n%s", got)
	}
	if got := fmt.Sprintf("%+v", Errorf("%w", Join(io.EOF))); !strings.Contains(got, "1 joined cause (see child details)") {
		t.Fatalf("single joined cause must use singular wording:\n%s", got)
	}
}

type customWrapper struct{ error }

func (e customWrapper) Unwrap() error { return e.error }

type panicUnwrapError struct{}

func (panicUnwrapError) Error() string { return "bad unwrap" }
func (panicUnwrapError) Unwrap() error { panic("bad unwrap") }

func TestDetailedPanicRecovery(t *testing.T) {
	var typedNil *plainError
	for _, format := range []string{"%v", "%s"} {
		for _, value := range []any{Formatter(typedNil), Join(typedNil)} {
			if got := fmt.Sprintf(format, value); !strings.Contains(got, "PANIC=Format") {
				t.Errorf("ordinary formatting must retain fmt panic handling: %q", got)
			}
		}
	}
	broken := panicUnwrapError{}
	later := New("later")
	for _, value := range []any{
		Errorf("%w / %w / %w", typedNil, broken, later),
		Join(typedNil, broken, later),
		Formatter(stderrors.Join(typedNil, broken, later)),
	} {
		got := fmt.Sprintf("%+v", value)
		if !strings.Contains(got, "root/2 [capture]") ||
			strings.Count(got, "node details unavailable (panic)") != 2 ||
			strings.Contains(got, "PANIC=Format") {
			t.Errorf("panicking nodes suppressed later causes:\n%s", got)
		}
	}
	safe := Errorf("%w / %w", (*nilError)(nil), later)
	if got := fmt.Sprintf("%+v", safe); !strings.Contains(got, "root/0 [untraced]\n      typed nil") {
		t.Fatalf("safe nil-receiver methods must still be used:\n%s", got)
	}
}

type cyclicError []error

func (cyclicError) Error() string     { return "cycle" }
func (e cyclicError) Unwrap() []error { return e }

func TestTraversalLimits(t *testing.T) {
	cycle := make(cyclicError, 1)
	cycle[0] = cycle
	if got := fmt.Sprintf("%+v", Formatter(cycle)); !strings.Contains(got, "depth limit reached") {
		t.Fatal("non-comparable cyclic tree did not reach depth limit")
	}
	children := make([]error, maxTreeNodes+1)
	for i := range children {
		children[i] = io.EOF
	}
	got := fmt.Sprintf("%+v", Formatter(stderrors.Join(children...)))
	if !strings.Contains(got, "node limit reached") || strings.Count(got, "[untraced]") != maxTreeNodes-1 {
		t.Fatal("wide tree did not respect node limit")
	}
	// Place the wide node one level before the limit so omitted children
	// still consume the global node budget.
	var deepWide error = cyclicError(children)
	for range maxTreeDepth - 1 {
		deepWide = cyclicError{deepWide}
	}
	got = fmt.Sprintf("%+v", Formatter(deepWide))
	if !strings.Contains(got, "node limit reached") || strings.Count(got, "depth limit reached") != maxTreeNodes-maxTreeDepth {
		t.Fatal("depth omissions did not consume the node budget")
	}
	mixed := cyclicError{nil, cycle, New("after cycle")}
	if got := fmt.Sprintf("%+v", Formatter(mixed)); !strings.Contains(got, "root/2 [capture]") {
		t.Fatal("depth-limited branch suppressed a later sibling")
	}
}

func TestNestedMessages(t *testing.T) {
	var aggregate error
	leaf := &mutableError{message: "<item>"}
	for range 1000 {
		aggregate = Join(aggregate, leaf)
	}
	root := aggregate.Error()
	output := fmt.Sprintf("%+v", aggregate)
	if !strings.HasPrefix(output, root+"\n\ntraces:\n") {
		t.Fatal("aggregate root message was changed")
	}
	details := output[len(root):]
	if strings.Count(details, "<item>") > maxTreeDepth || !strings.Contains(details, "2 joined causes") {
		t.Fatal("nested joins repeated aggregate messages")
	}
	if leaf.calls > 2000+maxTreeDepth {
		t.Fatal("nested joins repeatedly evaluated aggregate messages")
	}
	for _, message := range []string{
		strings.Repeat("a", maxMessageBytes-1) + "é end",
		"one\ntwo\nthree\nfour\nfive",
	} {
		err := Errorf("context: %w", namedError(message))
		output := fmt.Sprintf("%+v", err)
		if !strings.HasPrefix(output, err.Error()) {
			t.Fatal("root message was truncated")
		}
		details := output[len(err.Error()):]
		if !utf8.ValidString(details) || !strings.Contains(details, "... message truncated") || strings.Contains(details, message) {
			t.Fatal("nested message was not safely truncated")
		}
	}
}

func TestCommonSuffix(t *testing.T) {
	a := runtime.Frame{PC: 1, Function: "recursive", File: "a.go", Line: 1}
	b := runtime.Frame{PC: 2, Function: "caller", File: "a.go", Line: 2}
	c := runtime.Frame{PC: 3, Function: "other", File: "a.go", Line: 3}
	if got := commonSuffix([]runtime.Frame{a, a, b}, []runtime.Frame{c, a, b}); got != 2 {
		t.Fatalf("common suffix = %d, want 2", got)
	}
	if got := commonSuffix([]runtime.Frame{a, b, a}, []runtime.Frame{a, b, c}); got != 0 {
		t.Fatalf("interior frames were removed: %d", got)
	}
	if got := commonSuffix([]runtime.Frame{a, b}, []runtime.Frame{a, b}); got != 1 {
		t.Fatalf("capture frame was removed: %d", got)
	}
	var pcs [2]uintptr
	runtime.Callers(1, pcs[:])
	child := &plainError{trace: trace{message: "child", pcs: pcs[:]}}
	parent := &singleError{trace: trace{message: "parent", pcs: pcs[:]}, child: child}
	if got := fmt.Sprintf("%+v", parent); !strings.Contains(got, "1 common caller frame omitted") {
		t.Fatalf("one omitted frame must use singular wording:\n%s", got)
	}
}

//go:noinline
func recursiveError(depth int) error {
	if depth == 0 {
		return New("deep")
	}
	err := recursiveError(depth - 1)
	runtime.KeepAlive(depth)
	return err
}

func TestTruncatedStackParent(t *testing.T) {
	var pcs [3]uintptr
	n := runtime.Callers(1, pcs[:])
	leaf := &plainError{trace: trace{message: "leaf", pcs: pcs[:n]}}
	truncated := &singleError{
		trace: trace{message: "truncated", pcs: pcs[:n], truncated: true},
		child: leaf,
	}
	err := &multiError{
		trace:    trace{message: "root", pcs: pcs[:n]},
		children: []error{truncated, leaf},
	}
	got := fmt.Sprintf("%+v", err)
	first, later, found := strings.Cut(got, "    root/1 [capture]\n")
	if !found || !strings.Contains(first, "root/0/0 [capture]") ||
		strings.Contains(first, "common caller") ||
		!strings.Contains(later, "common caller") {
		t.Fatalf("truncated branch changed ancestor handling:\n%s", got)
	}
}

func TestStackLimitAndConcurrentFormatting(t *testing.T) {
	deep := recursiveError(maxStackPCs + 10)
	stack := deep.(interface{ stack() *trace }).stack()
	if len(stack.pcs) != maxStackPCs || !stack.truncated {
		t.Fatal("stack capture limit not applied")
	}
	if got := fmt.Sprintf("%+v", deep); !strings.Contains(got, "stack truncated after 64 PCs") || strings.Count(got, ".recursiveError\n") != maxStackPCs {
		t.Fatal("missing stack truncation marker or recursive frames")
	}
	err := Join(Errorf("read: %w", New("leaf")), deep)
	want := fmt.Sprintf("%+v", err)
	before := append([]uintptr(nil), stack.pcs...)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 10 {
				if got := fmt.Sprintf("%+v", err); got != want {
					t.Error("concurrent formatting changed output")
				}
			}
		})
	}
	wg.Wait()
	if !reflect.DeepEqual(before, stack.pcs) {
		t.Fatal("formatting mutated captured PCs")
	}
}

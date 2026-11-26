package term

import (
	"reflect"
	"testing"
)

func TestLinesKeepTheirWriteTimeIntoScrollback(t *testing.T) {
	screen := New(10, 3, 100)
	now := int64(0)
	screen.SetClock(func() int64 { return now })

	for i, text := range []string{"one\r\n", "two\r\n", "three\r\n", "four\r\n", "five"} {
		now = int64(100 + i)
		screen.Write([]byte(text))
	}
	// "one" and "two" scrolled off; the view shows three, four, five.
	if screen.Scrollback() != 2 {
		t.Fatalf("scrollback = %d, want 2", screen.Scrollback())
	}
	if got := screen.Row(0).Written; got != 102 {
		t.Fatalf("row 0 written = %d, want 102", got)
	}
	screen.Scroll(2)
	for i, want := range []int64{100, 101, 102} {
		if got := screen.Row(i).Written; got != want {
			t.Fatalf("history view row %d written = %d, want %d (%q)", i, got, want, screen.RowText(i))
		}
		if got := screen.LineNumber(i); got != uint64(i+1) {
			t.Fatalf("row %d number = %d, want %d", i, got, i+1)
		}
	}
}

// A full-screen program on the main screen (top does not use the alternate
// screen) repaints its rows in place, but a repaint is not new output: the
// time must stay when the row's content appeared.
func TestRewritingALineKeepsItsWriteTime(t *testing.T) {
	screen := New(10, 3, 10)
	now := int64(100)
	screen.SetClock(func() int64 { return now })
	screen.Write([]byte("one\r\ntwo\r\n"))

	now = 200
	screen.Write([]byte("\x1b[HONE"))
	if got := screen.Row(0).Written; got != 100 {
		t.Fatalf("rewritten row written = %d, want 100", got)
	}
	if got := screen.Row(1).Written; got != 100 {
		t.Fatalf("untouched row written = %d, want 100", got)
	}

	now = 300
	screen.Write([]byte("\x1b[2;1HTHREE"))
	if got := screen.Row(1).Written; got != 100 {
		t.Fatalf("erased and rewritten row written = %d, want 100", got)
	}
}

func TestErasingALineKeepsItsWriteTime(t *testing.T) {
	screen := New(10, 3, 10)
	now := int64(100)
	screen.SetClock(func() int64 { return now })
	screen.Write([]byte("one\r\n"))

	now = 200
	screen.Write([]byte("\x1b[H\x1b[K"))
	if got := screen.Row(0).Written; got != 100 {
		t.Fatalf("erased row written = %d, want 100", got)
	}
}

func TestErasingAnUnwrittenLineLeavesItUnstamped(t *testing.T) {
	screen := New(10, 3, 10)
	screen.SetClock(func() int64 { return 200 })
	screen.Write([]byte("\x1b[H\x1b[2J"))
	if got := screen.Row(0).Written; got != 0 {
		t.Fatalf("unwritten row written = %d, want 0", got)
	}
}

// The clock is asked once per Write, not once per line or character: the write
// paths stamp every run of text they print.
func TestClockIsAskedOncePerWrite(t *testing.T) {
	screen := New(20, 4, 10)
	calls := 0
	screen.SetClock(func() int64 { calls++; return 100 })
	screen.Write([]byte("one\r\ntwo\r\nthree\x1b[H\x1b[K"))
	if calls != 1 {
		t.Fatalf("clock calls = %d for one write, want 1", calls)
	}
}

func TestLineNumbersSurviveDroppedHistory(t *testing.T) {
	screen := New(10, 3, 2)
	for range 20 {
		screen.Write([]byte("x\r\n"))
	}
	// 20 lines were written, the screen holds the last 2 plus the blank cursor row.
	if got := screen.LineNumber(2); got != 21 {
		t.Fatalf("cursor row number = %d, want 21", got)
	}
	screen.Scroll(2)
	if got := screen.LineNumber(0); got != 17 {
		t.Fatalf("oldest retained line number = %d, want 17", got)
	}
}

func TestNoClockRecordsNothing(t *testing.T) {
	screen := New(10, 3, 10)
	screen.Write([]byte("hello\r\n"))
	if got := screen.Row(0).Written; got != 0 {
		t.Fatalf("written = %d without a clock", got)
	}
}

// The gutter numbers logical lines: a soft-wrapped run is one line, numbered
// on the row that begins it, and a resize that rewraps it into more or fewer
// rows moves no number.
func TestLogicalLineNumbersSkipWrappedRows(t *testing.T) {
	screen := New(10, 4, 20)
	screen.Write([]byte("0123456789ABCDEF\r\nsecond\r\nthird"))
	numbers, heads := screen.LogicalLineNumbers(nil, nil)
	if !reflect.DeepEqual(numbers, []uint64{1, 1, 2, 3}) {
		t.Fatalf("numbers = %v, want [1 1 2 3]", numbers)
	}
	if !reflect.DeepEqual(heads, []bool{true, false, true, true}) {
		t.Fatalf("heads = %v, want [true false true true]", heads)
	}
}

func TestResizeKeepsLogicalLineNumbers(t *testing.T) {
	screen := New(12, 4, 20)
	screen.Write([]byte("0123456789ABCDEF\r\nsecond\r\nthird"))
	before, beforeHeads := screen.LogicalLineNumbers(nil, nil)
	if !reflect.DeepEqual(before, []uint64{1, 1, 2, 3}) {
		t.Fatalf("numbers = %v, want [1 1 2 3]", before)
	}
	if !reflect.DeepEqual(beforeHeads, []bool{true, false, true, true}) {
		t.Fatalf("heads = %v, want [true false true true]", beforeHeads)
	}

	// Narrowing rewraps the first line into four rows. The view follows the
	// cursor, so it is the line's tail that leaves the screen; scrolled to the
	// oldest retained line, every visible row of the wrapped line carries the
	// line's number and only its first row begins it.
	screen.Resize(5, 4)
	screen.Scroll(100)
	narrow, narrowHeads := screen.LogicalLineNumbers(before, beforeHeads)
	if !reflect.DeepEqual(narrow, []uint64{1, 1, 1, 1}) {
		t.Fatalf("numbers at width 5 = %v, want [1 1 1 1]", narrow)
	}
	if !reflect.DeepEqual(narrowHeads, []bool{true, false, false, false}) {
		t.Fatalf("heads at width 5 = %v, want only the first row to begin the line", narrowHeads)
	}
	screen.Scroll(-2)
	if middle, middleHeads := screen.LogicalLineNumbers(narrow, narrowHeads); !reflect.DeepEqual(middle, []uint64{1, 1, 2, 2}) || !reflect.DeepEqual(middleHeads, []bool{false, false, true, false}) {
		t.Fatalf("numbers across the boundary = %v with heads %v, want [1 1 2 2] with [false false true false]", middle, middleHeads)
	}
	screen.ScrollToBottom()
	if bottom, _ := screen.LogicalLineNumbers(narrow, narrowHeads); !reflect.DeepEqual(bottom, []uint64{2, 2, 3, 3}) {
		t.Fatalf("numbers at the cursor = %v, want [2 2 3 3]", bottom)
	}

	screen.Resize(12, 4)
	wide, wideHeads := screen.LogicalLineNumbers(narrow, narrowHeads)
	if !reflect.DeepEqual(wide, before) || !reflect.DeepEqual(wideHeads, beforeHeads) {
		t.Fatalf("numbers after widening = %v with heads %v, want %v with heads %v", wide, wideHeads, before, beforeHeads)
	}
}

func TestResizeKeepsWrittenTimes(t *testing.T) {
	screen := New(12, 4, 20)
	now := int64(100)
	screen.SetClock(func() int64 { return now })
	screen.Write([]byte("0123456789ABCDEF\r\nsecond"))

	now = 200
	screen.Resize(5, 4)
	for row := range 4 {
		if got := screen.Row(row).Written; got != 100 {
			t.Fatalf("row %d written = %d after the rewrap, want 100", row, got)
		}
	}

	now = 300
	screen.Resize(12, 4)
	// The fourth row was never written to, before or after.
	for row, want := range []int64{100, 100, 100, 0} {
		if got := screen.Row(row).Written; got != want {
			t.Fatalf("row %d written = %d after rejoining, want %d", row, got, want)
		}
	}
}

// A shell redraws its prompt after a resize. The redraw is not new output: the
// prompt row keeps the time it appeared at, through the resize's display copy
// and through the redraw that replaces it.
func TestResizeRedrawKeepsThePromptTime(t *testing.T) {
	screen := New(10, 3, 10)
	now := int64(100)
	screen.SetClock(func() int64 { return now })
	screen.SetPromptToken("t")
	screen.Write([]byte("\x1b]133;A;motty=t\x07$ "))
	if got := screen.Row(0).Written; got != 100 {
		t.Fatalf("prompt written = %d, want 100", got)
	}

	now = 200
	screen.Resize(4, 3)
	if got := screen.Row(0).Written; got != 100 {
		t.Fatalf("prompt copy written = %d after the resize, want 100", got)
	}

	now = 300
	screen.Write([]byte("\x1b[H$ >"))
	if got := screen.Row(0).Written; got != 100 {
		t.Fatalf("prompt written = %d after the shell's redraw, want 100", got)
	}
}

// Numbers keep counting past dropped history, wrapped lines included: a
// dropped run still accounts for exactly one number.
func TestLogicalLineNumbersKeepCountingPastDroppedHistory(t *testing.T) {
	screen := New(8, 3, 4)
	screen.Write([]byte("abcdefghij\r\none\r\ntwo\r\nthree\r\nfour\r\nfive"))
	numbers, heads := screen.LogicalLineNumbers(nil, nil)
	for row := 1; row < len(numbers); row++ {
		step := int(numbers[row]) - int(numbers[row-1])
		if step != 0 && step != 1 {
			t.Fatalf("numbers %v step over more than one line at row %d", numbers, row)
		}
		if heads[row] != (step == 1) {
			t.Fatalf("heads %v disagree with numbers %v at row %d", heads, numbers, row)
		}
	}
	if numbers[0] < 1 {
		t.Fatalf("numbers %v start below 1", numbers)
	}
}

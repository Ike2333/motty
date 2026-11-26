package workbench

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/keymap"
)

func gutterTexts(canvas *gfx.Canvas) []string {
	var texts []string
	for _, text := range canvas.Texts() {
		texts = append(texts, text.Value)
	}
	return texts
}

func anyContains(texts []string, want string) bool {
	for _, text := range texts {
		if strings.Contains(text, want) {
			return true
		}
	}
	return false
}

func TestGutterShowsNumbersAndTimesAndHidesOnTheAlternateScreen(t *testing.T) {
	state, session, _ := selectionState()
	state.viewport.Width = 600
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local).Unix()
	state.wallClock = func() int64 { return stamp }
	state.setTerminalGutter(gutterBoth)
	geometry := state.layoutForSession(session.ID)
	session.term.write([]byte("alpha\r\nbeta\r\n"))
	want := time.Unix(stamp, 0).Format("15:04:05")

	var canvas gfx.Canvas
	state.paintGrid(&canvas, session, geometry)
	texts := gutterTexts(&canvas)
	if !anyContains(texts, want) || !slices.Contains(texts, "2") {
		t.Fatalf("no gutter for line 2 in %q", texts)
	}

	canvas.Reset()
	session.term.write([]byte("\x1b[?1049h"))
	state.paintGrid(&canvas, session, geometry)
	if texts := gutterTexts(&canvas); anyContains(texts, want) {
		t.Fatalf("gutter still painted on the alternate screen: %q", texts)
	}
}

// A program that repaints a row in place (top, a progress bar) is not new
// output: the gutter must keep showing when the row's content appeared.
func TestGutterKeepsRepaintedLines(t *testing.T) {
	state, session, _ := selectionState()
	state.viewport.Width = 600
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local).Unix()
	state.wallClock = func() int64 { return stamp }
	state.setTerminalGutter(gutterTime)
	geometry := state.layoutForSession(session.ID)
	session.term.write([]byte("alpha"))

	var canvas gfx.Canvas
	state.paintGrid(&canvas, session, geometry)
	first := time.Unix(stamp, 0).Format("15:04:05")
	if !anyContains(gutterTexts(&canvas), first) {
		t.Fatalf("gutter = %q, want %s", gutterTexts(&canvas), first)
	}

	stamp += 60
	canvas.Reset()
	session.term.write([]byte("\x1b[HALPHA"))
	state.paintGrid(&canvas, session, geometry)
	second := time.Unix(stamp, 0).Format("15:04:05")
	texts := gutterTexts(&canvas)
	if anyContains(texts, second) || !anyContains(texts, first) {
		t.Fatalf("gutter after repaint = %q, want %s and not %s", texts, first, second)
	}
}

func TestGutterTakesColumnsAndCopyLeavesItOut(t *testing.T) {
	state, session, _ := selectionState()
	state.viewport.Width = 600
	state.setTerminalGutter(gutterOff)
	before := state.layoutForSession(session.ID)
	state.setTerminalGutter(gutterBoth)
	geometry := state.layoutForSession(session.ID)
	if geometry.cols >= before.cols || geometry.left <= before.left {
		t.Fatalf("gutter did not take room: %+v -> %+v", before, geometry)
	}
	if cols, _ := session.term.screen.Size(); cols != geometry.cols {
		t.Fatalf("terminal has %d columns, layout %d: the remote would be told the wrong size", cols, geometry.cols)
	}

	session.term.write([]byte("alpha beta\r\n"))
	hit := terminalTestHit(state, session)
	startX, startY := cellPoint(geometry, 0, 2, state.cellWidth, state.cellHeight)
	endX, endY := cellPoint(geometry, 0, 7, state.cellWidth, state.cellHeight)
	state.Pointer(gfx.PointerEvent{Kind: gfx.PointerDown, Button: gfx.PointerButtonPrimary, Target: hit, X: startX, Y: startY, Viewport: state.viewport})
	state.Pointer(gfx.PointerEvent{Kind: gfx.PointerMove, Button: gfx.PointerButtonPrimary, Target: hit, Captured: true, X: endX, Y: endY, Viewport: state.viewport})
	state.Pointer(gfx.PointerEvent{Kind: gfx.PointerUp, Button: gfx.PointerButtonPrimary, Target: hit, Captured: true, X: endX, Y: endY, Viewport: state.viewport})
	if copied := state.Shortcut(gfx.KeyEvent{Target: hit, Modifiers: keymap.Primary, Text: "c"}).ClipboardText; copied != "pha be" {
		t.Fatalf("copied %q", copied)
	}
}

func TestGutterSettingPersists(t *testing.T) {
	options := freshOptions(t)
	options.Keystore = &memKeys{}
	state := newTestState(t, options)
	state.Click(hitSettings)
	state.selectSettingsCategory(settingsTerminal)
	clickSettingsChoice(t, state, state.settingsGutterLabel(gutterTime))
	if restored := newTestState(t, options); restored.terminalGutter != gutterTime {
		t.Fatalf("restored gutter %v", restored.terminalGutter)
	}
}

func countExact(texts []string, want string) int {
	count := 0
	for _, text := range texts {
		if text == want {
			count++
		}
	}
	return count
}

// A resize rewraps lines into more or fewer rows. The strip skips the rows a
// line gains, every number is painted once wherever the wrap falls, and the
// times stay what they were: a rewrap is not new output, so the clock must not
// move to the resize.
func TestGutterSkipsRewrappedRowsAndKeepsNumbersAndTimes(t *testing.T) {
	state, session, _ := selectionState()
	state.viewport.Width = 600
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local).Unix()
	clock := stamp
	state.wallClock = func() int64 { return clock }
	state.setTerminalGutter(gutterBoth)
	cols, _ := session.term.screen.Size()
	session.term.write([]byte(strings.Repeat("ab", cols) + "\r\nbeta\r\ngamma"))
	want := time.Unix(stamp, 0).Format("15:04:05")

	var canvas gfx.Canvas
	state.paintGrid(&canvas, session, state.layoutForSession(session.ID))
	texts := gutterTexts(&canvas)
	for _, number := range []string{"1", "2", "3"} {
		if got := countExact(texts, number); got != 1 {
			t.Fatalf("number %s painted %d times in %q", number, got, texts)
		}
	}
	// Every line was written in the same second, so each of the three lines
	// shows it once on the row that begins it.
	if got := countExact(texts, want); got != 3 {
		t.Fatalf("time %s painted %d times in %q", want, got, texts)
	}

	clock += 3600
	later := time.Unix(clock, 0).Format("15:04:05")
	state.Resize(gfx.Size{Width: 600 - 4*state.cellWidth, Height: state.viewport.Height})
	canvas.Reset()
	state.paintGrid(&canvas, session, state.layoutForSession(session.ID))
	texts = gutterTexts(&canvas)
	for _, number := range []string{"1", "2", "3"} {
		if got := countExact(texts, number); got != 1 {
			t.Fatalf("number %s painted %d times after narrowing in %q", number, got, texts)
		}
	}
	if got, gotLater := countExact(texts, want), countExact(texts, later); got != 3 || gotLater != 0 {
		t.Fatalf("times after narrowing: %s painted %d times, %s painted %d times in %q", want, got, later, gotLater, texts)
	}

	state.Resize(gfx.Size{Width: 600, Height: state.viewport.Height})
	canvas.Reset()
	state.paintGrid(&canvas, session, state.layoutForSession(session.ID))
	texts = gutterTexts(&canvas)
	for _, number := range []string{"1", "2", "3"} {
		if got := countExact(texts, number); got != 1 {
			t.Fatalf("number %s painted %d times after widening in %q", number, got, texts)
		}
	}
	if got, gotLater := countExact(texts, want), countExact(texts, later); got != 3 || gotLater != 0 {
		t.Fatalf("times after widening: %s painted %d times, %s painted %d times in %q", want, got, later, gotLater, texts)
	}
}

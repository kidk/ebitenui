package widget

import (
	img "image"
	"image/color"
	"math"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ebitenui/ebitenui/event"
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/internal/jsUtil"
	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"
)

type TextInput struct {
	ChangedEvent *event.Event
	SubmitEvent  *event.Event

	inputText string

	widgetOpts      []WidgetOpt
	caretOpts       []CaretOpt
	image           *TextInputImage
	color           *TextInputColor
	padding         Insets
	face            font.Face
	repeatDelay     time.Duration
	repeatInterval  time.Duration
	validationFunc  TextInputValidationFunc
	placeholderText string

	init                  *MultiOnce
	commandToFunc         map[textInputControlCommand]textInputCommandFunc
	widget                *Widget
	caret                 *Caret
	text                  *Text
	renderBuf             *image.MaskedRenderBuffer
	mask                  *image.NineSlice
	cursorPosition        int
	state                 textInputState
	scrollOffset          int
	focused               bool
	lastInputText         string
	secure                bool
	secureInputText       string
	clearOnSubmit         bool
	ignoreEmptySubmit     bool
	allowDuplicateSubmit  bool
	previousSubmittedText *string
	tabOrder              int
}

type TextInputOpt func(t *TextInput)

type TextInputOptions struct {
}

type TextInputChangedEventArgs struct {
	TextInput *TextInput
	InputText string
}

type TextInputChangedHandlerFunc func(args *TextInputChangedEventArgs)

type TextInputImage struct {
	Idle     *image.NineSlice
	Disabled *image.NineSlice
}

type TextInputColor struct {
	Idle          color.Color
	Disabled      color.Color
	Caret         color.Color
	DisabledCaret color.Color
}

type TextInputValidationFunc func(newInputText string) (bool, *string)

type textInputState func() (textInputState, bool)

type textInputControlCommand int

type textInputCommandFunc func()

var TextInputOpts TextInputOptions

const (
	textInputGoLeft = textInputControlCommand(iota + 1)
	textInputGoRight
	textInputGoStart
	textInputGoEnd
	textInputBackspace
	textInputDelete
	textInputEnter
)

var textInputKeyToCommand = map[ebiten.Key]textInputControlCommand{
	ebiten.KeyLeft:      textInputGoLeft,
	ebiten.KeyRight:     textInputGoRight,
	ebiten.KeyHome:      textInputGoStart,
	ebiten.KeyEnd:       textInputGoEnd,
	ebiten.KeyBackspace: textInputBackspace,
	ebiten.KeyDelete:    textInputDelete,
	ebiten.KeyEnter:     textInputEnter,
}

func NewTextInput(opts ...TextInputOpt) *TextInput {
	t := &TextInput{
		ChangedEvent: &event.Event{},
		SubmitEvent:  &event.Event{},

		repeatDelay:    300 * time.Millisecond,
		repeatInterval: 35 * time.Millisecond,

		init:          &MultiOnce{},
		commandToFunc: map[textInputControlCommand]textInputCommandFunc{},
		renderBuf:     image.NewMaskedRenderBuffer(),
	}
	t.state = t.idleState(true)

	t.commandToFunc[textInputGoLeft] = t.CursorMoveLeft
	t.commandToFunc[textInputGoRight] = t.CursorMoveRight
	t.commandToFunc[textInputGoStart] = t.CursorMoveStart
	t.commandToFunc[textInputGoEnd] = t.CursorMoveEnd
	t.commandToFunc[textInputBackspace] = t.Backspace
	t.commandToFunc[textInputDelete] = t.Delete
	t.commandToFunc[textInputEnter] = t.Submit

	t.init.Append(t.createWidget)

	for _, o := range opts {
		o(t)
	}

	return t
}

func (o TextInputOptions) WidgetOpts(opts ...WidgetOpt) TextInputOpt {
	return func(t *TextInput) {
		t.widgetOpts = append(t.widgetOpts, opts...)
	}
}

func (o TextInputOptions) CaretOpts(opts ...CaretOpt) TextInputOpt {
	return func(t *TextInput) {
		t.caretOpts = append(t.caretOpts, opts...)
	}
}

func (o TextInputOptions) ChangedHandler(f TextInputChangedHandlerFunc) TextInputOpt {
	return func(t *TextInput) {
		t.ChangedEvent.AddHandler(func(args interface{}) {
			f(args.(*TextInputChangedEventArgs))
		})
	}
}

func (o TextInputOptions) SubmitHandler(f TextInputChangedHandlerFunc) TextInputOpt {
	return func(t *TextInput) {
		t.SubmitEvent.AddHandler(func(args interface{}) {
			f(args.(*TextInputChangedEventArgs))
		})
	}
}

func (o TextInputOptions) ClearOnSubmit(clearOnSubmit bool) TextInputOpt {
	return func(t *TextInput) {
		t.clearOnSubmit = clearOnSubmit
	}
}

func (o TextInputOptions) IgnoreEmptySubmit(ignoreEmptySubmit bool) TextInputOpt {
	return func(t *TextInput) {
		t.ignoreEmptySubmit = ignoreEmptySubmit
	}
}

func (o TextInputOptions) AllowDuplicateSubmit(allowDuplicateSubmit bool) TextInputOpt {
	return func(t *TextInput) {
		t.allowDuplicateSubmit = allowDuplicateSubmit
	}
}

func (o TextInputOptions) Image(i *TextInputImage) TextInputOpt {
	return func(t *TextInput) {
		t.image = i
	}
}

func (o TextInputOptions) Color(c *TextInputColor) TextInputOpt {
	return func(t *TextInput) {
		t.color = c
	}
}

func (o TextInputOptions) Padding(i Insets) TextInputOpt {
	return func(t *TextInput) {
		t.padding = i
	}
}

func (o TextInputOptions) Face(f font.Face) TextInputOpt {
	return func(t *TextInput) {
		t.face = f
	}
}

func (o TextInputOptions) RepeatInterval(i time.Duration) TextInputOpt {
	return func(t *TextInput) {
		t.repeatInterval = i
	}
}

func (o TextInputOptions) Validation(f TextInputValidationFunc) TextInputOpt {
	return func(t *TextInput) {
		t.validationFunc = f
	}
}

func (o TextInputOptions) Placeholder(s string) TextInputOpt {
	return func(t *TextInput) {
		t.placeholderText = s
	}
}

func (o TextInputOptions) Secure(b bool) TextInputOpt {
	return func(t *TextInput) {
		t.secure = b
	}
}
func (o TextInputOptions) TabOrder(to int) TextInputOpt {
	return func(t *TextInput) {
		t.tabOrder = to
	}
}
func (t *TextInput) GetWidget() *Widget {
	t.init.Do()
	return t.widget
}

func (t *TextInput) SetLocation(rect img.Rectangle) {
	t.init.Do()
	t.widget.Rect = rect
}

func (t *TextInput) PreferredSize() (int, int) {
	t.init.Do()
	_, h := t.caret.PreferredSize()
	h = h + t.padding.Top + t.padding.Bottom
	w := 50

	if t.widget != nil && h < t.widget.MinHeight {
		h = t.widget.MinHeight
	}
	if t.widget != nil && w < t.widget.MinWidth {
		w = t.widget.MinWidth
	}

	return w, h
}

func (t *TextInput) Render(screen *ebiten.Image, def DeferredRenderFunc) {
	t.init.Do()

	t.text.GetWidget().Disabled = t.widget.Disabled
	if t.lastInputText != t.inputText {
		if t.validationFunc != nil {
			result, replacement := t.validationFunc(t.inputText)
			if !result {
				if replacement != nil {
					t.inputText = *replacement
				} else {
					t.inputText = t.lastInputText
				}
			}
		}
	}

	for {
		newState, rerun := t.state()
		if newState != nil {
			t.state = newState
		}
		if !rerun {
			break
		}
	}

	defer func() {
		t.lastInputText = t.inputText
	}()

	if t.inputText != t.lastInputText {
		t.ChangedEvent.Fire(&TextInputChangedEventArgs{
			TextInput: t,
			InputText: t.inputText,
		})

		if t.secure {
			t.secureInputText = strings.Repeat("*", len([]rune(t.inputText)))
		}
	}

	t.widget.Render(screen, def)

	t.renderImage(screen)
	t.renderTextAndCaret(screen, def)
}

func (t *TextInput) idleState(newKeyOrCommand bool) textInputState {
	return func() (textInputState, bool) {
		if !t.focused {
			return t.idleState(true), false
		}

		chars := input.InputChars()
		if len(chars) > 0 {
			return t.charsInputState(chars), true
		}

		st := textInputCheckForCommand(t, newKeyOrCommand)
		if st != nil {
			return st, true
		}

		if input.MouseButtonJustPressedLayer(ebiten.MouseButtonLeft, t.widget.EffectiveInputLayer()) {
			t.doGoXY(input.CursorPosition())
		}

		return t.idleState(true), false
	}
}

func textInputCheckForCommand(t *TextInput, newKeyOrCommand bool) textInputState {
	for key, cmd := range textInputKeyToCommand {
		if !input.KeyPressed(key) {
			continue
		}

		var delay time.Duration
		if newKeyOrCommand {
			delay = t.repeatDelay
		} else {
			delay = t.repeatInterval
		}

		return t.commandState(cmd, key, delay, nil, nil)
	}

	return nil
}

func (t *TextInput) charsInputState(c []rune) textInputState {
	return func() (textInputState, bool) {
		if !t.widget.Disabled {
			t.Insert(c)
		}

		t.caret.ResetBlinking()

		return t.idleState(true), false
	}
}

func (t *TextInput) commandState(cmd textInputControlCommand, key ebiten.Key, delay time.Duration, timer *time.Timer, expired *atomic.Value) textInputState {
	return func() (textInputState, bool) {
		if !input.KeyPressed(key) {
			return t.idleState(true), true
		}

		if timer != nil && expired.Load().(bool) {
			return t.idleState(false), true
		}

		if timer == nil {
			t.commandToFunc[cmd]()

			expired = &atomic.Value{}
			expired.Store(false)

			timer = time.AfterFunc(delay, func() {
				expired.Store(true)
			})

			return t.commandState(cmd, key, delay, timer, expired), false
		}

		return nil, false
	}
}

func (t *TextInput) insert(c rune) {
	t.lastInputText = t.inputText
	switch c {
	case 8:
		t.Backspace()
	case 13:
		t.Submit()
	case 35:
		t.CursorMoveEnd()
	case 36:
		t.CursorMoveStart()
	case 37:
		t.CursorMoveLeft()
	case 39:
		t.CursorMoveRight()
	case 46:
		t.Delete()
	default:
		t.Insert([]rune{c})
	}
}

func (t *TextInput) Insert(c []rune) {
	s := string(insertChars([]rune(t.inputText), c, t.cursorPosition))

	if t.validationFunc != nil {
		result, replacement := t.validationFunc(s)
		if !result {
			if replacement != nil {
				s = *replacement
			} else {
				return
			}
		}
	}

	t.inputText = s
	t.cursorPosition += len(c)
}

func (t *TextInput) CursorMoveLeft() {
	if t.cursorPosition > 0 {
		t.cursorPosition--
	}
	t.caret.ResetBlinking()
}

func (t *TextInput) CursorMoveRight() {
	if t.cursorPosition < len([]rune(t.inputText)) {
		t.cursorPosition++
	}
	t.caret.ResetBlinking()
}

func (t *TextInput) CursorMoveStart() {
	t.cursorPosition = 0
	t.caret.ResetBlinking()
}

func (t *TextInput) CursorMoveEnd() {
	t.cursorPosition = len([]rune(t.inputText))
	t.caret.ResetBlinking()
}

func (t *TextInput) doGoXY(x int, y int) {
	p := img.Point{x, y}
	if p.In(t.widget.Rect) {
		tr := t.padding.Apply(t.widget.Rect)
		if x < tr.Min.X {
			x = tr.Min.X
		}
		if x > tr.Max.X {
			x = tr.Max.X
		}

		t.cursorPosition = fontStringIndex([]rune(t.inputText), t.face, x-t.scrollOffset-tr.Min.X)
		if runtime.GOOS == "js" && runtime.GOARCH == "wasm" && jsUtil.IsMobileBrowser() {
			jsUtil.SetCursorPosition(t.cursorPosition)
		}

		t.caret.ResetBlinking()
	}
}

func (t *TextInput) Backspace() {
	if !t.widget.Disabled && t.cursorPosition > 0 {
		t.inputText = string(removeChar([]rune(t.inputText), t.cursorPosition-1))
		t.cursorPosition--
	}
	t.caret.ResetBlinking()
}

func (t *TextInput) Delete() {
	if !t.widget.Disabled && t.cursorPosition < len([]rune(t.inputText)) {
		t.inputText = string(removeChar([]rune(t.inputText), t.cursorPosition))
	}
	t.caret.ResetBlinking()
}

func (t *TextInput) Submit() {
	if !t.ignoreEmptySubmit || len(t.inputText) > 0 {
		if t.allowDuplicateSubmit || t.previousSubmittedText == nil || t.inputText != *t.previousSubmittedText {
			t.SubmitEvent.Fire(&TextInputChangedEventArgs{
				TextInput: t,
				InputText: t.inputText,
			})
			previousText := t.inputText
			t.previousSubmittedText = &previousText
		}
	}
	if t.clearOnSubmit {
		t.CursorMoveStart()
		t.inputText = ""
	}
}

func insertChars(r []rune, c []rune, pos int) []rune {
	res := make([]rune, len(r)+len(c))
	copy(res, r[:pos])
	copy(res[pos:], c)
	copy(res[pos+len(c):], r[pos:])
	return res
}

func removeChar(r []rune, pos int) []rune {
	res := make([]rune, len(r)-1)
	copy(res, r[:pos])
	copy(res[pos:], r[pos+1:])
	return res
}

func (t *TextInput) renderImage(screen *ebiten.Image) {
	if t.image != nil {
		i := t.image.Idle
		if t.widget.Disabled && t.image.Disabled != nil {
			i = t.image.Disabled
		}

		rect := t.widget.Rect
		i.Draw(screen, rect.Dx(), rect.Dy(), func(opts *ebiten.DrawImageOptions) {
			opts.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
		})
	}
}

func (t *TextInput) renderTextAndCaret(screen *ebiten.Image, def DeferredRenderFunc) {
	t.renderBuf.Draw(screen,
		func(buf *ebiten.Image) {
			t.drawTextAndCaret(buf, def)
		},
		func(buf *ebiten.Image) {
			rect := t.widget.Rect
			t.mask.Draw(buf, rect.Dx()-t.padding.Left-t.padding.Right, rect.Dy()-t.padding.Top-t.padding.Bottom,
				func(opts *ebiten.DrawImageOptions) {
					opts.GeoM.Translate(float64(rect.Min.X+t.padding.Left), float64(rect.Min.Y+t.padding.Top))
					opts.CompositeMode = ebiten.CompositeModeCopy
				})
		})
}

func (t *TextInput) drawTextAndCaret(screen *ebiten.Image, def DeferredRenderFunc) {
	rect := t.widget.Rect
	tr := rect
	tr = tr.Add(img.Point{t.padding.Left, t.padding.Top})

	inputStr := t.inputText
	if t.secure {
		inputStr = t.secureInputText
	}

	cx := 0
	if t.focused {
		sub := string([]rune(inputStr)[:t.cursorPosition])
		cx = fontAdvance(sub, t.face)

		dx := tr.Min.X + t.scrollOffset + cx + t.caret.Width + t.padding.Right - rect.Max.X
		if dx > 0 {
			t.scrollOffset -= dx
		}

		dx = tr.Min.X + t.scrollOffset + cx - t.padding.Left - rect.Min.X
		if dx < 0 {
			t.scrollOffset -= dx
		}
	}

	tr = tr.Add(img.Point{t.scrollOffset, 0})

	t.text.SetLocation(tr)
	if len([]rune(t.inputText)) > 0 {
		t.text.Label = inputStr
	} else {
		t.text.Label = t.placeholderText
	}
	if t.widget.Disabled || len([]rune(t.inputText)) == 0 {
		t.text.Color = t.color.Disabled
	} else {
		t.text.Color = t.color.Idle
	}
	t.text.Render(screen, def)

	if t.focused {
		if t.widget.Disabled {
			t.caret.Color = t.color.DisabledCaret
		} else {
			t.caret.Color = t.color.Caret
		}

		tr = tr.Add(img.Point{cx, 0})
		t.caret.SetLocation(tr)

		t.caret.Render(screen, def)
	}
}

func (t *TextInput) Focus(focused bool) {
	t.init.Do()
	t.GetWidget().FireFocusEvent(t, focused, img.Point{-1, -1})
	t.caret.resetBlinking()
	t.focused = focused

	if focused && runtime.GOOS == "js" && runtime.GOARCH == "wasm" && jsUtil.IsMobileBrowser() {
		result, successful := jsUtil.Prompt("Please enter a value.", t.inputText, t.cursorPosition, t.widget.Rect.Min.Y, t.setJSText)
		if successful {
			t.setText(result, true)
		}
	}
}

func (t *TextInput) GetText() string {
	return t.inputText
}

func (t *TextInput) SetText(text string) {
	t.setText(text, false)
}

func (t *TextInput) setJSText(text string) {
	t.setText(text, true)
}

func (t *TextInput) setText(text string, isJS bool) {
	t.init.Do()
	t.inputText = text
	if t.validationFunc != nil {
		result, replacement := t.validationFunc(t.inputText)
		if !result {
			if replacement != nil {
				t.inputText = *replacement
			} else {
				t.inputText = t.lastInputText
			}
		}
	}
	if t.inputText != t.lastInputText {
		if t.secure {
			t.secureInputText = strings.Repeat("*", len([]rune(t.inputText)))
		}
		t.ChangedEvent.Fire(&TextInputChangedEventArgs{
			TextInput: t,
			InputText: t.inputText,
		})
		t.lastInputText = t.inputText

		if isJS {
			t.cursorPosition = jsUtil.GetCursorPosition()
		} else {
			t.CursorMoveEnd()
		}
	}
}

func (t *TextInput) IsFocused() bool {
	return t.focused
}

func (t *TextInput) TabOrder() int {
	return t.tabOrder
}

func (t *TextInput) createWidget() {
	t.widget = NewWidget(t.widgetOpts...)
	t.widget.focusable = t
	t.widgetOpts = nil

	t.caret = NewCaret(append(t.caretOpts, CaretOpts.Color(t.color.Caret))...)
	t.caretOpts = nil

	t.text = NewText(TextOpts.Text("", t.face, color.White))

	t.mask = image.NewNineSliceColor(color.NRGBA{255, 0, 255, 255})
}

func fontAdvance(s string, f font.Face) int {
	_, a := font.BoundString(f, s)
	return int(math.Round(fixedInt26_6ToFloat64(a)))
}

// fontStringIndex returns an index into r that corresponds closest to pixel position x
// when string(r) is drawn using f. Pixel position x==0 corresponds to r[0].
func fontStringIndex(r []rune, f font.Face, x int) int {
	start := 0
	end := len(r)
	p := 0
loop:
	for {
		p = start + (end-start)/2
		sub := string(r[:p])
		a := fontAdvance(sub, f)

		switch {
		// x is right of advance
		case x > a:
			if p == start {
				break loop
			}

			start = p

		// x is left of advance
		case x < a:
			if end == p {
				break loop
			}

			end = p

		// x matches advance exactly
		default:
			return p
		}
	}

	if len(r) > 0 {
		a1 := fontAdvance(string(r[:p]), f)
		a2 := fontAdvance(string(r[:p+1]), f)
		if math.Abs(float64(x-a2)) < math.Abs(float64(x-a1)) {
			p++
		}
	}

	return p
}

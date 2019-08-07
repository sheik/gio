// SPDX-License-Identifier: Unlicense OR MIT

package input

import (
	"gioui.org/ui"
	"gioui.org/ui/input"
	"gioui.org/ui/internal/opconst"
	"gioui.org/ui/internal/ops"
	"gioui.org/ui/key"
)

type TextInputState uint8

type keyQueue struct {
	focus    input.Key
	handlers map[input.Key]*keyHandler
	reader   ops.Reader
	state    TextInputState
}

type keyHandler struct {
	active bool
}

type listenerPriority uint8

const (
	priNone listenerPriority = iota
	priDefault
	priCurrentFocus
	priNewFocus
)

const (
	TextInputKeep TextInputState = iota
	TextInputClose
	TextInputOpen
)

// InputState returns the last text input state as
// determined in Frame.
func (q *keyQueue) InputState() TextInputState {
	return q.state
}

func (q *keyQueue) Frame(root *ui.Ops, events *handlerEvents) {
	if q.handlers == nil {
		q.handlers = make(map[input.Key]*keyHandler)
	}
	for _, h := range q.handlers {
		h.active = false
	}
	q.reader.Reset(root)
	focus, pri, hide := q.resolveFocus(events)
	for k, h := range q.handlers {
		if !h.active {
			delete(q.handlers, k)
			if q.focus == k {
				q.focus = nil
				hide = true
			}
		}
	}
	if focus != q.focus {
		if q.focus != nil {
			events.Add(q.focus, key.FocusEvent{Focus: false})
		}
		q.focus = focus
		if q.focus != nil {
			events.Add(q.focus, key.FocusEvent{Focus: true})
		} else {
			hide = true
		}
	}
	switch {
	case pri == priNewFocus:
		q.state = TextInputOpen
	case hide:
		q.state = TextInputClose
	default:
		q.state = TextInputKeep
	}
}

func (q *keyQueue) Push(e input.Event, events *handlerEvents) {
	if q.focus != nil {
		events.Add(q.focus, e)
	}
}

func (q *keyQueue) resolveFocus(events *handlerEvents) (input.Key, listenerPriority, bool) {
	var k input.Key
	var pri listenerPriority
	var hide bool
loop:
	for encOp, ok := q.reader.Decode(); ok; encOp, ok = q.reader.Decode() {
		switch opconst.OpType(encOp.Data[0]) {
		case opconst.TypeKeyHandler:
			op := decodeKeyHandlerOp(encOp.Data, encOp.Refs)
			var newPri listenerPriority
			switch {
			case op.Focus:
				newPri = priNewFocus
			case op.Key == q.focus:
				newPri = priCurrentFocus
			default:
				newPri = priDefault
			}
			// Switch focus if higher priority or if focus requested.
			if newPri.replaces(pri) {
				k, pri = op.Key, newPri
			}
			h, ok := q.handlers[op.Key]
			if !ok {
				h = new(keyHandler)
				q.handlers[op.Key] = h
				// Reset the handler on (each) first appearance.
				events.Set(op.Key, []input.Event{key.FocusEvent{Focus: false}})
			}
			h.active = true
		case opconst.TypeHideInput:
			hide = true
		case opconst.TypePush:
			newK, newPri, h := q.resolveFocus(events)
			hide = hide || h
			if newPri.replaces(pri) {
				k, pri = newK, newPri
			}
		case opconst.TypePop:
			break loop
		}
	}
	return k, pri, hide
}

func (p listenerPriority) replaces(p2 listenerPriority) bool {
	// Favor earliest default focus or latest requested focus.
	return p > p2 || p == p2 && p == priNewFocus
}

func decodeKeyHandlerOp(d []byte, refs []interface{}) key.HandlerOp {
	if opconst.OpType(d[0]) != opconst.TypeKeyHandler {
		panic("invalid op")
	}
	return key.HandlerOp{
		Focus: d[1] != 0,
		Key:   refs[0].(input.Key),
	}
}

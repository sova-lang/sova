package codegen

import "fmt"

type HoistKit[Stmt any] struct {
	frames   []frame[Stmt]
	filePre  []Stmt
	filePost []Stmt

	scope  []int
	prefix string
}

type frame[Stmt any] struct {
	pre  []Stmt
	post []Stmt
}

func NewHoistKit[Stmt any](prefix string) *HoistKit[Stmt] {
	return &HoistKit[Stmt]{
		frames:   []frame[Stmt]{{}},
		filePre:  []Stmt{},
		filePost: []Stmt{},
		scope:    []int{0},
		prefix:   prefix,
	}
}

func (h *HoistKit[Stmt]) PushScope() {
	h.scope = append(h.scope, 0)
}

func (h *HoistKit[Stmt]) PopScope() {
	if len(h.scope) > 1 {
		h.scope = h.scope[:len(h.scope)-1]
	}
}

func (h *HoistKit[Stmt]) NewTemp() string {
	if len(h.scope) == 0 {
		h.PushScope()
	}

	top := len(h.scope) - 1
	h.scope[top]++
	return fmt.Sprintf("%s%d_%d", h.prefix, top, h.scope[top])
}

func (h *HoistKit[Stmt]) Begin() {
	h.frames = append(h.frames, frame[Stmt]{})
}

func (h *HoistKit[Stmt]) Before(s Stmt) {
	if n := len(h.frames); n > 0 {
		h.frames[n-1].pre = append(h.frames[n-1].pre, s)
		return
	}

	h.filePre = append(h.filePre, s)
}

func (h *HoistKit[Stmt]) After(s Stmt) {
	if n := len(h.frames); n > 0 {
		h.frames[n-1].post = append(h.frames[n-1].post, s)
		return
	}

	h.filePost = append(h.filePost, s)
}

func (h *HoistKit[Stmt]) End() (pre []Stmt, post []Stmt) {
	if len(h.frames) == 0 {
		return nil, nil
	}

	n := len(h.frames) - 1
	pre = h.frames[n].pre
	post = h.frames[n].post
	h.frames = h.frames[:n]
	return
}

func (h *HoistKit[Stmt]) FlushFileHoists() (pre []Stmt, post []Stmt) {
	pre, post = h.filePre, h.filePost
	h.filePre, h.filePost = nil, nil
	return
}

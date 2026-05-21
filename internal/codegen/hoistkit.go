package codegen

import "fmt"

// HoistKit is a utility structure that provides a stack-based mechanism for hoisting statements in the code generation process.
// It holds information about the current generation units position, allowing to hoist statements as pre- or postludes.
type HoistKit[Stmt any] struct {
	frames   []frame[Stmt] // frames is a stack of frames that hold pre- and post-statements for hoisting.
	filePre  []Stmt        // filePre is a slice of statements that are hoisted to the beginning of the file.
	filePost []Stmt        // filePost is a slice of statements that are hoisted to the end of the file.

	scope  []int  // scope is a stack that holds the current counter for temporary variables, used to ensure unique names across different scopes.
	prefix string // prefix is a string that is prepended to the names of temporary variables to ensure uniqueness.
}

// frame is a structure that holds a pair of slices for pre- and post-statements. A frame is basically a generation unit
// that can be used to hoist statements in the code generation process.
type frame[Stmt any] struct {
	pre  []Stmt
	post []Stmt
}

// NewHoistKit creates a new HoistKit instance with the specified prefix for temporary variable names.
func NewHoistKit[Stmt any](prefix string) *HoistKit[Stmt] {
	return &HoistKit[Stmt]{
		frames:   []frame[Stmt]{{}},
		filePre:  []Stmt{},
		filePost: []Stmt{},
		scope:    []int{0},
		prefix:   prefix,
	}
}

// PushScope pushes a new scope onto the stack defining a new counter for temporary variables.
func (h *HoistKit[Stmt]) PushScope() {
	h.scope = append(h.scope, 0)
}

// PopScope pops the current scope from the stack, returning to the previous counter for temporary variables.
func (h *HoistKit[Stmt]) PopScope() {
	if len(h.scope) > 1 {
		h.scope = h.scope[:len(h.scope)-1]
	}
}

// NewTemp generates a new temporary variable name based on the current scope and the prefix.
func (h *HoistKit[Stmt]) NewTemp() string {
	if len(h.scope) == 0 {
		h.PushScope()
	}
	top := len(h.scope) - 1
	h.scope[top]++
	return fmt.Sprintf("%s%d_%d", h.prefix, top, h.scope[top])
}

// Begin starts a new frame for which the hoisting is being done.
func (h *HoistKit[Stmt]) Begin() {
	h.frames = append(h.frames, frame[Stmt]{})
}

// Before registers a statement that should be inserted *before* the current statement.
// This is typically used for prelude statements that need to be executed before the current frame's statements.
func (h *HoistKit[Stmt]) Before(s Stmt) {
	if n := len(h.frames); n > 0 {
		h.frames[n-1].pre = append(h.frames[n-1].pre, s)
		return
	}
	// kein aktiver Frame → Datei-Präambel
	h.filePre = append(h.filePre, s)
}

// After registers a statement that should be inserted *after* the current statement.
// This is typically used for statements that should be executed after the current frame has been processed.
func (h *HoistKit[Stmt]) After(s Stmt) {
	if n := len(h.frames); n > 0 {
		h.frames[n-1].post = append(h.frames[n-1].post, s)
		return
	}
	h.filePost = append(h.filePost, s)
}

// End ends the current frame and returns the pre- and post-statements for that frame.
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

// FlushFileHoists flushes the hoisted statements that were collected for the file.
func (h *HoistKit[Stmt]) FlushFileHoists() (pre []Stmt, post []Stmt) {
	pre, post = h.filePre, h.filePost
	h.filePre, h.filePost = nil, nil
	return
}
